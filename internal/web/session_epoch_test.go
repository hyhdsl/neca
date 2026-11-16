package web

import (
	"testing"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

func TestSessionEpochInvalidatesOldCookie(t *testing.T) {
	manager := newSessionManager(make([]byte, 32), 30*time.Minute, time.Now)
	token, _, err := manager.issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.validate(token); err != nil {
		t.Fatalf("initial validate: %v", err)
	}
	manager.SetEpoch(1)
	if _, err := manager.validate(token); err == nil {
		t.Fatal("old cookie survived an epoch change")
	}
	fresh, _, err := manager.issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.validate(fresh); err != nil {
		t.Fatalf("fresh cookie: %v", err)
	}
}

func TestReloadChangesSessionEpoch(t *testing.T) {
	server := &Server{
		cfg: &core.Config{Web: core.WebConfig{
			Enabled: true, Listen: "127.0.0.1:18080", ReadOnly: true, IdleTimeoutRaw: "30m",
		}},
		sessions: newSessionManager(make([]byte, 32), 30*time.Minute, time.Now),
	}
	token, _, err := server.sessions.issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	next := &core.Config{Web: core.WebConfig{
		Enabled: true, Listen: "127.0.0.1:18080", ReadOnly: true, IdleTimeoutRaw: "30m", SessionEpoch: 4,
	}}
	if err := server.Reload(next); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := server.sessions.Epoch(); got != 4 {
		t.Fatalf("epoch = %d, want 4", got)
	}
	if _, err := server.sessions.validate(token); err == nil {
		t.Fatal("old cookie survived SIGHUP epoch change")
	}
}
