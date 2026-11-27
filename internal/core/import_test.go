// 本文件测试 p12 导入的事务、冲突和过期策略。
package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func importFakeRunner(t *testing.T, expired bool) fakeRunner {
	t.Helper()
	return fakeRunner{handler: func(args []string) (Result, error) {
		switch args[0] {
		case "pkcs12":
			out := importArgAfter(args, "-out")
			if out == "" {
				return Result{}, fmt.Errorf("pkcs12 missing -out: %v", args)
			}
			body := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
			if hasArg(args, "-nocerts") {
				body = "Bag Attributes\n-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"
			}
			if hasArg(args, "-export") {
				body = "P12"
			}
			if err := os.WriteFile(out, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			return Result{}, nil
		case "pkey":
			return Result{Stdout: "-----BEGIN PUBLIC KEY-----\nABC\n-----END PUBLIC KEY-----\n"}, nil
		case "x509":
			switch {
			case hasArg(args, "-text"):
				return Result{Stdout: "X509v3 Basic Constraints: critical\n    CA:FALSE\nX509v3 Extended Key Usage: critical\n    TLS Web Client Authentication\n"}, nil
			case hasArg(args, "-nameopt"):
				return Result{Stdout: "subject=\n    C=CN\n    O=ExampleOrg\n    CN=TMP01\n"}, nil
			case hasArg(args, "-subject"):
				return Result{Stdout: "subject=C = CN, O = ExampleOrg, CN = TMP01\n"}, nil
			case hasArg(args, "-serial"):
				return Result{Stdout: "serial=ABC123\n"}, nil
			case hasArg(args, "-startdate"):
				return Result{Stdout: "notBefore=Nov 26 00:00:00 2026 GMT\n"}, nil
			case hasArg(args, "-enddate"):
				date := "Nov 27 00:00:00 2027 GMT"
				if expired {
					date = "Nov 26 00:00:00 2026 GMT"
				}
				return Result{Stdout: "notAfter=" + date + "\n"}, nil
			case hasArg(args, "-pubkey"):
				return Result{Stdout: "-----BEGIN PUBLIC KEY-----\nABC\n-----END PUBLIC KEY-----\n"}, nil
			}
		case "verify":
			return Result{Stdout: "client.crt: OK\n"}, nil
		}
		return Result{}, fmt.Errorf("unexpected openssl command: %v", args)
	}}
}

func importArgAfter(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func newImportFixture(t *testing.T) (*Config, ImportRequest, *FileIndexStore) {
	t.Helper()
	root := t.TempDir()
	pki := filepath.Join(root, "pki")
	clients := filepath.Join(root, "clients")
	meta := filepath.Join(root, "meta")
	for _, dir := range []string{pki, clients, meta} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ca := filepath.Join(pki, "ca.crt")
	if err := os.WriteFile(ca, []byte("CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	p12 := filepath.Join(root, "input.p12")
	pass := filepath.Join(root, "input.p12.pass")
	if err := os.WriteFile(p12, []byte("P12"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pass, []byte("Secret1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		PKIDir: pki, ClientsDir: clients, MetaDir: meta, CACert: ca,
		IndexFile: filepath.Join(pki, "index.txt"), LockFile: filepath.Join(pki, ".lock"),
		CRLFile: filepath.Join(root, "crl.pem"), P12Profile: P12Compat,
		ServerID: "vpn.example", CACN: "Example CA",
	}
	store := NewFileIndexStore(cfg.IndexFile, cfg.LockFile)
	req := ImportRequest{CN: "TMP01", Source: ImportSourceP12, P12Path: p12, PasswordFile: pass, P12Profile: P12Compat}
	return cfg, req, store
}

func TestImportCertificateCreatesManagedCertificate(t *testing.T) {
	cfg, req, store := newImportFixture(t)
	oss := NewOpenSSL(importFakeRunner(t, false))
	result, err := ImportCertificate(context.Background(), cfg, store, oss, req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IndexAdded || !result.MetaWritten || result.Expired {
		t.Fatalf("unexpected result: %+v", result)
	}
	rows, err := store.Load(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Status != "V" || rows[0].Serial != "ABC123" {
		t.Fatalf("index = %+v, %v", rows, err)
	}
	meta, err := LoadMeta(MetaPath(cfg.MetaDir, "TMP01"))
	if err != nil || meta == nil {
		t.Fatalf("meta = %v, %v", meta, err)
	}
	if meta.Get("NOT_BEFORE") != "Nov 26 00:00:00 2026 GMT" || meta.Get("IMPORT_SOURCE") != "P12" {
		t.Fatalf("meta = %+v", meta)
	}
	for _, path := range []string{
		filepath.Join(cfg.PKIDir, "TMP01.crt"), filepath.Join(cfg.PKIDir, "TMP01.key"),
		filepath.Join(cfg.ClientsDir, "TMP01", "TMP01.p12"), filepath.Join(cfg.ClientsDir, "TMP01", "TMP01.p12.pass"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
}

func TestImportCertificateRejectsExpiredWithoutOverride(t *testing.T) {
	cfg, req, store := newImportFixture(t)
	oss := NewOpenSSL(importFakeRunner(t, true))
	if _, err := ImportCertificate(context.Background(), cfg, store, oss, req); err == nil {
		t.Fatal("expired import should be rejected")
	}
	if _, err := os.Stat(filepath.Join(cfg.PKIDir, "TMP01.crt")); !os.IsNotExist(err) {
		t.Fatalf("expired import left certificate: %v", err)
	}
}

func TestImportCertificateExpiredOverrideMarksExpired(t *testing.T) {
	cfg, req, store := newImportFixture(t)
	req.AllowExpired = true
	result, err := ImportCertificate(context.Background(), cfg, store, NewOpenSSL(importFakeRunner(t, true)), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Expired || !strings.Contains(strings.Join(result.NextActions, " "), "reissue") {
		t.Fatalf("result = %+v", result)
	}
	rows, _ := store.Load(context.Background())
	if len(rows) != 1 || rows[0].Status != "E" {
		t.Fatalf("index = %+v", rows)
	}
}

func TestImportCertificateRejectsExistingTarget(t *testing.T) {
	cfg, req, store := newImportFixture(t)
	if err := os.WriteFile(filepath.Join(cfg.PKIDir, "TMP01.crt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportCertificate(context.Background(), cfg, store, NewOpenSSL(importFakeRunner(t, false)), req); err == nil {
		t.Fatal("existing target should be rejected")
	}
}

func TestImportDryRunDoesNotWrite(t *testing.T) {
	cfg, req, store := newImportFixture(t)
	req.DryRun = true
	result, err := ImportCertificate(context.Background(), cfg, store, NewOpenSSL(importFakeRunner(t, false)), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.IndexAdded || result.MetaWritten {
		t.Fatalf("dry-run result = %+v", result)
	}
	rows, _ := store.Load(context.Background())
	if len(rows) != 0 {
		t.Fatalf("dry-run wrote index: %+v", rows)
	}
}
