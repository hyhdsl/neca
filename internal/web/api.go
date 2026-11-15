package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hyhdsl/neca/internal/core"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	state := s.helperState(r.Context())
	writable := !s.cfg.Web.ReadOnly && state.Available && !state.ReadOnly
	operations := []string{"read"}
	if writable {
		operations = append(operations, "issue", "revoke", "unrevoke", "reissue")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"webEnabled": s.cfg.Web.Enabled,
		"readOnly":   s.cfg.Web.ReadOnly,
		"writable":   writable,
		"helper":     state,
		"passkey": map[string]any{
			"enabled": s.cfg.Web.Passkey.Enabled, "implemented": false,
		},
		"operations": operations,
		"m6a":        true,
		"m6b":        true,
	})
}

func (s *Server) handleAPICerts(w http.ResponseWriter, r *http.Request) {
	list, err := core.ListWebCerts(r.Context(), s.cfg, s.store, s.oss)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	certs := filterCerts(list.Certs, r.URL.Query().Get("q"), r.URL.Query().Get("status"))
	writeJSON(w, http.StatusOK, map[string]any{"certs": certs, "crlRevoked": list.CRLRevoked})
}

func (s *Server) handleAPICertDetail(w http.ResponseWriter, r *http.Request) {
	cert, err := core.GetWebCert(r.Context(), s.cfg, s.store, s.oss, r.PathValue("cn"))
	if err != nil {
		writeWebError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cert)
}

func (s *Server) handleAPIVerify(w http.ResponseWriter, r *http.Request) {
	report, err := core.VerifyWebCert(r.Context(), s.cfg, s.oss, r.PathValue("cn"))
	if err != nil {
		writeWebError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cn": report.CN, "ok": report.OKMessage != "", "okMessage": report.OKMessage,
		"certCount": report.CertCount, "passwordLength": report.PasswordLen,
		"warnings": report.Warnings,
	})
}

func (s *Server) handleAPIDownloads(w http.ResponseWriter, r *http.Request) {
	views, err := s.downloadViews()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"downloads": views})
}

func writeWebError(w http.ResponseWriter, err error) {
	switch core.KindOf(err) {
	case core.KindUsage:
		writeJSONError(w, http.StatusBadRequest, "bad_request", err.Error())
	case core.KindPrecondition:
		writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

func filterCerts(certs []core.WebCert, query, status string) []core.WebCert {
	query = strings.ToLower(strings.TrimSpace(query))
	status = strings.ToLower(strings.TrimSpace(status))
	out := make([]core.WebCert, 0, len(certs))
	for _, cert := range certs {
		if status != "" && cert.Status != status {
			continue
		}
		if query != "" {
			text := strings.ToLower(cert.CN + " " + cert.Serial + " " + cert.Subject + " " + strings.Join(cert.Platforms, " "))
			if !strings.Contains(text, query) {
				continue
			}
		}
		out = append(out, cert)
	}
	return out
}
