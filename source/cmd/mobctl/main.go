package main

import (
	"bufio"
	"compress/gzip"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	envcfg "github.com/adamhoof/MOBv3/internal/config"
)

type ctlConfig struct {
	ServerURL      string
	ImportEndpoint string
	MDBPath        string
	CAPath         string
	ClientCertPath string
	ClientKeyPath  string
	Timeout        time.Duration

	MQTTBrokerURL          string
	MQTTClientID           string
	MQTTControlTopic       string
	QTermOTAControlTopic   string
	QTermOTAChunkTopic     string
	QTermOTAStatusTopic    string
	BarcodeOTAControlTopic string
	BarcodeOTAChunkTopic   string
	BarcodeOTAStatusTopic  string
}

type importResponse struct {
	Status   string `json:"status"`
	Imported int64  `json:"imported,omitempty"`
	Duration string `json:"duration,omitempty"`
	Error    string `json:"error,omitempty"`
}

func main() {
	configPath := flag.String("file", "", "required path to env config")
	flag.Parse()

	if *configPath == "" {
		path, err := defaultConfigPath()
		if err != nil {
			fatalf("default config path: %s", err)
		}
		*configPath = path
	}
	configDir := filepath.Dir(*configPath)
	if err := envcfg.LoadFile(*configPath); err != nil {
		fatalf("load config: %s", err)
	}
	cfg, err := loadConfig(configDir)
	if err != nil {
		fatalf("config: %s", err)
	}
	if flag.NArg() == 0 {
		if err := runInteractive(cfg, *configPath); err != nil {
			fatalf("interactive failed: %s", err)
		}
		return
	}
	if err := runCommand(cfg, flag.Arg(0), flag.Args()[1:]); err != nil {
		fatalf("%s failed: %s", flag.Arg(0), err)
	}
}

func runCommand(cfg ctlConfig, command string, args []string) error {
	switch strings.ToLower(command) {
	case "upd":
		return runImport(cfg, args)
	case "ss":
		return sendControlCommand(cfg, "ss", "sleep")
	case "sw":
		return sendControlCommand(cfg, "sw", "wake")
	default:
		if handleOptionalCommand(cfg, command, args) {
			return nil
		}
		return fmt.Errorf("unknown command %q", command)
	}
}

func runImport(cfg ctlConfig, args []string) error {
	fs := flag.NewFlagSet("upd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mdbPath := fs.String("mdb", cfg.MDBPath, "path to catalog .mdb or .mdb.gz")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *mdbPath == "" && fs.NArg() == 1 {
		*mdbPath = fs.Arg(0)
	}
	if *mdbPath == "" {
		return fmt.Errorf("missing MDB path; set MOBCTL_MDB_PATH or use upd --mdb path/to/catalog.mdb")
	}
	if err := validateMDBPath(*mdbPath); err != nil {
		return err
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	reader, closeReader, err := gzipMDB(*mdbPath, newImportProgress(os.Stderr))
	if err != nil {
		return err
	}
	defer closeReader()

	requestURL := endpointURL(cfg.ServerURL, cfg.ImportEndpoint)
	req, err := http.NewRequest(http.MethodPost, requestURL, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Filename", uploadFilename(filepath.Base(*mdbPath)))

	fmt.Printf("Importing %s via %s\n", *mdbPath, requestURL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send import request: %w", err)
	}
	defer resp.Body.Close()

	var result importResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, result.Error)
	}

	fmt.Printf("Import completed: %d products in %s\n", result.Imported, result.Duration)
	return nil
}

