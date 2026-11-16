package cli

import "testing"

func TestBuiltinCommandMetadata(t *testing.T) {
	registry := newBuiltinRegistry()
	cases := []struct {
		name  string
		group string
		write bool
	}{
		{"help", "meta", false},
		{"list", "cert", false},
		{"issue", "cert", true},
		{"index", "pki", true},
		{"downloads", "pki", true},
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
