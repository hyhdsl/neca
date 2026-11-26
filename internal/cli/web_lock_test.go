// 本文件测试 web_lock 相关逻辑。
package cli

import (
	"os"
	"strings"
	"testing"
)

func TestWebLockSubcommand(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("web lock requires root")
	}
	configPath, _ := setupWebCLI(t)
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	// Avoid invoking a Web service in the test; only exercise config mutation.
	raw = []byte(strings.Replace(string(raw), "\"enabled\": true", "\"enabled\": false", 1))
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLIWithInput([]string{"web", "lock"}, "y\n")
	if code != ExitOK {
		t.Fatalf("web lock: code=%d out=%q err=%q", code, out, errOut)
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "\"readOnly\": true") || !strings.Contains(string(updated), "\"sessionEpoch\": 1") {
		t.Fatalf("web lock did not update config: %s", updated)
	}
}
