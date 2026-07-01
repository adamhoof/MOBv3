package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRequiredTrimsAndRejectsMissing(t *testing.T) {
	t.Setenv("REQUIRED_VALUE", "  yes  ")
	value, err := Required("REQUIRED_VALUE")
	if err != nil {
		t.Fatal(err)
	}
	if value != "yes" {
		t.Fatalf("unexpected value %q", value)
	}

	if _, err := Required("MISSING_VALUE"); err == nil {
		t.Fatal("expected missing env error")
	}
}

func TestRequiredAnyUsesFirstPresentValue(t *testing.T) {
	t.Setenv("SECOND_VALUE", "two")
	value, err := RequiredAny("FIRST_VALUE", "SECOND_VALUE")
	if err != nil {
		t.Fatal(err)
	}
	if value != "two" {
		t.Fatalf("unexpected value %q", value)
	}
}

func TestOptionalParsers(t *testing.T) {
	t.Setenv("INT_VALUE", "42")
	t.Setenv("INT64_VALUE", "420")
	t.Setenv("DURATION_VALUE", "3s")

	if value, err := OptionalInt("INT_VALUE", 1); err != nil || value != 42 {
		t.Fatalf("OptionalInt=%d err=%v", value, err)
	}
	if value, err := OptionalInt64("INT64_VALUE", 1); err != nil || value != 420 {
		t.Fatalf("OptionalInt64=%d err=%v", value, err)
	}
	if value, err := OptionalDuration("DURATION_VALUE", time.Second); err != nil || value != 3*time.Second {
		t.Fatalf("OptionalDuration=%s err=%v", value, err)
	}
}

func TestLoadFileSetsEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\nA=one\nB='two'\nC=\"three\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := LoadFile(path); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"A": "one", "B": "two", "C": "three"} {
		if got := os.Getenv(key); got != want {
			t.Fatalf("%s=%q, want %q", key, got, want)
		}
	}
}

func TestLoadFileRejectsMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("not-valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(path); err == nil {
		t.Fatal("expected malformed line error")
	}
}
