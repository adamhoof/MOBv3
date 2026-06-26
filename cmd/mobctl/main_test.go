package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEndpointURLJoinsCleanly(t *testing.T) {
	got := endpointURL("https://server/", "/catalog/import")
	if got != "https://server/catalog/import" {
		t.Fatalf("got %q", got)
	}
}

func TestUploadFilenameAddsGzipSuffix(t *testing.T) {
	if got := uploadFilename("catalog.mdb"); got != "catalog.mdb.gz" {
		t.Fatalf("got %q", got)
	}
	if got := uploadFilename("catalog.mdb.gz"); got != "catalog.mdb.gz" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateMDBPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.mdb")
	if err := os.WriteFile(path, []byte("mdb"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateMDBPath(path); err != nil {
		t.Fatal(err)
	}

	badPath := filepath.Join(t.TempDir(), "catalog.csv")
	if err := os.WriteFile(badPath, []byte("csv"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateMDBPath(badPath); err == nil {
		t.Fatal("expected extension error")
	}
}

func TestGzipMDBCompressesPlainMDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.mdb")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	reader, closeReader, err := gzipMDB(path, newImportProgress(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	defer closeReader()

	gz, err := gzip.NewReader(reader)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q", string(data))
	}
}

func TestImportProgressPrintsCompletion(t *testing.T) {
	var out bytes.Buffer
	progress := newImportProgress(&out)
	reader := progress.wrap(strings.NewReader("hello"), 5)

	if _, err := io.ReadAll(reader); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Uploaded 100% (5/5 bytes)") {
		t.Fatalf("progress output = %q", out.String())
	}
}

func TestLoadConfigUsesSharedTimeout(t *testing.T) {
	t.Setenv("MOBCTL_SERVER_URL", "https://example.local:8443")
	t.Setenv("CATALOG_IMPORT_TIMEOUT", "45s")
	t.Setenv("TLS_CA_PATH", "/ca.crt")
	t.Setenv("TLS_CLIENT_CERT_PATH", "/client.crt")
	t.Setenv("TLS_CLIENT_KEY_PATH", "/client.key")

	cfg, err := loadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Timeout != 45*time.Second {
		t.Fatalf("timeout=%s", cfg.Timeout)
	}
}

func TestLoadConfigRejectsWildcardHostWithoutClientURL(t *testing.T) {
	t.Setenv("CATALOG_HTTP_HOST", "0.0.0.0")
	t.Setenv("CATALOG_HTTP_PORT", "8443")
	t.Setenv("TLS_CA_PATH", "/ca.crt")
	t.Setenv("TLS_CLIENT_CERT_PATH", "/client.crt")
	t.Setenv("TLS_CLIENT_KEY_PATH", "/client.key")

	if _, err := loadConfig(t.TempDir()); err == nil {
		t.Fatal("expected wildcard host error")
	}
}

func TestResolveConfigPath(t *testing.T) {
	dir := filepath.Join("tmp", "cfg")
	if got := resolveConfigPath(dir, "certs/ca.crt"); got != filepath.Join(dir, "certs/ca.crt") {
		t.Fatalf("relative path=%q", got)
	}
	abs := filepath.Join(string(filepath.Separator), "certs", "ca.crt")
	if got := resolveConfigPath(dir, abs); got != abs {
		t.Fatalf("absolute path=%q", got)
	}
}
