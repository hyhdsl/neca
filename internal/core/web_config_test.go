package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyhdsl/neca/internal/core/ports"
)

func TestWebConfigDefaultsAndValidation(t *testing.T) {
	cfg := parseWebConfigValues(nil)
	if cfg.Enabled || !cfg.ReadOnly || cfg.Listen != DefaultWebListen {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if idle, err := cfg.IdleTimeout(); err != nil || idle != DefaultWebIdleTimeout {
		t.Fatalf("idle = %v, %v", idle, err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default web config should validate: %v", err)
	}

	bad := cfg
	bad.Listen = "0.0.0.0:18080"
	if err := bad.Validate(); err == nil {
		t.Fatal("non-loopback listen must be rejected")
	}
	bad = cfg
	bad.IdleTimeoutRaw = "1m"
	if err := bad.Validate(); err == nil {
		t.Fatal("idle timeout below 5m must be rejected")
	}
	bad = cfg
	bad.IdleTimeoutRaw = "25h"
	if err := bad.Validate(); err == nil {
		t.Fatal("idle timeout above 24h must be rejected")
	}
	bad = cfg
	bad.Passkey.Enabled = true
	if err := bad.Validate(); err == nil {
		t.Fatal("passkey without rpId must be rejected")
	}
}

func TestWebSessionEpoch(t *testing.T) {
	cfg := parseWebConfigValues(map[string]string{"NECA_WEB_SESSION_EPOCH": "3"})
	if cfg.SessionEpoch != 3 {
		t.Fatalf("sessionEpoch = %d, want 3", cfg.SessionEpoch)
	}
	bad := cfg
	bad.SessionEpoch = -1
	if err := bad.Validate(); err == nil {
		t.Fatal("negative sessionEpoch must be rejected")
	}
}

func TestUpdateWebConfigPreservesDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "neca.json")
	original := `{
  "schemaVersion": 1,
  "serverId": "keep.example.com",
  "unknown": {"x": 1},
  "web": {
    "enabled": true,
    "listen": "127.0.0.1:18080",
    "readOnly": true,
    "sessionEpoch": 2,
    "idleTimeout": "30m",
    "adminCNs": ["webadmin-01"]
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	web, err := UpdateWebReadOnly(path, false)
	if err != nil {
		t.Fatalf("UpdateWebReadOnly: %v", err)
	}
	if web.ReadOnly {
		t.Fatal("readOnly should be false")
	}
	if _, err := UpdateWebIdleTimeout(path, "10m"); err != nil {
		t.Fatalf("UpdateWebIdleTimeout: %v", err)
	}
	if _, err := UpdateWebIdleTimeout(path, "1m"); err == nil {
		t.Fatal("invalid idle timeout should fail")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("config mode = %o, want 640", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["serverId"] != "keep.example.com" {
		t.Fatalf("top-level fields lost: %v", doc)
	}
	if _, ok := doc["unknown"]; !ok {
		t.Fatalf("unknown top-level field lost: %v", doc)
	}
	webDoc, _ := doc["web"].(map[string]any)
	if webDoc["readOnly"] != false {
		t.Fatalf("readOnly not persisted: %v", webDoc)
	}
	if webDoc["idleTimeout"] != "10m0s" {
		t.Fatalf("idleTimeout not persisted: %v", webDoc)
	}
	if webDoc["sessionEpoch"] != float64(2) {
		t.Fatalf("sessionEpoch not preserved: %v", webDoc)
	}
}

func TestWebAllowHighRiskSettings(t *testing.T) {
	cfg := parseWebConfigValues(map[string]string{"NECA_WEB_ALLOW_HIGH_RISK_SETTINGS": "true"})
	if !cfg.AllowHighRiskSettings {
		t.Fatal("allowHighRiskSettings was not parsed")
	}
}

func TestUpdateWebLockAndExport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "neca.json")
	original := `{"schemaVersion":1,"web":{"enabled":true,"listen":"127.0.0.1:18080","readOnly":false,"sessionEpoch":2}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	locked, err := UpdateWebLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if !locked.ReadOnly || locked.SessionEpoch != 3 {
		t.Fatalf("lock = %+v, want readOnly=true epoch=3", locked)
	}
	exported, err := UpdateWebExportEnabled(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if !exported.ExportEnabled {
		t.Fatalf("export = %+v, want enabled", exported)
	}
}

func TestExampleConfigJSONHasSafeWebDefaults(t *testing.T) {
	data, err := ExampleConfigJSON()
	if err != nil {
		t.Fatal(err)
	}
	var doc fileConfig
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Web.Enabled == nil || *doc.Web.Enabled {
		t.Fatalf("example web.enabled = %v, want false", doc.Web.Enabled)
	}
	if doc.Web.ReadOnly == nil || !*doc.Web.ReadOnly {
		t.Fatalf("example web.readOnly = %v, want true", doc.Web.ReadOnly)
	}
	if doc.Web.Listen != DefaultWebListen || doc.Web.IdleTimeout != "30m0s" {
		t.Fatalf("example web = %+v", doc.Web)
	}
}

func TestLoadConfigReadOnlyDoesNotCreateLayout(t *testing.T) {
	root := t.TempDir()
	pki := filepath.Join(root, "pki")
	if err := os.MkdirAll(pki, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pki, "ca.crt"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFrom([]ports.Source{mapSource{"PKI_DIR": pki}}, false)
	if err != nil {
		t.Fatalf("loadConfigFrom: %v", err)
	}
	if _, err := os.Stat(cfg.CADBDir); !os.IsNotExist(err) {
		t.Fatalf("read-only loader must not create ca db dir: %v", err)
	}
	if cfg.Web.Listen != DefaultWebListen || cfg.Web.IdleTimeoutRaw != DefaultWebIdleTimeout.String() {
		t.Fatalf("web defaults not applied: %+v", cfg.Web)
	}
}
