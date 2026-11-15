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
	if !strings.Contains(string(data), "\"schemaVersion\": 1") {
		t.Fatalf("bad config: %s", data)
	}
	if code, _, _ := runCLI("config", "init"); code != ExitUsage {
		t.Fatalf("second init without --force should fail, got %d", code)
	}
	if code, _, _ := runCLI("config", "init", "--force"); code != ExitOK {
		t.Fatalf("force init failed: %d", code)
	}
}
