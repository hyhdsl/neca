// 本文件测试目录批量导入的发现、sidecar 与部分成功语义。
package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeBatchP12(t *testing.T, cfg *Config, batchRoot, passRoot, cn string) {
	t.Helper()
	_, req, _ := newImportFixture(t)
	dir := filepath.Join(batchRoot, cn)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p12, err := os.ReadFile(req.P12Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, batchFileP12), p12, 0o600); err != nil {
		t.Fatal(err)
	}
	pass, err := os.ReadFile(req.PasswordFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(passRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(passRoot, cn+".pass"), pass, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverBatchDirectoryAndSidecar(t *testing.T) {
	root := t.TempDir()
	passDir := t.TempDir()
	cn := "BATCH01"
	dir := filepath.Join(root, cn)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{batchFileP12, batchFileCert, batchFileKey} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("dummy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.Remove(filepath.Join(dir, batchFileCert))
	_ = os.Remove(filepath.Join(dir, batchFileKey))
	sidecar := `{"platforms":["linux"],"installers":"all","p12Profile":"modern","allowExpired":true}`
	if err := os.WriteFile(filepath.Join(dir, batchFileSidecar), []byte(sidecar), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(passDir, cn+".pass"), []byte("pass"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates, err := discoverBatch(BatchOptions{Dir: root, PasswordDir: passDir, Defaults: BatchDefaults{P12Profile: P12Compat}})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d", len(candidates))
	}
	req := candidates[0].req
	if req.Source != ImportSourceP12 || req.Platforms[0] != "linux" || req.P12Profile != P12Modern || !req.AllowExpired {
		t.Fatalf("unexpected request: %+v", req)
	}
	if req.Installers[PlatformWindows] != true || req.Installers[PlatformLinux] != true || req.Installers[PlatformMacOS] != true {
		t.Fatalf("installers = %+v", req.Installers)
	}
}

func TestDiscoverBatchRejectsMixedSources(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "MIXED")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{batchFileP12, batchFileCert, batchFileKey} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("dummy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := discoverBatch(BatchOptions{Dir: root, PasswordDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].err == "" {
		t.Fatalf("mixed sources should be rejected: %+v", candidates)
	}
}

func TestImportBatchPartialSuccess(t *testing.T) {
	cfg, _, store := newImportFixture(t)
	batchRoot := t.TempDir()
	passRoot := t.TempDir()
	writeBatchP12(t, cfg, batchRoot, passRoot, "TMP01")
	if err := os.MkdirAll(filepath.Join(batchRoot, "MISSING01"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(batchRoot, "MISSING01", batchFileP12), []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ImportBatch(context.Background(), cfg, store, NewOpenSSL(importFakeRunner(t, false)), BatchOptions{
		Dir: batchRoot, PasswordDir: passRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 || result.Failed != 1 || len(result.Entries) != 2 {
		t.Fatalf("result = %+v", result)
	}
	rows, err := store.Load(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("index = %+v, %v", rows, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.PKIDir, "TMP01.crt")); err != nil {
		t.Fatalf("successful entry missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.PKIDir, "MISSING01.crt")); !os.IsNotExist(err) {
		t.Fatalf("failed entry wrote cert: %v", err)
	}
}

func TestImportBatchDryRunWritesNothing(t *testing.T) {
	cfg, _, store := newImportFixture(t)
	batchRoot := t.TempDir()
	passRoot := t.TempDir()
	writeBatchP12(t, cfg, batchRoot, passRoot, "TMP01")
	result, err := ImportBatch(context.Background(), cfg, store, NewOpenSSL(importFakeRunner(t, false)), BatchOptions{
		Dir: batchRoot, PasswordDir: passRoot, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 || result.Entries[0].Status != "validated" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(cfg.PKIDir, "TMP01.crt")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote cert: %v", err)
	}
	rows, _ := store.Load(context.Background())
	if len(rows) != 0 {
		t.Fatalf("dry-run wrote index: %+v", rows)
	}
}
