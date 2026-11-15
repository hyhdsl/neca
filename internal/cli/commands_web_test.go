package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupWebCLI(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	pki := filepath.Join(root, "pki")
	if err := os.MkdirAll(pki, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pki, "ca.crt"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "neca.json")
	payload := `{
  "schemaVersion": 1,
  "serverId": "test.example.com",
  "web": {
    "enabled": true,
    "listen": "127.0.0.1:18080",
    "readOnly": true,
    "idleTimeout": "30m"
  }
}`
	if err := os.WriteFile(configPath, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NECA_CONFIG", configPath)
	t.Setenv("PKI_DIR", pki)
	return configPath, pki
}

func runCLIWithInput(args []string, input string) (int, string, string) {
	var out, errBuf bytes.Buffer
	code := RunWithIO(args, strings.NewReader(input), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestWebStatusAndReadOnly(t *testing.T) {
	setupWebCLI(t)
	code, out, errOut := runCLI("web", "status")
	if code != ExitOK || errOut != "" {
		t.Fatalf("web status: code=%d out=%q err=%q", code, out, errOut)
	}
	if !strings.Contains(out, "只读") || !strings.Contains(out, "127.0.0.1:18080") {
		t.Fatalf("unexpected status: %q", out)
	}

	if os.Geteuid() != 0 {
		t.Skip("readonly on/off requires root")
	}
	code, out, _ = runCLI("web", "readonly", "on")
	if code != ExitOK || !strings.Contains(out, "只读") {
		t.Fatalf("readonly on: code=%d out=%q", code, out)
	}
	code, _, errOut = runCLI("web", "readonly", "off")
	if code != ExitPrecondition {
		t.Fatalf("readonly off must be rejected in M6a: code=%d err=%q", code, errOut)
	}
	if !strings.Contains(errOut, "M6a") {
		t.Fatalf("off error should explain M6a: %q", errOut)
	}
}

func TestWebIdleTimeout(t *testing.T) {
	setupWebCLI(t)
	code, out, _ := runCLI("web", "idle-timeout", "status")
	if code != ExitOK || !strings.Contains(out, "30m0s") {
		t.Fatalf("idle status: code=%d out=%q", code, out)
	}
	if os.Geteuid() != 0 {
		t.Skip("idle-timeout update requires root")
	}
	code, out, _ = runCLI("web", "idle-timeout", "10m")
	if code != ExitOK || !strings.Contains(out, "10m0s") {
		t.Fatalf("idle update: code=%d out=%q", code, out)
	}
	code, _, _ = runCLI("web", "idle-timeout", "1m")
	if code != ExitUsage {
		t.Fatalf("invalid idle timeout code=%d, want %d", code, ExitUsage)
	}
	code, _, _ = runCLI("web", "idle-timeout", "25h")
	if code != ExitUsage {
		t.Fatalf("too large idle timeout code=%d, want %d", code, ExitUsage)
	}
}

func TestWebHashPassword(t *testing.T) {
	code, out, errOut := runCLIWithInput([]string{"web", "hash-password"}, "secret123\n")
	if code != ExitOK || errOut != "" {
		t.Fatalf("hash-password: code=%d out=%q err=%q", code, out, errOut)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "$2") {
		t.Fatalf("not a bcrypt hash: %q", out)
	}
	code, _, _ = runCLIWithInput([]string{"web", "hash-password"}, "short\n")
	if code != ExitUsage {
		t.Fatalf("short password code=%d, want %d", code, ExitUsage)
	}
}
