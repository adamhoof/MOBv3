//go:build admin

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	otaQTermChunkSize   = 16 * 1024
	otaBarcodeChunkSize = 512
	otaWindowSize       = 8
	otaTimeout          = 30 * time.Minute
	otaAckTimeout       = 180 * time.Second
	otaDiscoveryWindow  = 8 * time.Second
	otaChunkMagic       = "QOTA"
	otaSessionHexLen    = 32
	otaSigSectorSize    = 4096
	otaSigMagic         = 0xe7
	otaSigVersionRSA    = 2
)

type otaTarget struct {
	name            string
	controlTopic    string
	chunkTopic      string
	statusTopic     string
	chunkSize       int
	discoverDevices bool
}

type otaStatus struct {
	Type    string `json:"type"`
	Device  string `json:"device"`
	Status  string `json:"status"`
	Session string `json:"session"`
	Written int    `json:"written"`
	Err     string `json:"err"`
}

func qtermOTATarget() otaTarget {
	return otaTarget{name: "qterm", chunkSize: otaQTermChunkSize, discoverDevices: false}
}

func barcodeOTATarget() otaTarget {
	return otaTarget{name: "barcode", chunkSize: otaBarcodeChunkSize, discoverDevices: true}
}

func runOTACommand(cfg ctlConfig, args []string, target otaTarget) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: %s firmware.bin", target.name+"-ota")
	}
	applyOTATopics(cfg, &target)
	return runOTA(cfg, target, args[0])
}

func applyOTATopics(cfg ctlConfig, target *otaTarget) {
	if target.discoverDevices {
		target.controlTopic = cfg.BarcodeOTAControlTopic
		target.chunkTopic = cfg.BarcodeOTAChunkTopic
		target.statusTopic = cfg.BarcodeOTAStatusTopic
		return
	}
	target.controlTopic = cfg.QTermOTAControlTopic
	target.chunkTopic = cfg.QTermOTAChunkTopic
	target.statusTopic = cfg.QTermOTAStatusTopic
}

func runOTA(cfg ctlConfig, target otaTarget, firmwarePath string) error {
	if err := validateOTAConfig(cfg, target, firmwarePath); err != nil {
		return err
	}
	digest, size, err := sha256File(firmwarePath)
	if err != nil {
		return err
	}
	if size == 0 {
		return fmt.Errorf("firmware is empty")
	}
	if signed, err := firmwareLooksSigned(firmwarePath, size); err == nil && !signed && !target.discoverDevices {
		fmt.Fprintln(os.Stderr, "warning: firmware does not look signed; signed-update devices should reject it")
	}
	session, sessionBytes, err := newOTASession()
	if err != nil {
		return err
	}

	statusCh := make(chan otaStatus, 128)
	client, err := newMQTTClientWithHandler(cfg, func(_ mqtt.Client, msg mqtt.Message) {
		var status otaStatus
		if err := json.Unmarshal(msg.Payload(), &status); err != nil {
			return
		}
		if status.Type == "ota" && status.Session == session {
			select {
			case statusCh <- status:
			default:
			}
		}
	})
	if err != nil {
		return err
	}
	if err := connectMQTT(client); err != nil {
		return err
	}
	defer client.Disconnect(250)

	if err := subscribeMQTT(client, target.statusTopic); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	fmt.Printf("%s OTA session=%s size=%d sha256=%s\n", target.name, session, size, digest)
	begin := map[string]any{
		"command":     "ota_begin",
		"session":     session,
		"size":        size,
		"sha256":      digest,
		"timeout_sec": int(otaTimeout.Seconds()),
	}
	if err := publishMQTTBytes(client, target.controlTopic, 0, false, mustJSON(begin)); err != nil {
		return err
	}

	if target.discoverDevices {
		devices, err := discoverOTADevices(statusCh, otaDiscoveryWindow)
		if err != nil {
			_ = publishMQTTBytes(client, target.controlTopic, 0, false, mustJSON(map[string]any{"command": "ota_abort", "session": session}))
			return err
		}
		fmt.Printf("discovered %d barcode station(s): %s\n", len(devices), strings.Join(devices, ", "))
		if err := uploadOTAChunksForDevices(client, statusCh, target, firmwarePath, sessionBytes, size, devices); err != nil {
			_ = publishMQTTBytes(client, target.controlTopic, 0, false, mustJSON(map[string]any{"command": "ota_abort", "session": session}))
			return err
		}
		if err := publishMQTTBytes(client, target.controlTopic, 0, false, mustJSON(map[string]any{"command": "ota_finish", "session": session})); err != nil {
			return err
		}
		if err := waitAllOTADevices(statusCh, devices, map[string]bool{"done": true}, map[string]bool{"bad_finish": true, "finish_failed": true}, size, otaAckTimeout); err != nil {
			return err
		}
		fmt.Println("barcode OTA accepted by all discovered stations; stations will reboot")
		return nil
	}

	if _, err := waitOTAStatus(statusCh, map[string]bool{"started": true}, map[string]bool{"bad_begin": true, "begin_failed": true}, otaAckTimeout); err != nil {
		return err
	}
	if err := uploadOTAChunks(client, statusCh, target, firmwarePath, sessionBytes, size); err != nil {
		_ = publishMQTTBytes(client, target.controlTopic, 0, false, mustJSON(map[string]any{"command": "ota_abort", "session": session}))
		return err
	}
	if err := publishMQTTBytes(client, target.controlTopic, 0, false, mustJSON(map[string]any{"command": "ota_finish", "session": session})); err != nil {
		return err
	}
	if _, err := waitOTAStatus(statusCh, map[string]bool{"done": true}, map[string]bool{"bad_finish": true, "finish_failed": true}, otaAckTimeout); err != nil {
		return err
	}
	fmt.Println("qterm OTA accepted; terminal will reboot")
	return nil
}

