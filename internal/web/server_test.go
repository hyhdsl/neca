// 本文件测试 server 相关逻辑。
package web

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/helper"
	"golang.org/x/crypto/bcrypt"
)

type testRunner struct{}

func (testRunner) Run(ctx context.Context, timeout time.Duration, name string, args ...string) (core.Result, error) {
	if len(args) == 0 {
		return core.Result{}, nil
	}
	switch args[0] {
	case "x509":
		flags := strings.Join(args, " ")
		switch {
		case strings.Contains(flags, "-serial"):
			return core.Result{Stdout: "serial=ABC123\n"}, nil
		case strings.Contains(flags, "-startdate"):
			return core.Result{Stdout: "notBefore=Nov 14 20:06:48 2026 GMT\n"}, nil
		case strings.Contains(flags, "-enddate"):
			return core.Result{Stdout: "notAfter=Nov 14 20:06:48 2027 GMT\n"}, nil
		case strings.Contains(flags, "-fingerprint"):
			return core.Result{Stdout: "SHA256 Fingerprint=AA:BB:CC\n"}, nil
		case strings.Contains(flags, "-nameopt"):
			return core.Result{Stdout: "subject=\n    C=CN\n    O=ExampleOrg\n    CN=TMP01\n"}, nil
		case strings.Contains(flags, "-subject"):
			return core.Result{Stdout: "subject=C = CN, O = ExampleOrg, CN = TMP01\n"}, nil
		case strings.Contains(flags, "-ext"):
			return core.Result{Stdout: "X509v3 Subject Alternative Name: \n    DNS:vpn.example.com, DNS:TMP01\n"}, nil
		}
	case "crl":
		return core.Result{Stdout: "Serial Number: 1\nSerial Number: 2\n"}, nil
	case "pkcs12":
		if strings.Contains(strings.Join(args, " "), "-nokeys") {
			return core.Result{Stdout: "BEGIN CERTIFICATE\n"}, nil
		}
		return core.Result{Stdout: "pbeWithSHA1And3-KeyTripleDES-CBC\n"}, nil
	}
	return core.Result{}, nil
}

type webHarness struct {
	server *Server
	http   *httptest.Server
	jar    http.CookieJar
	clock  *time.Time
	config *core.Config
}

