// 本文件测试 audit 相关逻辑。
package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAuditWithOrigin(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{AuditFile: filepath.Join(dir, "audit.log")}
	if err := AppendAuditWithOrigin(cfg, "helper", "web-issue", "TMP02", "ABCDEF", "ok"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cfg.AuditFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "origin=helper web-issue TMP02 ABCDEF ok") {
		t.Fatalf("audit line = %q", string(data))
	}
}

func TestAppendAudit(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{AuditFile: filepath.Join(dir, "audit.log")}
	if err := AppendAudit(cfg, "issue", "TMP01", "ABCDEF", "ok"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cfg.AuditFile)
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	if !strings.Contains(line, "issue TMP01 ABCDEF ok") {
		t.Fatalf("audit line = %q", line)
	}
	info, err := os.Stat(cfg.AuditFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("audit mode = %o, want 600", info.Mode().Perm())
	}
}
