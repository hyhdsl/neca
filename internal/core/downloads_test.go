package core

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAndCleanDownloadZip(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{
		ClientsDir:       filepath.Join(root, "clients"),
		MetaDir:          filepath.Join(root, "meta"),
		DownloadsDir:     filepath.Join(root, "downloads"),
		DownloadTTL:      time.Minute,
		DownloadMaxFiles: 2,
		P12Profile:       P12Compat,
	}
	cn := "TMP01"
	dir := filepath.Join(cfg.ClientsDir, cn, "Windows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{cn + ".p12", cn + ".p12.pass", "ca.crt"} {
		if err := os.WriteFile(filepath.Join(cfg.ClientsDir, cn, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "使用说明.txt"), []byte("说明"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := CreateDownloadZip(cfg, cn, []string{PlatformWindows})
	if err != nil {
		t.Fatalf("CreateDownloadZip error = %v", err)
	}
	if _, err := os.Stat(entry.Path); err != nil {
		t.Fatalf("zip 不存在: %v", err)
	}
	if _, err := os.Stat(entry.Path + ".json"); err != nil {
		t.Fatalf("sidecar 不存在: %v", err)
	}
	reader, err := zip.OpenReader(entry.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, want := range []string{cn + "/MANIFEST.txt", cn + "/" + cn + ".p12", cn + "/Windows/使用说明.txt"} {
		if !names[want] {
			t.Fatalf("zip 缺少 %s，实际 %v", want, names)
		}
	}

	// 过期后 CleanDownloads 应删除。
	entry.ExpiresAt = time.Now().Add(-time.Minute)
	if err := writeDownloadMeta(entry); err != nil {
		t.Fatal(err)
	}
	removed, err := CleanDownloads(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("清理数量 = %d, want 1", removed)
	}
}
