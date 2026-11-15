package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/helper"
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
			TitleKey: "web.login.title", Nav: "login", Login: true, Next: next,
			Error: s.translatorFor(r).T("web.login.error"),
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
		TitleKey: "web.dashboard.title", Nav: "dashboard", Stats: stats,
		Certs: list.Certs, Downloads: downloads, CRLRevoked: list.CRLRevoked,
		Writable: s.writable(r.Context()), HelperAvailable: s.helperAvailable(r),
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
		TitleKey: "web.certs.title", Nav: "certs", Certs: certs, Query: query, Status: status,
		CRLRevoked: list.CRLRevoked, Writable: s.writable(r.Context()), HelperAvailable: s.helperAvailable(r),
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
		TitleKey: "web.detail.title", TitleArg: cn, Nav: "certs", Cert: cert, Verify: report,
		Writable: s.writable(r.Context()), HelperAvailable: s.helperAvailable(r),
		NoticeKey: noticeKey(r.URL.Query().Get("notice")),
	})
}

func (s *Server) handleDownloads(w http.ResponseWriter, r *http.Request) {
	views, err := s.downloadViews()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.render(w, r, "downloads", pageData{TitleKey: "web.downloads.title", Nav: "downloads", Downloads: views, Writable: s.writable(r.Context())})
}

func (s *Server) handleNewCertPage(w http.ResponseWriter, r *http.Request) {
	if !s.writable(r.Context()) {
		s.renderError(w, r, http.StatusForbidden, "Web 面板当前为只读或 root helper 不可用")
		return
	}
	s.render(w, r, "cert_new", pageData{
		TitleKey: "web.new.title", Nav: "certs", Writable: true, HelperAvailable: true,
		Days: s.cfg.DefaultDays, Platforms: "windows,android", P12Profile: string(s.cfg.P12Profile),
	})
}

func (s *Server) handleNewCert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		http.Error(w, "CSRF token 无效", http.StatusForbidden)
		return
	}
	cn := strings.TrimSpace(r.FormValue("cn"))
	days, err := strconv.Atoi(strings.TrimSpace(r.FormValue("days")))
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "有效期必须是整数天数")
		return
	}
	platforms := splitPlatforms(r.FormValue("platforms"))
	profile := r.FormValue("p12Profile")
	response, err := s.executeWrite(r.Context(), helper.Request{
		RequestID: newWriteRequestID(), Op: helper.OpIssue, CN: cn, Days: days,
		Platforms: platforms, P12Profile: profile,
	})
	if err != nil {
		s.renderWriteError(w, r, err)
		return
	}
	notice := "issued"
	if response.Result != nil && response.Result.Already {
		notice = "already"
	}
	http.Redirect(w, r, "/certs/"+url.PathEscape(cn)+"?notice="+notice, http.StatusSeeOther)
}

func (s *Server) handleCertAction(op helper.Op) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cn := r.PathValue("cn")
		if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
			http.Error(w, "CSRF token 无效", http.StatusForbidden)
			return
		}
		response, err := s.executeWrite(r.Context(), helper.Request{
			RequestID: newWriteRequestID(), Op: op, CN: cn,
		})
		if err != nil {
			s.renderWriteError(w, r, err)
			return
		}
		notice := string(op)
		if response.Result != nil && response.Result.Already {
			notice = "already"
		}
		http.Redirect(w, r, "/certs/"+url.PathEscape(cn)+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
	}
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.renderStatus(w, r, status, "error", pageData{TitleKey: "web.error.title", Nav: "", Error: message})
}

func (s *Server) renderWriteError(w http.ResponseWriter, r *http.Request, err error) {
	var failure *writeFailure
	if errors.As(err, &failure) {
		s.renderError(w, r, failure.Status, failure.Message)
		return
	}
	s.renderError(w, r, http.StatusInternalServerError, err.Error())
}

func (s *Server) writable(ctx context.Context) bool {
	state := s.helperState(ctx)
	return !s.readOnly() && state.Available && !state.ReadOnly
}

func (s *Server) helperAvailable(r *http.Request) bool {
	return s.helperState(r.Context()).Available
}

func splitPlatforms(raw string) []string {
	var platforms []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			platforms = append(platforms, part)
		}
	}
	return platforms
}

func noticeKey(code string) string {
	switch code {
	case "issued":
		return "web.notice.issued"
	case "reissue":
		return "web.notice.reissue"
	case "revoke":
		return "web.notice.revoke"
	case "unrevoke":
		return "web.notice.unrevoke"
	case "already":
		return "web.notice.already"
	default:
		return ""
	}
}
