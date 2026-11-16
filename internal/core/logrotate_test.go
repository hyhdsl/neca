package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RotateLog(path, 5, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original still exists: %v", err)
	}
	data, err := os.ReadFile(path + ".1")
	if err != nil || string(data) != "0123456789" {
		t.Fatalf("rotated = %q, %v", data, err)
	}
	if err := RotateLog(path+".1", 0, 2); err != nil {
		t.Fatal(err)
	}
}

func TestRotatingFileWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web-audit.log")
	w := NewRotatingFileWriter(path, 24, 2)
	if _, err := w.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("abcdefghij")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("ABCDEFGHIJ")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ABCDEFGHIJ") {
		t.Fatalf("current log = %q", data)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
}
