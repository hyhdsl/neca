package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureLayoutPreservesDownloadsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not meaningful on Windows")
	}
	root := t.TempDir()
	pki := filepath.Join(root, "pki")
	if err := os.MkdirAll(pki, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pki, "ca.crt"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := filepath.Join(root, "downloads")
	if err := os.MkdirAll(downloads, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(downloads, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFrom(mapSource{"PKI_DIR": pki, "NECA_DOWNLOAD_DIR": downloads}); err != nil {
		t.Fatalf("LoadConfigFrom() error = %v", err)
	}
	info, err := os.Stat(downloads)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Fatalf("downloads mode = %o, want 750", got)
	}
}
