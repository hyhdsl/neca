package web

import (
	"net/http"
	"strings"

	"github.com/hyhdsl/neca/internal/core"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	clientCN, ok := s.checkProxyIdentity(r)
	if !ok {
		s.audit.log("login-denied", clientIP(r), "", "proxy-identity")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	next := safeNext(r.FormValue("next"))
	if !s.validCSRF(r) {
		s.audit.log("login-failed", clientIP(r), clientCN, "csrf")
		http.Error(w, "CSRF token 无效", http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	now := s.now()
	if !s.limiter.allow(ip, now) {
		s.audit.log("login-blocked", ip, clientCN, "rate-limit")
		http.Error(w, "登录失败次数过多，请稍后再试", http.StatusTooManyRequests)
		return
	}
	password := r.FormValue("password")
	if !s.checkPassword(password) {
		s.limiter.fail(ip, now)
		s.audit.log("login-failed", ip, clientCN, "password")
		s.renderStatus(w, r, http.StatusUnauthorized, "login", pageData{
			Title: "登录", Nav: "login", Login: true, Next: next,
			Error: "密码错误或登录已受限",
		})
		return
	}
	s.limiter.reset(ip)
	token, _, err := s.sessions.issue(clientCN)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token, s.secureCookies, core.AbsoluteWebSessionTimeout)
	s.audit.log("login-success", ip, clientCN, "")
	s.redirect(w, r, next)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	claims, ok := sessionClaimsFrom(r.Context())
	if !ok {
		s.unauthorized(w, r)
		return
	}
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		http.Error(w, "CSRF token 无效", http.StatusForbidden)
		return
	}
	s.sessions.revoke(claims)
	clearSessionCookie(w, s.secureCookies)
	s.audit.log("logout", clientIP(r), claims.ClientCN, "")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	list, err := core.ListWebCerts(r.Context(), s.cfg, s.store, s.oss)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	downloads, _ := s.downloadViews()
	stats := dashboardStats{Total: len(list.Certs), CRLRevoked: list.CRLRevoked, Downloads: len(downloads)}
	for _, cert := range list.Certs {
		switch cert.Status {
		case "valid":
			stats.Valid++
		case "revoked":
			stats.Revoked++
		case "expired":
			stats.Expired++
		default:
			stats.Unindexed++
		}
	}
	s.render(w, r, "dashboard", pageData{
		Title: "概览", Nav: "dashboard", Stats: stats,
		Certs: list.Certs, Downloads: downloads, CRLRevoked: list.CRLRevoked,
	})
}

func (s *Server) handleCertList(w http.ResponseWriter, r *http.Request) {
	list, err := core.ListWebCerts(r.Context(), s.cfg, s.store, s.oss)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	certs := filterCerts(list.Certs, query, status)
	s.render(w, r, "certs", pageData{
		Title: "证书", Nav: "certs", Certs: certs, Query: query, Status: status,
		CRLRevoked: list.CRLRevoked,
	})
}

func (s *Server) handleCertDetail(w http.ResponseWriter, r *http.Request) {
	cn := r.PathValue("cn")
	cert, err := core.GetWebCert(r.Context(), s.cfg, s.store, s.oss, cn)
	if err != nil {
		if core.KindOf(err) == core.KindUsage {
			s.renderError(w, r, http.StatusBadRequest, err.Error())
		} else {
			s.renderError(w, r, http.StatusNotFound, err.Error())
		}
		return
	}
	report, verifyErr := core.VerifyWebCert(r.Context(), s.cfg, s.oss, cn)
	if verifyErr != nil {
		report = &core.VerifyReport{CN: cn, Warnings: []string{verifyErr.Error()}}
	}
	s.render(w, r, "cert_detail", pageData{
		Title: "证书 " + cn, Nav: "certs", Cert: cert, Verify: report,
	})
}

func (s *Server) handleDownloads(w http.ResponseWriter, r *http.Request) {
	views, err := s.downloadViews()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.render(w, r, "downloads", pageData{Title: "下载", Nav: "downloads", Downloads: views})
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.renderStatus(w, r, status, "error", pageData{Title: "错误", Nav: "", Error: message})
}
