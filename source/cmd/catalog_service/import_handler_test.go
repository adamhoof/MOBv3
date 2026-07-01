package main

import (
	"os"
	"strings"
	"testing"
)

func TestCommandOutputTrimsAndLimits(t *testing.T) {
	if got := commandOutput([]byte("  failed\n")); got != "failed" {
		t.Fatalf("got %q", got)
	}
	if got := commandOutput(nil); got != "no command output" {
		t.Fatalf("got %q", got)
	}
}

func TestMDBConverterPathUsesEnvOverride(t *testing.T) {
	t.Setenv("MDB_CONVERTER_PATH", "/custom/convert.sh")
	if got := mdbConverterPath(); got != "/custom/convert.sh" {
		t.Fatalf("got %q", got)
	}
}

func TestSaveUploadWritesTempFile(t *testing.T) {
	path, err := saveUpload(strings.NewReader("upload"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "upload" {
		t.Fatalf("got %q", string(data))
	}
}

func TestMDBConverterDefaultPath(t *testing.T) {
	t.Setenv("MDB_CONVERTER_PATH", "")
	if got := mdbConverterPath(); got != "./mdb_to_csv.sh" {
		t.Fatalf("got %q", got)
	}
}
