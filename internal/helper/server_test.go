package helper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyhdsl/neca/internal/core"
)

func writeHelperConfig(t *testing.T, readOnly bool) (string, string) {
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
	payload := map[string]any{
		"schemaVersion": 1,
		"paths": map[string]any{
			"pkiDir": pki, "caCert": filepath.Join(pki, "ca.crt"),
		},
		"web": map[string]any{"enabled": true, "readOnly": readOnly, "listen": "127.0.0.1:18080"},
	}
	data, _ := json.Marshal(payload)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NECA_CONFIG", configPath)
	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return configPath, cfg.IndexFile
}

func TestServerReadOnlyRejectsExecute(t *testing.T) {
	configPath, _ := writeHelperConfig(t, true)
	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, configPath)
	body := `{"requestId":"req-1","op":"issue","cn":"TMP01"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/execute", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	var response Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == nil || response.Error.Code != "read_only" {
		t.Fatalf("response = %+v", response)
	}
}

func TestServerRequestIDConflict(t *testing.T) {
	configPath, _ := writeHelperConfig(t, false)
	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, configPath)
	server.cache["req-1"] = cacheEntry{
		fingerprint: "different",
		response:    Response{OK: true, Protocol: ProtocolVersion},
		expires:     server.now().Add(CacheTTL),
	}
	body := `{"requestId":"req-1","op":"issue","cn":"TMP01"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/execute", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	var response Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == nil || response.Error.Code != "request_id_conflict" {
		t.Fatalf("response = %+v", response)
	}
}