func validateOTAConfig(cfg ctlConfig, target otaTarget, firmwarePath string) error {
	if strings.TrimSpace(cfg.MQTTBrokerURL) == "" {
		return fmt.Errorf("missing MOBCTL_MQTT_BROKER_URL or MQTT_PROTOCOL+MQTT_HOST+MQTT_PORT")
	}
	if strings.TrimSpace(target.controlTopic) == "" || strings.TrimSpace(target.chunkTopic) == "" || strings.TrimSpace(target.statusTopic) == "" {
		return fmt.Errorf("missing %s OTA topics", target.name)
	}
	st, err := os.Stat(firmwarePath)
	if err != nil {
		return fmt.Errorf("firmware: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("firmware path is a directory")
	}
	return nil
}

func firmwareLooksSigned(path string, size int64) (bool, error) {
	if size < otaSigSectorSize || size%otaSigSectorSize != 0 {
		return false, nil
	}
	fh, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer fh.Close()

	buf := make([]byte, otaSigSectorSize)
	if _, err := fh.ReadAt(buf, size-otaSigSectorSize); err != nil {
		return false, err
	}
	return buf[0] == otaSigMagic && buf[1] == otaSigVersionRSA, nil
}

func newOTASession() (string, []byte, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	hexSession := hex.EncodeToString(buf)
	return hexSession, []byte(hexSession), nil
}

func sha256File(path string) (string, int64, error) {
	fh, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer fh.Close()
	h := sha256.New()
	n, err := io.Copy(h, fh)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func uploadOTAChunks(client mqtt.Client, statusCh <-chan otaStatus, target otaTarget, firmwarePath string, session []byte, size int64) error {
	fh, err := os.Open(firmwarePath)
	if err != nil {
		return err
	}
	defer fh.Close()
	buf := make([]byte, target.chunkSize)
	offset := 0
	acked := 0
	pending := 0
	eof := false
	lastPercent := -1
	lastLog := time.Now()
	for !eof || pending > 0 {
		for !eof && pending < otaWindowSize {
			n, err := fh.Read(buf)
			if err == io.EOF {
				eof = true
				break
			}
			if err != nil {
				return err
			}
			if err := publishMQTTBytes(client, target.chunkTopic, 0, false, otaChunkFrame(session, offset, buf[:n])); err != nil {
				return err
			}
			offset += n
			pending++
		}
		if time.Since(lastLog) > 2*time.Second {
			fmt.Printf("sent %d/%d, acked %d/%d, pending_chunks=%d\n", offset, size, acked, size, pending)
			lastLog = time.Now()
		}

		status, err := waitOTAStatus(statusCh, map[string]bool{"chunk": true}, map[string]bool{"bad_chunk": true, "chunk_failed": true}, otaAckTimeout)
		if err != nil {
			return fmt.Errorf("%w after sent=%d acked=%d pending_chunks=%d", err, offset, acked, pending)
		}
		if status.Written < acked {
			continue
		}
		if status.Written > offset {
			return fmt.Errorf("device ack offset %d above sent offset %d", status.Written, offset)
		}
		acked = status.Written
		pending = (offset - acked + target.chunkSize - 1) / target.chunkSize
		percent := int((int64(acked) * 100) / size)
		if percent != lastPercent {
			lastPercent = percent
			fmt.Printf("uploaded %d/%d (%d%%)\n", acked, size, percent)
		}
	}
	if int64(acked) != size {
		return fmt.Errorf("device ack offset %d, expected final size %d", acked, size)
	}
	return nil
}

func uploadOTAChunksForDevices(client mqtt.Client, statusCh <-chan otaStatus, target otaTarget, firmwarePath string, session []byte, size int64, devices []string) error {
	fh, err := os.Open(firmwarePath)
	if err != nil {
		return err
	}
	defer fh.Close()
	buf := make([]byte, target.chunkSize)
	offset := 0
	lastPercent := -1
	for offset < int(size) {
		n, err := fh.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n > 0 {
			if err := publishMQTTBytes(client, target.chunkTopic, 0, false, otaChunkFrame(session, offset, buf[:n])); err != nil {
				return err
			}
			offset += n
		}
		nextAck := offset
		if int64(nextAck) > size {
			nextAck = int(size)
		}
		if nextAck%int(64*1024) != 0 && int64(nextAck) != size {
			if err == io.EOF {
				return fmt.Errorf("unexpected EOF at %d, expected %d", nextAck, size)
			}
			continue
		}
		if err := waitAllOTADevices(statusCh, devices, map[string]bool{"chunk": true}, map[string]bool{"bad_chunk": true, "chunk_failed": true}, int64(nextAck), otaAckTimeout); err != nil {
			return err
		}
		percent := int((int64(nextAck) * 100) / size)
		if percent != lastPercent {
			lastPercent = percent
			fmt.Printf("uploaded %d/%d (%d%%) to %d station(s)\n", nextAck, size, percent, len(devices))
		}
		if err == io.EOF {
			break
		}
	}
	return nil
}

func otaChunkFrame(session []byte, offset int, chunk []byte) []byte {
	frame := make([]byte, 0, 42+len(chunk))
	frame = append(frame, otaChunkMagic...)
	frame = append(frame, session...)
	var hdr [6]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(offset))
	binary.BigEndian.PutUint16(hdr[4:], uint16(len(chunk)))
	frame = append(frame, hdr[:]...)
	frame = append(frame, chunk...)
	return frame
}

func discoverOTADevices(statusCh <-chan otaStatus, window time.Duration) ([]string, error) {
	devices := map[string]bool{}
	timer := time.NewTimer(window)
	defer timer.Stop()
	for {
		select {
		case status := <-statusCh:
			if status.Status == "bad_begin" || status.Status == "begin_failed" {
				return nil, otaStatusError(status)
			}
			if status.Status == "started" {
				device := strings.TrimSpace(status.Device)
				if device == "" {
					return nil, errors.New("barcode station reported started without device id")
				}
				devices[device] = true
			}
		case <-timer.C:
			if len(devices) == 0 {
				return nil, fmt.Errorf("no barcode stations joined OTA session within %s", window)
			}
			out := make([]string, 0, len(devices))
			for device := range devices {
				out = append(out, device)
			}
			sort.Strings(out)
			return out, nil
		}
	}
}

func waitAllOTADevices(statusCh <-chan otaStatus, devices []string, ok, fail map[string]bool, minWritten int64, timeout time.Duration) error {
	remaining := map[string]bool{}
	for _, device := range devices {
		remaining[device] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for len(remaining) > 0 {
		select {
		case status := <-statusCh:
			device := strings.TrimSpace(status.Device)
			if !remaining[device] {
				continue
			}
			if fail[status.Status] {
				return otaStatusError(status)
			}
			if ok[status.Status] && int64(status.Written) >= minWritten {
				delete(remaining, device)
			}
		case <-ctx.Done():
			pending := make([]string, 0, len(remaining))
			for device := range remaining {
				pending = append(pending, device)
			}
			sort.Strings(pending)
			return fmt.Errorf("timed out waiting for OTA status from: %s", strings.Join(pending, ", "))
		}
	}
	return nil
}

func waitOTAStatus(statusCh <-chan otaStatus, ok, fail map[string]bool, timeout time.Duration) (otaStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		select {
		case status := <-statusCh:
			if ok[status.Status] {
				return status, nil
			}
			if fail[status.Status] {
				return status, otaStatusError(status)
			}
		case <-ctx.Done():
			return otaStatus{}, fmt.Errorf("timed out waiting for OTA status")
		}
	}
}

func otaStatusError(status otaStatus) error {
	device := strings.TrimSpace(status.Device)
	if device != "" {
		return fmt.Errorf("device %s reported %s: %s", device, status.Status, status.Err)
	}
	return fmt.Errorf("device reported %s: %s", status.Status, status.Err)
}

func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func newMQTTClientWithHandler(cfg ctlConfig, handler mqtt.MessageHandler) (mqtt.Client, error) {
	tlsConfig, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBrokerURL).
		SetClientID(cfg.MQTTClientID).
		SetTLSConfig(tlsConfig).
		SetAutoReconnect(false).
		SetConnectRetry(false).
		SetCleanSession(true)
	if handler != nil {
		opts.SetDefaultPublishHandler(handler)
	}
	return mqtt.NewClient(opts), nil
}

func connectMQTT(client mqtt.Client) error {
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("timed out connecting to MQTT broker")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("connect to MQTT broker: %w", err)
	}
	return nil
}

func publishMQTTBytes(client mqtt.Client, topic string, qos byte, retained bool, payload []byte) error {
	token := client.Publish(topic, qos, retained, payload)
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("timed out publishing MQTT message")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("publish MQTT message: %w", err)
	}
	return nil
}

func subscribeMQTT(client mqtt.Client, topic string) error {
	token := client.Subscribe(topic, 0, nil)
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("timed out subscribing to MQTT topic %s", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribe MQTT topic %s: %w", topic, err)
	}
	return nil
}
