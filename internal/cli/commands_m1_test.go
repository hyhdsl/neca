// 本文件测试 commands_m1 相关逻辑。
package cli

import (
	"strings"
	"testing"

	"github.com/hyhdsl/neca/internal/core"
)

// TestParseExportArgs 校验 export 参数组合与错误。
func TestParseExportArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantKind core.ExportKind
		wantOut  string
		wantErr  bool
	}{
		{"issued", []string{"issued"}, core.ExportIssued, "", false},
		{"revoked with out", []string{"revoked", "--out", "/tmp/a.csv"}, core.ExportRevoked, "/tmp/a.csv", false},
		{"out first", []string{"--out", "/tmp/b.csv", "issued"}, core.ExportIssued, "/tmp/b.csv", false},
		{"missing type", []string{"--out", "/tmp/a.csv"}, "", "", true},
		{"duplicate type", []string{"issued", "revoked"}, "", "", true},
		{"missing out value", []string{"issued", "--out"}, "", "", true},
		{"unknown arg", []string{"issued", "extra"}, "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, out, err := parseExportArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("应当报错")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseExportArgs() error = %v", err)
			}
			if kind != tc.wantKind || out != tc.wantOut {
				t.Fatalf("got (%q, %q), want (%q, %q)", kind, out, tc.wantKind, tc.wantOut)
			}
		})
	}
}

// TestBuiltinRegistryIncludesNamespaces 校验 v0.5 顶层命名空间已注册。
func TestBuiltinRegistryIncludesNamespaces(t *testing.T) {
	registry := newBuiltinRegistry()
	for _, name := range []string{"cert", "pki", "trash", "config", "web"} {
		if _, ok := registry.Lookup(name); !ok {
			t.Fatalf("缺少命名空间 %q", name)
		}
	}
}

// TestListRejectsExtraArgs 校验 list 参数校验发生在加载配置之前。
func TestListRejectsExtraArgs(t *testing.T) {
	code, _, errOut := runCLI("cert", "list", "extra")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "list 不接受参数") {
		t.Fatalf("stderr = %q", errOut)
	}
}
