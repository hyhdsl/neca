// 本文件测试 export 相关逻辑。
package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hyhdsl/neca/internal/core"
)

func TestExportDisabledIsHidden(t *testing.T) {
	server := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/export/issued.csv", nil)
	server.handleExport(core.ExportIssued)(rec, req)
	if rec.Code != 404 {
		t.Fatalf("disabled export status = %d, want 404", rec.Code)
	}
}

func TestWebExportIncludesClientCertificate(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	h.login(t)
	h.server.exportEnabledFlag.Store(true)
	resp := h.do(t, http.MethodGet, "/export/issued.csv", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "TMP01") {
		t.Fatalf("export body does not include client cert: %q", string(body))
	}
}

func TestCertPageShowsExportLinksWhenEnabled(t *testing.T) {
	h := newWebHarness(t)
	defer h.close()
	h.login(t)

	check := func(want bool) {
		t.Helper()
		resp := h.do(t, http.MethodGet, "/certs", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("certs status = %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		page := string(body)
		for _, marker := range []string{"data-testid=\"export-issued\"", "data-testid=\"export-revoked\"", "/export/issued.csv", "/export/revoked.csv"} {
			if strings.Contains(page, marker) != want {
				t.Fatalf("export marker %q present=%v, want %v", marker, strings.Contains(page, marker), want)
			}
		}
	}

	check(false)
	h.server.exportEnabledFlag.Store(true)
	check(true)
}
