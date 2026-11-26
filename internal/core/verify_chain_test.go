// 本文件测试 p12 证书链校验与 verify 报告字段。
package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func argAfter(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func newVerifyFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	clientDir := filepath.Join(root, "clients", "TMP01")
	if err := os.MkdirAll(clientDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMP01.p12", "TMP01.p12.pass"} {
		if err := os.WriteFile(filepath.Join(clientDir, name), []byte("TestPass1234\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	caPath := filepath.Join(root, "ca.crt")
	if err := os.WriteFile(caPath, []byte("CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "clients"), caPath
}

func chainRunner(t *testing.T, verifyFails bool) fakeRunner {
	t.Helper()
	return fakeRunner{handler: func(args []string) (Result, error) {
		switch args[0] {
		case "pkcs12":
			if hasArg(args, "-info") {
				return Result{Stdout: "PKCS7 Encrypted data: pbeWithSHA1And3-KeyTripleDES-CBC\n"}, nil
			}
			if hasArg(args, "-out") {
				if err := os.WriteFile(argAfter(args, "-out"), []byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return Result{}, nil
			}
			return Result{Stdout: "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----\n"}, nil
		case "verify":
			if verifyFails {
				return Result{Stdout: "error 20 at 0 depth lookup: unable to get local issuer certificate\n"}, fmt.Errorf("openssl verify failed")
			}
			return Result{Stdout: "client.pem: OK\n"}, nil
		default:
			return Result{}, fmt.Errorf("unexpected command: %v", args)
		}
	}}
}

func TestVerifyClientsChainOK(t *testing.T) {
	clientsDir, caPath := newVerifyFixture(t)
	result, err := VerifyClients(context.Background(), &Config{ClientsDir: clientsDir, CACert: caPath}, NewOpenSSL(chainRunner(t, false)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(result.Reports))
	}
	report := result.Reports[0]
	if !report.ChainChecked || !report.ChainOK {
		t.Fatalf("链状态 = checked:%v ok:%v message:%q", report.ChainChecked, report.ChainOK, report.ChainMessage)
	}
	if !strings.Contains(report.OKMessage, "证书链 ✓") {
		t.Fatalf("OKMessage = %q", report.OKMessage)
	}
}

func TestVerifyClientsChainFailure(t *testing.T) {
	clientsDir, caPath := newVerifyFixture(t)
	result, err := VerifyClients(context.Background(), &Config{ClientsDir: clientsDir, CACert: caPath}, NewOpenSSL(chainRunner(t, true)), nil)
	if err != nil {
		t.Fatal(err)
	}
	report := result.Reports[0]
	if !report.ChainChecked || report.ChainOK {
		t.Fatalf("链状态 = checked:%v ok:%v message:%q", report.ChainChecked, report.ChainOK, report.ChainMessage)
	}
	if !strings.Contains(strings.Join(report.Warnings, " "), "证书链验证失败") {
		t.Fatalf("warnings = %v", report.Warnings)
	}
	if !strings.Contains(report.OKMessage, "证书链 ✗") {
		t.Fatalf("OKMessage = %q", report.OKMessage)
	}
}

func TestVerifyClientsMissingCA(t *testing.T) {
	clientsDir, _ := newVerifyFixture(t)
	result, err := VerifyClients(context.Background(), &Config{ClientsDir: clientsDir, CACert: filepath.Join(t.TempDir(), "missing.crt")}, NewOpenSSL(chainRunner(t, false)), nil)
	if err != nil {
		t.Fatal(err)
	}
	report := result.Reports[0]
	if report.ChainChecked {
		t.Fatal("CA 缺失时不应标记为已校验")
	}
	if !strings.Contains(strings.Join(report.Warnings, " "), "未校验证书链") {
		t.Fatalf("warnings = %v", report.Warnings)
	}
}
