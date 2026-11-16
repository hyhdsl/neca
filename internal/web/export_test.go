package web

import (
	"net/http/httptest"
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
