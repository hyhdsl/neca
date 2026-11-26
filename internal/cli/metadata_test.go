// 本文件测试 metadata 相关逻辑。
package cli

import "testing"

func TestGuessCNNamespaces(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"cert", []string{"issue", "TMP01"}, "TMP01"},
		{"cert", []string{"list"}, ""},
		{"trash", []string{"restore", "entry-1"}, "entry-1"},
	}
	for _, tc := range cases {
		if got := guessCN(tc.name, tc.args); got != tc.want {
			t.Fatalf("guessCN(%q, %v) = %q, want %q", tc.name, tc.args, got, tc.want)
		}
	}
}

func TestBuiltinCommandMetadata(t *testing.T) {
	registry := newBuiltinRegistry()
	cases := []struct {
		name  string
		group string
		write bool
	}{
		{"help", "meta", false},
		{"cert", "cert", true},
		{"pki", "pki", true},
		{"trash", "trash", true},
		{"web", "web", true},
	}
	for _, tc := range cases {
		cmd, ok := registry.Lookup(tc.name)
		if !ok {
			t.Fatalf("missing builtin %q", tc.name)
		}
		if cmd.Surface != SurfaceBackend || cmd.Group != tc.group || cmd.Write != tc.write {
			t.Fatalf("%s metadata = %+v", tc.name, cmd)
		}
		if cmd.AuditEvent == "" {
			t.Fatalf("%s missing audit event", tc.name)
		}
	}
}
