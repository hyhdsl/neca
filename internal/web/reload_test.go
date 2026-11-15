package web

import (
	"testing"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

func TestReloadWebRuntimeConfig(t *testing.T) {
	server := &Server{
		cfg: &core.Config{Web: core.WebConfig{
			Enabled: true, Listen: "127.0.0.1:18080", ReadOnly: true, IdleTimeoutRaw: "30m",
		}},
		sessions: newSessionManager(make([]byte, 32), 30*time.Minute, time.Now),
	}
	server.readOnlyFlag.Store(true)
	server.idleTimeoutNanos.Store(int64(30 * time.Minute))

	next := &core.Config{Web: core.WebConfig{
		Enabled: true, Listen: "127.0.0.1:18080", ReadOnly: false, IdleTimeoutRaw: "10m",
	}}
	if err := server.Reload(next); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if server.readOnly() {
		t.Fatal("readOnly was not reloaded")
	}
	if got := server.idleTimeout(); got != 10*time.Minute {
		t.Fatalf("idleTimeout = %s, want 10m", got)
	}
	if got := time.Duration(server.sessions.idle.Load()); got != 10*time.Minute {
		t.Fatalf("session idle = %s, want 10m", got)
	}

	bad := &core.Config{Web: core.WebConfig{
		Enabled: true, Listen: "127.0.0.1:18080", ReadOnly: true, IdleTimeoutRaw: "1m",
	}}
	if err := server.Reload(bad); err == nil {
		t.Fatal("Reload accepted invalid idle timeout")
	}
	if server.readOnly() {
		t.Fatal("failed reload changed readOnly")
	}
}
