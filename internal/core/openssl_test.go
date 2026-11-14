package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeRunner 按参数返回预设输出，避免测试依赖真实 openssl。
type fakeRunner struct {
	handler func(args []string) (Result, error)
}

func (f fakeRunner) Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error) {
	return f.handler(args)
}

// TestSerial 校验去掉 serial= 前缀并转大写。
func TestSerial(t *testing.T) {
	oss := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{Stdout: "serial=ab12cd\n"}, nil
	}})
	got, err := oss.Serial(context.Background(), "/tmp/a.crt")
	if err != nil || got != "AB12CD" {
		t.Fatalf("Serial() = %q, %v", got, err)
	}
}

// TestCN 校验 sep_multiline subject 解析（开发文档 §8.2）。
func TestCN(t *testing.T) {
	oss := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{Stdout: "subject=\n    C=CN\n    O=NekoBigLazyer\n    CN=TMP01\n"}, nil
	}})
	got, err := oss.CN(context.Background(), "/tmp/a.crt")
	if err != nil || got != "TMP01" {
		t.Fatalf("CN() = %q, %v", got, err)
	}
}

// TestSubject 校验保留默认格式（index.txt subject 列依赖它）。
func TestSubject(t *testing.T) {
	oss := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{Stdout: "subject=C = CN, O = NekoBigLazyer, CN = TMP01\n"}, nil
	}})
	got, err := oss.Subject(context.Background(), "/tmp/a.crt")
	if err != nil || got != "C = CN, O = NekoBigLazyer, CN = TMP01" {
		t.Fatalf("Subject() = %q, %v", got, err)
	}
}

// TestEndDate 校验原始文本与时间解析。
func TestEndDate(t *testing.T) {
	oss := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{Stdout: "notAfter=Nov 14 20:06:48 2027 GMT\n"}, nil
	}})
	got, raw, err := oss.EndDate(context.Background(), "/tmp/a.crt")
	if err != nil {
		t.Fatalf("EndDate() error = %v", err)
	}
	want := time.Date(2027, 11, 14, 20, 6, 48, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("EndDate() = %v, want %v", got, want)
	}
	if raw != "Nov 14 20:06:48 2027 GMT" {
		t.Fatalf("raw = %q", raw)
	}
}

// TestSAN 校验去空格并以逗号连接。
func TestSAN(t *testing.T) {
	oss := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{Stdout: "X509v3 Subject Alternative Name: \n    DNS:vpn.example.work, DNS:TMP01\n"}, nil
	}})
	got, err := oss.SAN(context.Background(), "/tmp/a.crt")
	if err != nil || got != "DNS:vpn.example.work,DNS:TMP01" {
		t.Fatalf("SAN() = %q, %v", got, err)
	}
}

// TestCRLRevokedCount 校验统计 "Serial Number" 且失败时返回 0。
func TestCRLRevokedCount(t *testing.T) {
	oss := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{Stdout: "Serial Number: 1\nSerial Number: 2\n"}, nil
	}})
	got, _ := oss.CRLRevokedCount(context.Background(), "/tmp/crl.pem")
	if got != 2 {
		t.Fatalf("CRLRevokedCount() = %d, want 2", got)
	}

	failing := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{}, context.DeadlineExceeded
	}})
	if got, _ := failing.CRLRevokedCount(context.Background(), "/tmp/none.pem"); got != 0 {
		t.Fatalf("失败时计数 = %d, want 0", got)
	}
}

// TestPKCS12InfoTruncates 校验 info 最多返回 8 行且保留 3DES 标记。
func TestPKCS12InfoTruncates(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line"
	}
	lines[1] = "PKCS7 Encrypted data: pbeWithSHA1And3-KeyTripleDES-CBC"
	oss := NewOpenSSL(fakeRunner{handler: func([]string) (Result, error) {
		return Result{Stdout: strings.Join(lines, "\n")}, nil
	}})
	got, err := oss.PKCS12Info(context.Background(), "/tmp/a.p12", "/tmp/a.pass")
	if err != nil {
		t.Fatalf("PKCS12Info() error = %v", err)
	}
	if lines := strings.Count(got, "\n") + 1; lines != 8 {
		t.Fatalf("行数 = %d, want 8", lines)
	}
	if !strings.Contains(got, "pbeWithSHA1And3-KeyTripleDES-CBC") {
		t.Fatal("info 应保留 3DES 标记")
	}
}

// TestIndexExpiry 校验 index.txt 时间格式（UTC）。
func TestIndexExpiry(t *testing.T) {
	tm := time.Date(2027, 11, 14, 20, 6, 48, 0, time.UTC)
	if got := IndexExpiry(tm); got != "271114200648Z" {
		t.Fatalf("IndexExpiry() = %q", got)
	}
}