func loadConfig(configDir string) (ctlConfig, error) {
	serverURL := envcfg.Optional("MOBCTL_SERVER_URL", "")
	if serverURL == "" {
		host := envcfg.Optional("CATALOG_HTTP_HOST", "")
		port := envcfg.Optional("CATALOG_HTTP_PORT", "")
		if host == "" || port == "" {
			return ctlConfig{}, fmt.Errorf("missing MOBCTL_SERVER_URL or CATALOG_HTTP_HOST+CATALOG_HTTP_PORT")
		}
		if host == "0.0.0.0" {
			return ctlConfig{}, fmt.Errorf("MOBCTL_SERVER_URL is required when CATALOG_HTTP_HOST is 0.0.0.0")
		}
		serverURL = "https://" + host + ":" + port
	}
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return ctlConfig{}, fmt.Errorf("parse server url: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return ctlConfig{}, fmt.Errorf("server url must be https with host")
	}

	timeout, err := envcfg.OptionalDuration("CATALOG_IMPORT_TIMEOUT", time.Minute)
	if err != nil {
		return ctlConfig{}, err
	}
	mqttBrokerURL := envcfg.Optional("MOBCTL_MQTT_BROKER_URL", "")
	if mqttBrokerURL == "" {
		protocol := envcfg.Optional("MQTT_PROTOCOL", "tcps")
		host := envcfg.Optional("MQTT_HOST", "")
		port := envcfg.Optional("MQTT_PORT", "")
		if host != "" && port != "" {
			mqttBrokerURL = protocol + "://" + host + ":" + port
		}
	}
	cfg := ctlConfig{
		ServerURL:              serverURL,
		ImportEndpoint:         envcfg.Optional("CATALOG_IMPORT_ENDPOINT", "/catalog/import"),
		MDBPath:                resolveConfigPath(configDir, envcfg.Optional("MOBCTL_MDB_PATH", "")),
		CAPath:                 resolveConfigPath(configDir, envcfg.Optional("TLS_CA_PATH", "")),
		ClientCertPath:         resolveConfigPath(configDir, envcfg.Optional("TLS_CLIENT_CERT_PATH", "")),
		ClientKeyPath:          resolveConfigPath(configDir, envcfg.Optional("TLS_CLIENT_KEY_PATH", "")),
		Timeout:                timeout,
		MQTTBrokerURL:          mqttBrokerURL,
		MQTTClientID:           envcfg.Optional("MOBCTL_MQTT_CLIENT_ID", "mobctl"),
		MQTTControlTopic:       envcfg.Optional("MQTT_CONTROL_TOPIC", "station/control"),
		QTermOTAControlTopic:   envcfg.Optional("QTERM_OTA_CONTROL_TOPIC", "qterm/ota/control"),
		QTermOTAChunkTopic:     envcfg.Optional("QTERM_OTA_CHUNK_TOPIC", "qterm/ota/chunk"),
		QTermOTAStatusTopic:    envcfg.Optional("QTERM_OTA_STATUS_TOPIC", "qterm/ota/status"),
		BarcodeOTAControlTopic: envcfg.Optional("BARCODE_OTA_CONTROL_TOPIC", "station/ota/control"),
		BarcodeOTAChunkTopic:   envcfg.Optional("BARCODE_OTA_CHUNK_TOPIC", "station/ota/chunk"),
		BarcodeOTAStatusTopic:  envcfg.Optional("BARCODE_OTA_STATUS_TOPIC", "station/ota/status"),
	}
	for name, value := range map[string]string{
		"TLS_CA_PATH":          cfg.CAPath,
		"TLS_CLIENT_CERT_PATH": cfg.ClientCertPath,
		"TLS_CLIENT_KEY_PATH":  cfg.ClientKeyPath,
	} {
		if value == "" {
			return ctlConfig{}, fmt.Errorf("missing %s", name)
		}
	}
	return cfg, nil
}

func defaultConfigPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), ".env"), nil
}

func resolveConfigPath(configDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(configDir, path)
}