func newWebHarness(t *testing.T) *webHarness {
	t.Helper()
	root := t.TempDir()
	cfg := &core.Config{
		PKIDir: filepath.Join(root, "pki"), CACert: filepath.Join(root, "pki", "ca.crt"),
		ClientsDir: filepath.Join(root, "clients"), MetaDir: filepath.Join(root, "pki", "meta"),
		DownloadsDir: filepath.Join(root, "downloads"), IndexFile: filepath.Join(root, "pki", "ca", "index.txt"),
		CRLFile: filepath.Join(root, "pki", "ca.crl.pem"),
		Web: core.WebConfig{
			Enabled: true, Listen: "127.0.0.1:18080", ReadOnly: true,
			IdleTimeoutRaw: "30m",
		},
	}
	for _, dir := range []string{cfg.PKIDir, cfg.ClientsDir, cfg.MetaDir, cfg.DownloadsDir, filepath.Dir(cfg.IndexFile)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(cfg.CACert, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.IndexFile, []byte("V\t271114200648Z\t\tABC123\tunknown\t/CN=TMP01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cn := "TMP01"
	clientDir := filepath.Join(cfg.ClientsDir, cn)
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{cn + ".crt", cn + ".p12", cn + ".p12.pass"} {
		if err := os.WriteFile(filepath.Join(clientDir, name), []byte("dummy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cfg.MetaDir, cn+".env"), []byte("CN=TMP01\nPLATFORMS=windows,android\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	clock := &now
	server, err := New(Options{
		Config: cfg, Store: core.NewFileIndexStore(cfg.IndexFile, filepath.Join(cfg.PKIDir, ".lock")),
		OpenSSL: core.NewOpenSSL(testRunner{}), PasswordHash: hash, SessionKey: key,
		AllowNonLoopback: true, Audit: io.Discard, Now: func() time.Time { return *clock },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(server.Handler())
	jar, _ := cookiejar.New(nil)
	return &webHarness{server: server, http: ts, jar: jar, clock: clock, config: cfg}
}

func (h *webHarness) close() { h.http.Close() }

func (h *webHarness) do(t *testing.T, method, path string, form url.Values) *http.Response {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, h.http.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	client := &http.Client{Jar: h.jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (h *webHarness) login(t *testing.T) {
	t.Helper()
	resp := h.do(t, http.MethodGet, "/login", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login page status = %d", resp.StatusCode)
	}
	var token string
	for _, cookie := range h.jar.Cookies(mustURL(t, h.http.URL+"/login")) {
		if cookie.Name == csrfCookieName {
			token = cookie.Value
		}
	}
	if token == "" {
		t.Fatal("missing CSRF cookie")
	}
	resp = h.do(t, http.MethodPost, "/login", url.Values{"password": {"secret123"}, "csrf_token": {token}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", resp.StatusCode)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestWebRequiresAuthentication(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	resp := h.do(t, http.MethodGet, "/api/v1/certs", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated API status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
	resp = h.do(t, http.MethodGet, "/certs", nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("unauthenticated page status = %d, want 303", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWebLoginAndReadOnly(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	h.login(t)

	resp := h.do(t, http.MethodGet, "/api/v1/certs", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("certs status = %d", resp.StatusCode)
	}
	var payload struct {
		Certs []core.WebCert `json:"certs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(payload.Certs) != 1 || payload.Certs[0].CN != "TMP01" {
		t.Fatalf("certs = %+v", payload.Certs)
	}

	resp = h.do(t, http.MethodGet, "/", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status = %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(data), "概览") || !strings.Contains(string(data), "TMP01") {
		t.Fatalf("dashboard missing content: %s", data)
	}

	resp = h.do(t, http.MethodPost, "/api/v1/certs", url.Values{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only POST status = %d, want 403", resp.StatusCode)
	}
}

func TestWebIdleTimeoutExpires(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	h.login(t)
	*h.clock = h.clock.Add(31 * time.Minute)
	resp := h.do(t, http.MethodGet, "/api/v1/certs", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("idle expired status = %d, want 401", resp.StatusCode)
	}
}

func TestWebProxyIdentityAndDetail(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	h.server.cfg.Web.AdminCNs = []string{"webadmin-01"}
	h.server.proxySecret = []byte("proxy-secret")

	resp := h.do(t, http.MethodGet, "/login", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login without proxy identity = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, h.http.URL+"/login", nil)
	req.Header.Set("X-Neca-Proxy-Secret", "proxy-secret")
	req.Header.Set("X-Neca-Client-CN", "webadmin-01")
	client := &http.Client{Jar: h.jar}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login with proxy identity = %d, want 200", resp.StatusCode)
	}

	h.server.cfg.Web.AdminCNs = nil
	h.login(t)
	resp = h.do(t, http.MethodGet, "/api/v1/certs/TMP01", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cert detail = %d", resp.StatusCode)
	}
	var cert core.WebCert
	if err := json.NewDecoder(resp.Body).Decode(&cert); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if cert.CN != "TMP01" || cert.Subject == "" {
		t.Fatalf("cert detail = %+v", cert)
	}

	resp = h.do(t, http.MethodGet, "/api/v1/certs/TMP01/verify", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = h.do(t, http.MethodGet, "/api/v1/downloads/%2e%2e%2f%2e%2e%2fetc%2fpasswd", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("path traversal status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	resp = h.do(t, http.MethodGet, "/healthz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPasskeyRoutesAbsent(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	h.login(t)
	for _, path := range []string{"/passkey/register", "/api/v1/passkey/login"} {
		resp := h.do(t, http.MethodGet, path, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("passkey route %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

func startFakeHelper(t *testing.T) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(helper.Status{OK: true, Protocol: helper.ProtocolVersion, Version: "test", ReadOnly: false})
		case "/v1/execute":
			var req helper.Request
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(helper.Response{OK: true, Protocol: helper.ProtocolVersion, Result: &helper.Result{CN: req.CN, Serial: "ABC123"}})
		default:
			http.NotFound(w, r)
		}
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	return socket
}

func csrfToken(t *testing.T, h *webHarness) string {
	t.Helper()
	for _, cookie := range h.jar.Cookies(mustURL(t, h.http.URL+"/")) {
		if cookie.Name == csrfCookieName {
			return cookie.Value
		}
	}
	t.Fatal("missing CSRF cookie")
	return ""
}

func postJSON(t *testing.T, h *webHarness, path, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.http.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)
	client := &http.Client{Jar: h.jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestWebWriteAPIWithHelper(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	h.login(t)
	socket := startFakeHelper(t)
	h.server.readOnlyFlag.Store(false)
	h.server.helperClient = helper.NewClient(socket)
	token := csrfToken(t, h)

	body := `{"requestId":"req-1","cn":"TMP01","days":30,"platforms":["windows"],"p12Profile":"compat"}`
	resp := postJSON(t, h, "/api/v1/certs", token, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("write API status = %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if result["ok"] != true {
		t.Fatalf("write result = %+v", result)
	}

	resp = h.do(t, http.MethodGet, "/api/v1/capabilities", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("capabilities status = %d", resp.StatusCode)
	}
	var capabilities struct {
		Writable bool        `json:"writable"`
		Helper   helperState `json:"helper"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !capabilities.Writable || !capabilities.Helper.Available {
		t.Fatalf("capabilities = %+v", capabilities)
	}

	h.server.readOnlyFlag.Store(true)
	resp = postJSON(t, h, "/api/v1/certs", token, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only write status = %d, want 403", resp.StatusCode)
	}
}

func TestWebLanguageSwitch(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	resp := h.do(t, http.MethodGet, "/language?lang=en-US&next=/login", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("language switch status = %d", resp.StatusCode)
	}
	resp = h.do(t, http.MethodGet, "/login", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("english login status = %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(data), "Administrator sign-in") || !strings.Contains(string(data), "lang=\"en-US\"") {
		t.Fatalf("english login page not translated: %s", data)
	}
}

func TestWebCSRFAndDownloads(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	resp := h.do(t, http.MethodPost, "/login", url.Values{"password": {"secret123"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login without CSRF status = %d, want 403", resp.StatusCode)
	}
	h.login(t)

	zipPath := filepath.Join(h.config.DownloadsDir, "TMP01-test.zip")
	if err := os.WriteFile(zipPath, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{
		"path": zipPath, "cn": "TMP01", "createdAt": h.clock.UTC(),
		"expiresAt": h.clock.Add(30 * time.Minute).UTC(), "size": 3, "sha256": "abc",
	}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(zipPath+".json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	resp = h.do(t, http.MethodGet, "/api/v1/downloads", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("downloads status = %d", resp.StatusCode)
	}
	var listing struct {
		Downloads []downloadView `json:"downloads"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(listing.Downloads) != 1 {
		t.Fatalf("downloads = %+v", listing.Downloads)
	}
	resp = h.do(t, http.MethodGet, "/api/v1/downloads/"+listing.Downloads[0].ID, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d", resp.StatusCode)
	}

	*h.clock = h.clock.Add(31 * time.Minute)
	resp = h.do(t, http.MethodGet, "/api/v1/downloads/"+listing.Downloads[0].ID, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired download status = %d", resp.StatusCode)
	}
}
