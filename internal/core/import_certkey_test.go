// 本文件测试 cert+key 导入和事务恢复。
package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hyhdsl/neca/internal/core/ports"
)

func certKeyFakeRunner(t *testing.T) fakeRunner {
	t.Helper()
	base := importFakeRunner(t, false)
	return fakeRunner{handler: func(args []string) (Result, error) {
		if len(args) == 0 {
			return Result{}, fmt.Errorf("empty openssl command")
		}
		if args[0] == "pkey" {
			if hasArg(args, "-text") {
				return Result{Stdout: "Private-Key: (3072 bit, 2 primes)\n"}, nil
			}
			if hasArg(args, "-out") {
				out := importArgAfter(args, "-out")
				if out == "" {
					return Result{}, fmt.Errorf("pkey missing -out")
				}
				body := "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"
				return Result{}, os.WriteFile(out, []byte(body), 0o600)
			}
		}
		return base.handler(args)
	}}
}

func newCertKeyFixture(t *testing.T, encrypted bool) (*Config, ImportRequest, *FileIndexStore) {
	t.Helper()
	cfg, _, store := newImportFixture(t)
	root := filepath.Dir(cfg.PKIDir)
	certPath := filepath.Join(root, "input.crt")
	keyPath := filepath.Join(root, "input.key")
	passPath := filepath.Join(root, "input.key.pass")
	if err := os.WriteFile(certPath, []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keyBody := "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"
	if encrypted {
		keyBody = "-----BEGIN ENCRYPTED PRIVATE KEY-----\nMIIB\n-----END ENCRYPTED PRIVATE KEY-----\n"
	}
	if err := os.WriteFile(keyPath, []byte(keyBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passPath, []byte("Secret1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := ImportRequest{
		CN: "TMP01", Source: ImportSourceCertKey,
		CertPath: certPath, KeyPath: keyPath, P12Profile: P12Compat,
	}
	if encrypted {
		req.KeyPasswordFile = passPath
	}
	return cfg, req, store
}

func TestImportCertKeyCreatesManagedCertificate(t *testing.T) {
	cfg, req, store := newCertKeyFixture(t, false)
	result, err := ImportCertificate(context.Background(), cfg, store, NewOpenSSL(certKeyFakeRunner(t)), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IndexAdded || !result.MetaWritten || result.Source != ImportSourceCertKey {
		t.Fatalf("unexpected result: %+v", result)
	}
	rows, err := store.Load(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Serial != "ABC123" {
		t.Fatalf("index = %+v, %v", rows, err)
	}
	meta, err := LoadMeta(MetaPath(cfg.MetaDir, "TMP01"))
	if err != nil || meta.Get("IMPORT_SOURCE") != "CERT-KEY" {
		t.Fatalf("meta = %+v, %v", meta, err)
	}
	keyInfo, err := os.Stat(filepath.Join(cfg.PKIDir, "TMP01.key"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v", keyInfo.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(cfg.ClientsDir, "TMP01", "TMP01.p12")); err != nil {
		t.Fatal(err)
	}
	entries, _ := filepath.Glob(filepath.Join(cfg.PKIDir, ".txn", "import-*.json"))
	if len(entries) != 0 {
		t.Fatalf("transaction marker left behind: %v", entries)
	}
}

func TestImportEncryptedCertKey(t *testing.T) {
	cfg, req, store := newCertKeyFixture(t, true)
	result, err := ImportCertificate(context.Background(), cfg, store, NewOpenSSL(certKeyFakeRunner(t)), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IndexAdded {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestImportEncryptedCertKeyRequiresPasswordFile(t *testing.T) {
	cfg, req, store := newCertKeyFixture(t, true)
	req.KeyPasswordFile = ""
	if _, err := ImportCertificate(context.Background(), cfg, store, NewOpenSSL(certKeyFakeRunner(t)), req); err == nil {
		t.Fatal("encrypted key without password should fail")
	}
	if _, err := os.Stat(filepath.Join(cfg.PKIDir, "TMP01.key")); !os.IsNotExist(err) {
		t.Fatalf("failed import left a key: %v", err)
	}
}

func TestNormalizeImportRejectsMixedSources(t *testing.T) {
	_, req, _ := newCertKeyFixture(t, false)
	req.P12Path = "input.p12"
	req.PasswordFile = "input.p12.pass"
	if err := normalizeImportRequest(&Config{}, &req); err == nil {
		t.Fatal("mixed p12 + cert/key should fail")
	}
}

func TestValidatePrivateKeyRejectsUnsupportedAlgorithms(t *testing.T) {
	oss := NewOpenSSL(fakeRunner{handler: func(args []string) (Result, error) {
		return Result{Stdout: "ED25519 Private-Key:\n"}, nil
	}})
	if err := oss.ValidatePrivateKey(context.Background(), "key.pem"); err == nil {
		t.Fatal("ed25519 should be rejected")
	}
	oss = NewOpenSSL(fakeRunner{handler: func(args []string) (Result, error) {
		return Result{Stdout: "Private-Key: (1024 bit, 2 primes)\n"}, nil
	}})
	if err := oss.ValidatePrivateKey(context.Background(), "key.pem"); err == nil {
		t.Fatal("1024-bit RSA should be rejected")
	}
	oss = NewOpenSSL(fakeRunner{handler: func(args []string) (Result, error) {
		return Result{Stdout: "Private-Key: (256 bit)\nASN1 OID: prime256v1\n"}, nil
	}})
	if err := oss.ValidatePrivateKey(context.Background(), "key.pem"); err != nil {
		t.Fatalf("P-256 should be accepted: %v", err)
	}
}

func TestRecoverUncommittedImportTransaction(t *testing.T) {
	cfg, _, store := newCertKeyFixture(t, false)
	if err := os.MkdirAll(filepath.Join(cfg.ClientsDir, "TMP01"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(cfg.PKIDir, "TMP01.crt"),
		filepath.Join(cfg.PKIDir, "TMP01.key"),
		filepath.Join(cfg.ClientsDir, "TMP01", "TMP01.p12"),
		MetaPath(cfg.MetaDir, "TMP01"),
	} {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpdateLocked(context.Background(), func(idx *ports.Index) error {
		idx.Rows = append(idx.Rows, ports.Row{Status: ports.IndexValid, Serial: "ABC123", Expires: "2028-01-01", Subject: "CN=TMP01"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	txn, err := beginImportTxn(cfg, "TMP01", "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if err := txn.setPhase("index"); err != nil {
		t.Fatal(err)
	}
	if err := RecoverImportTransactions(context.Background(), cfg, store); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(cfg.PKIDir, "TMP01.crt"),
		filepath.Join(cfg.PKIDir, "TMP01.key"),
		filepath.Join(cfg.ClientsDir, "TMP01"),
		MetaPath(cfg.MetaDir, "TMP01"),
		txn.path,
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("recovery left %s: %v", path, err)
		}
	}
	rows, _ := store.Load(context.Background())
	if len(rows) != 0 {
		t.Fatalf("recovery left index row: %+v", rows)
	}
}

func TestRecoverCommittedImportTransactionKeepsFiles(t *testing.T) {
	cfg, _, store := newCertKeyFixture(t, false)
	certPath := filepath.Join(cfg.PKIDir, "TMP01.crt")
	if err := os.WriteFile(certPath, []byte("committed"), 0o644); err != nil {
		t.Fatal(err)
	}
	txn, err := beginImportTxn(cfg, "TMP01", "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if err := txn.commit(); err != nil {
		t.Fatal(err)
	}
	if err := RecoverImportTransactions(context.Background(), cfg, store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("committed cert was removed: %v", err)
	}
	if _, err := os.Stat(txn.path); !os.IsNotExist(err) {
		t.Fatalf("committed marker was not cleaned: %v", err)
	}
}

func TestImportCertKeyRejectsExtraCertificateBlocks(t *testing.T) {
	cfg, req, store := newCertKeyFixture(t, false)
	data, err := os.ReadFile(req.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")...)
	if err := os.WriteFile(req.CertPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, importErr := ImportCertificate(context.Background(), cfg, store, NewOpenSSL(certKeyFakeRunner(t)), req); importErr == nil {
		t.Fatal("multiple certificates should be rejected")
	} else if !strings.Contains(importErr.Error(), "CERTIFICATE") {
		t.Fatalf("unexpected error: %v", importErr)
	}
}