func runInteractive(cfg ctlConfig, configPath string) error {
	fmt.Printf("Loaded config: %s\n", configPath)
	printCommands()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("HekrMejMej > ")
		if !scanner.Scan() {
			return scanner.Err()
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		command, args := splitInteractiveCommand(input)
		switch strings.ToLower(command) {
		case "e", "exit", "quit":
			return nil
		case "ls", "help":
			printCommands()
			continue
		}
		if err := runCommand(cfg, command, args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
}

func splitInteractiveCommand(input string) (string, []string) {
	command, rest, ok := strings.Cut(strings.TrimSpace(input), " ")
	if !ok {
		return command, nil
	}
	args := strings.Fields(strings.TrimSpace(rest))
	return command, args
}

func printCommands() {
	fmt.Printf("Commands: upd, ss, sw%s, ls, e\n", optionalCommands())
}

func newHTTPClient(cfg ctlConfig) (*http.Client, error) {
	tlsConfig, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &http.Client{Timeout: cfg.Timeout, Transport: &http.Transport{TLSClientConfig: tlsConfig}}, nil
}

func clientTLSConfig(cfg ctlConfig) (*tls.Config, error) {
	caCert, err := os.ReadFile(cfg.CAPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("parse CA cert")
	}
	cert, err := tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}
	return &tls.Config{
		RootCAs:      rootCAs,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func sendControlCommand(cfg ctlConfig, command, payload string) error {
	if strings.TrimSpace(cfg.MQTTBrokerURL) == "" {
		return fmt.Errorf("missing MOBCTL_MQTT_BROKER_URL or MQTT_PROTOCOL+MQTT_HOST+MQTT_PORT")
	}
	tlsConfig, err := clientTLSConfig(cfg)
	if err != nil {
		return err
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBrokerURL).
		SetClientID(cfg.MQTTClientID).
		SetTLSConfig(tlsConfig).
		SetAutoReconnect(false).
		SetConnectRetry(false).
		SetCleanSession(true)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("timed out connecting to MQTT broker")
	} else if token.Error() != nil {
		return fmt.Errorf("connect to MQTT broker: %w", token.Error())
	}
	defer client.Disconnect(250)

	if token := client.Publish(cfg.MQTTControlTopic, 1, true, payload); !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("timed out publishing MQTT control message")
	} else if token.Error() != nil {
		return fmt.Errorf("publish MQTT control message: %w", token.Error())
	}

	fmt.Printf("sent %s (%q) to %s\n", command, payload, cfg.MQTTControlTopic)
	return nil
}

type importProgress struct {
	out     io.Writer
	total   int64
	current int64
	last    time.Time
	started time.Time
	done    bool
}

func newImportProgress(out io.Writer) *importProgress {
	return &importProgress{out: out, started: time.Now()}
}

func (p *importProgress) wrap(reader io.Reader, total int64) io.Reader {
	p.total = total
	return &progressReader{reader: reader, progress: p}
}

func (p *importProgress) add(n int) {
	if n <= 0 || p.done {
		return
	}
	p.current += int64(n)
	if time.Since(p.last) >= 500*time.Millisecond && p.current < p.total {
		p.print(false)
	}
}

func (p *importProgress) finish() {
	if p.done {
		return
	}
	p.print(true)
	p.done = true
}

func (p *importProgress) print(final bool) {
	p.last = time.Now()
	if p.total <= 0 {
		fmt.Fprintf(p.out, "\rUploaded %d bytes", p.current)
	} else {
		percent := float64(p.current) * 100 / float64(p.total)
		fmt.Fprintf(p.out, "\rUploaded %.0f%% (%d/%d bytes)", percent, p.current, p.total)
	}
	if final {
		fmt.Fprintf(p.out, " in %s\n", time.Since(p.started).Round(time.Millisecond))
	}
}

type progressReader struct {
	reader   io.Reader
	progress *importProgress
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.progress.add(n)
	if err == io.EOF {
		r.progress.finish()
	}
	return n, err
}

func gzipMDB(path string, progress *importProgress) (io.Reader, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open mdb: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("stat mdb: %w", err)
	}
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		return progress.wrap(file, info.Size()), func() { file.Close() }, nil
	}

	reader, writer := io.Pipe()
	go func() {
		gzipWriter := gzip.NewWriter(writer)
		_, copyErr := io.Copy(gzipWriter, progress.wrap(file, info.Size()))
		closeErr := gzipWriter.Close()
		file.Close()
		if copyErr != nil {
			writer.CloseWithError(copyErr)
			return
		}
		writer.CloseWithError(closeErr)
	}()
	return reader, func() { reader.Close() }, nil
}

func validateMDBPath(path string) error {
	name := strings.ToLower(filepath.Base(path))
	if !strings.HasSuffix(name, ".mdb") && !strings.HasSuffix(name, ".mdb.gz") {
		return fmt.Errorf("MDB path must end in .mdb or .mdb.gz")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("stat MDB path: %w", err)
	}
	return nil
}

func endpointURL(base, endpoint string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(endpoint, "/")
}

func uploadFilename(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".gz") {
		return name
	}
	return name + ".gz"
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: mobctl --file path/to/.env [upd [--mdb path/to/catalog.mdb] | ss | sw%s]\n", optionalUsage())
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
