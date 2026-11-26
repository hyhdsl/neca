// 本文件测试 bundle_scripts 相关逻辑。
package core

import (
	"context"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRenderUnixScripts(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{
		ClientsDir: filepath.Join(root, "clients"),
		MetaDir:    filepath.Join(root, "meta"),
		CACN:       "ExampleOrg VPN CA",
		ServerID:   "vpn.example.com",
	}
	cn := "TMP01"
	base := filepath.Join(cfg.ClientsDir, cn)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{cn + ".p12", cn + ".p12.pass", "ca.crt"} {
		content := []byte("dummy")
		if name == "ca.crt" {
			content = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("dummy-cert")})
		}
		if err := os.WriteFile(filepath.Join(base, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	platforms := []string{PlatformLinux, PlatformMacOS}
	result, err := RenderBundles(context.Background(), cfg, cn, platforms, InstallerSet{PlatformLinux: true, PlatformMacOS: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Platforms) != 2 {
		t.Fatalf("platforms = %v", result.Platforms)
	}
	for _, name := range []string{
		"Linux/install.sh", "Linux/uninstall.sh",
		"macOS/install.command", "macOS/uninstall.command",
	} {
		path := filepath.Join(base, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s is not executable", name)
		}
		if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
			t.Fatalf("bash -n %s: %v\n%s", name, err, out)
		}
	}
}
