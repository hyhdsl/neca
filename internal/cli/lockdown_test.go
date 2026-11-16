package cli

import "testing"

func TestLockdownArgsOnlyNamedServices(t *testing.T) {
	args := lockdownArgs("/opt/ipsec/docker-compose.neca.yml")
	want := []string{"compose", "-f", "/opt/ipsec/docker-compose.neca.yml", "--profile", "web", "--profile", "web-write", "stop", "web", "web-helper"}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}
