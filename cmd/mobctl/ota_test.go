//go:build admin

package main

import (
	"bytes"
	"testing"
)

func TestOTATargetTopics(t *testing.T) {
	cfg := ctlConfig{
		QTermOTAControlTopic:   "qterm/control",
		QTermOTAChunkTopic:     "qterm/chunk",
		QTermOTAStatusTopic:    "qterm/status",
		BarcodeOTAControlTopic: "station/control",
		BarcodeOTAChunkTopic:   "station/chunk",
		BarcodeOTAStatusTopic:  "station/status",
	}

	qterm := qtermOTATarget()
	applyOTATopics(cfg, &qterm)
	if qterm.name != "qterm" || qterm.discoverDevices || qterm.controlTopic != "qterm/control" || qterm.chunkTopic != "qterm/chunk" || qterm.statusTopic != "qterm/status" {
		t.Fatalf("unexpected qterm target: %+v", qterm)
	}

	barcode := barcodeOTATarget()
	applyOTATopics(cfg, &barcode)
	if barcode.name != "barcode" || !barcode.discoverDevices || barcode.controlTopic != "station/control" || barcode.chunkTopic != "station/chunk" || barcode.statusTopic != "station/status" {
		t.Fatalf("unexpected barcode target: %+v", barcode)
	}
}

func TestOTAChunkFrame(t *testing.T) {
	session := []byte("0123456789abcdef0123456789abcdef")
	frame := otaChunkFrame(session, 0x01020304, []byte("abc"))

	if string(frame[:4]) != "QOTA" {
		t.Fatalf("bad magic: %q", frame[:4])
	}
	if string(frame[4:36]) != string(session) {
		t.Fatalf("bad session: %q", frame[4:36])
	}
	if got := []byte{frame[36], frame[37], frame[38], frame[39]}; !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("bad offset bytes: %v", got)
	}
	if got := []byte{frame[40], frame[41]}; !bytes.Equal(got, []byte{0, 3}) {
		t.Fatalf("bad len bytes: %v", got)
	}
	if string(frame[42:]) != "abc" {
		t.Fatalf("bad payload: %q", frame[42:])
	}
}
