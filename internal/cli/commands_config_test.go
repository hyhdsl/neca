// 本文件测试 commands_config 相关逻辑。
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigInitWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "neca.json")
	t.Setenv("NECA_CONFIG", path)
	code, out, errOut := runCLI("config", "init")
	if code != ExitOK || errOut != "" || !strings.Contains(out, "配置已写入") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"schemaVersion\": 2") {
		t.Fatalf("bad config: %s", data)
	}
	if code, _, _ := runCLI("config", "init"); code != ExitUsage {
		t.Fatalf("second init without --force should fail, got %d", code)
	}
	if code, out, errOut := runCLI("config", "init", "--force"); code != ExitOK {
		t.Fatalf("force init failed: code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestConfigMigrateCheckAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "neca.json")
	original := `{
  "schemaVersion": 1,
  "serverId": "vpn.example.com",
  "web": {"enabled": false}
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NECA_CONFIG", path)

	code, out, errOut := runCLI("config", "migrate", "--check")
	if code != ExitOK || errOut != "" || !strings.Contains(out, "可迁移") {
		t.Fatalf("check: code=%d out=%q err=%q", code, out, errOut)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `"schemaVersion": 1`) {
		t.Fatalf("check modified config: %s", data)
	}

	code, out, errOut = runCLI("config", "migrate", "--write")
	if code != ExitOK || errOut != "" || !strings.Contains(out, "已迁移") {
		t.Fatalf("write: code=%d out=%q err=%q", code, out, errOut)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schemaVersion": 2`) {
		t.Fatalf("migration did not write v2: %s", data)
	}
	backups, err := filepath.Glob(path + ".schema-v1-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, err=%v", backups, err)
	}
	if code, _, _ := runCLI("config", "migrate", "--check", "--write"); code != ExitUsage {
		t.Fatalf("conflicting flags must fail, got %d", code)
	}
}
