package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONSourcePrecedence(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caPath, []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, "neca.json")
	payload := map[string]any{
		"schemaVersion": 1,
		"serverId":      "file.example.com",
		"language":      "en-US",
		"paths":         map[string]any{"pkiDir": dir, "caCert": caPath},
		"defaults":      map[string]any{"days": 30, "keyBits": 2048, "p12Profile": "modern", "installers": "windows"},
		"download":      map[string]any{"ttl": "5m", "maxFiles": 3},
		"web": map[string]any{
			"enabled": true, "listen": "127.0.0.1:19000", "readOnly": false,
			"idleTimeout": "10m", "adminCNs": []string{"webadmin-01"},
			"passkey": map[string]any{"enabled": true, "rpId": "vpn.example.com", "rpName": "Test Admin"},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	lookup := func(key string) (string, bool) {
		if key == "SERVER_ID" {
			return "env.example.com", true
		}
		return "", false
	}
	cfg, err := LoadConfigFrom(EnvSource{Lookup: lookup}, JSONSource{Path: jsonPath})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerID != "env.example.com" {
		t.Fatalf("env precedence failed: %q", cfg.ServerID)
	}
	if cfg.Language != "en-US" || cfg.DefaultDays != 30 || cfg.KeyBits != 2048 {
		t.Fatalf("json values not applied: %+v", cfg)
	}
	if cfg.P12Profile != P12Modern || cfg.Installers != "windows" {
		t.Fatalf("profile/installers not applied: %+v", cfg)
	}
	if cfg.DownloadTTL.String() != "5m0s" || cfg.DownloadMaxFiles != 3 {
		t.Fatalf("download not applied: %v %d", cfg.DownloadTTL, cfg.DownloadMaxFiles)
	}
	if !cfg.Web.Enabled || cfg.Web.ReadOnly || cfg.Web.Listen != "127.0.0.1:19000" || cfg.Web.IdleTimeoutRaw != "10m" {
		t.Fatalf("web not applied: %+v", cfg.Web)
	}
	if len(cfg.Web.AdminCNs) != 1 || !cfg.Web.Passkey.Enabled || cfg.Web.Passkey.RPID != "vpn.example.com" {
		t.Fatalf("web admin/passkey not applied: %+v", cfg.Web)
	}
}

func TestJSONSourceMissingAndMalformed(t *testing.T) {
	if _, err := (JSONSource{Path: filepath.Join(t.TempDir(), "missing.json")}).Load(nil); err != nil {
		t.Fatalf("missing file should be ignored: %v", err)
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (JSONSource{Path: path}).Load(nil); err == nil {
		t.Fatal("malformed JSON should fail")
	}
}
