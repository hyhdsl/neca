// 本文件提供证书 CSV 导出路由。
package web

import (
	"fmt"
	"net/http"
	"os"

	"github.com/hyhdsl/neca/internal/core"
)

func (s *Server) handleExport(kind core.ExportKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.exportEnabled() {
			http.NotFound(w, r)
			return
		}
		tmp, err := os.CreateTemp("", "neca-export-*.csv")
		if err != nil {
			http.Error(w, "export failed", http.StatusInternalServerError)
			return
		}
		path := tmp.Name()
		_ = tmp.Close()
		defer os.Remove(path)
		if _, err := core.ExportWebCerts(r.Context(), s.cfg, s.store, s.oss, kind, path); err != nil {
			http.Error(w, "export failed", http.StatusInternalServerError)
			return
		}
		filename := "issued.csv"
		if kind == core.ExportRevoked {
			filename = "revoked.csv"
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, path)
	}
}
