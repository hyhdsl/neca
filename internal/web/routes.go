package web

import (
	"net/http"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/helper"
)

func (s *Server) registerPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /language", s.handleLanguage)
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

func (s *Server) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)
}

func (s *Server) registerCertRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /certs", s.handleCertList)
	mux.HandleFunc("GET /certs/new", s.handleNewCertPage)
	mux.HandleFunc("POST /certs/new", s.handleNewCert)
	mux.HandleFunc("GET /certs/{cn}", s.handleCertDetail)
	mux.HandleFunc("POST /certs/{cn}/revoke", s.handleCertAction(helper.OpRevoke))
	mux.HandleFunc("POST /certs/{cn}/unrevoke", s.handleCertAction(helper.OpUnrevoke))
	mux.HandleFunc("POST /certs/{cn}/reissue", s.handleCertAction(helper.OpReissue))
	mux.HandleFunc("POST /certs/{cn}/trash", s.handleCertAction(helper.OpTrash))
}

func (s *Server) registerDownloadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /downloads", s.handleDownloads)
	mux.HandleFunc("GET /settings", s.handleSettings)
}

func (s *Server) registerExportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /export/issued.csv", s.handleExport(core.ExportIssued))
	mux.HandleFunc("GET /export/revoked.csv", s.handleExport(core.ExportRevoked))
	mux.HandleFunc("GET /api/v1/export/issued.csv", s.handleExport(core.ExportIssued))
	mux.HandleFunc("GET /api/v1/export/revoked.csv", s.handleExport(core.ExportRevoked))
}

func (s *Server) registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /api/v1/certs", s.handleAPICerts)
	mux.HandleFunc("GET /api/v1/certs/{cn}", s.handleAPICertDetail)
	mux.HandleFunc("GET /api/v1/certs/{cn}/verify", s.handleAPIVerify)
	mux.HandleFunc("POST /api/v1/certs", s.handleAPIIssue)
	mux.HandleFunc("POST /api/v1/certs/{cn}/revoke", s.handleAPICertAction(helper.OpRevoke))
	mux.HandleFunc("POST /api/v1/certs/{cn}/unrevoke", s.handleAPICertAction(helper.OpUnrevoke))
	mux.HandleFunc("POST /api/v1/certs/{cn}/reissue", s.handleAPICertAction(helper.OpReissue))
	mux.HandleFunc("POST /api/v1/certs/{cn}/trash", s.handleAPICertAction(helper.OpTrash))
	mux.HandleFunc("GET /api/v1/downloads", s.handleAPIDownloads)
	mux.HandleFunc("GET /api/v1/downloads/{id}", s.handleAPIDownload)
}
