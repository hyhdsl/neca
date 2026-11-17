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

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	s.renderSettingsPage(w, r, "settings", "settings", "web.settings.general_title")
}

func (s *Server) handleSettingsSecurity(w http.ResponseWriter, r *http.Request) {
	s.renderSettingsPage(w, r, "settings_security", "settings-security", "web.settings.security_title")
}

func (s *Server) handleSettingsAbout(w http.ResponseWriter, r *http.Request) {
	s.renderSettingsPage(w, r, "settings_about", "settings-about", "web.settings.about_title")
}

func (s *Server) renderSettingsPage(w http.ResponseWriter, r *http.Request, templateName, nav, titleKey string) {
	cfg, err := core.LoadConfigReadOnly()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	retention := "不限时"
	if cfg.TrashRetention > 0 {
		retention = cfg.TrashRetention.String()
	}
	s.render(w, r, templateName, pageData{
		TitleKey: titleKey, Nav: nav, Web: &cfg.Web, TrashRetention: retention,
		ReadOnly: s.readOnly(), Writable: s.writable(r.Context()), HelperAvailable: s.helperAvailable(r),
		NoticeKey: noticeKey(r.URL.Query().Get("notice")),
	})
}

func (s *Server) handleSettingsHighRisk(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "表单非法")
		return
	}
	if !s.validCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "CSRF token 无效")
		return
	}
	cfg, err := core.LoadConfigReadOnly()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !cfg.Web.AllowHighRiskSettings {
		s.renderError(w, r, http.StatusForbidden, "后台未允许 Web 修改高风险设置")
		return
	}
	if !s.writable(r.Context()) {
		s.renderError(w, r, http.StatusForbidden, "Web 当前不可写或 root helper 不可用")
		return
	}
	if !s.checkPassword(r.FormValue("password")) {
		s.audit.log("settings-reauth-failed", clientIP(r), "", "high-risk")
		s.renderError(w, r, http.StatusForbidden, "重新认证失败")
		return
	}
	if r.FormValue("confirm") != "yes" {
		s.renderError(w, r, http.StatusBadRequest, "需要明确确认（Y）")
		return
	}
	update := helper.SettingsRequest{RequestID: newWriteRequestID()}
	if value := r.FormValue("readOnly"); value == "true" || value == "false" {
		parsed := value == "true"
		update.ReadOnly = &parsed
	}
	if value := strings.TrimSpace(r.FormValue("idleTimeout")); value != "" {
		update.IdleTimeout = &value
	}
	if value := r.FormValue("exportEnabled"); value == "true" || value == "false" {
		parsed := value == "true"
		update.ExportEnabled = &parsed
	}
	if update.ReadOnly == nil && update.IdleTimeout == nil && update.ExportEnabled == nil {
		s.renderError(w, r, http.StatusBadRequest, "至少修改一个设置")
		return
	}
	if s.helperClient == nil {
		s.renderError(w, r, http.StatusServiceUnavailable, "root helper 未安装")
		return
	}
	if _, err := s.helperClient.UpdateSettings(r.Context(), update); err != nil {
		s.renderError(w, r, http.StatusBadGateway, err.Error())
		return
	}
	next, err := core.LoadConfigReadOnly()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Reload(next); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit.log("settings-high-risk", clientIP(r), "", "readOnly/idleTimeout/export updated")
	http.Redirect(w, r, "/settings/security?notice=updated", http.StatusSeeOther)
}

func (s *Server) handleNewCertPage(w http.ResponseWriter, r *http.Request) {
	if !s.writable(r.Context()) {
		s.renderError(w, r, http.StatusForbidden, "Web 面板当前为只读或 root helper 不可用")
		return
	}
	s.render(w, r, "cert_new", pageData{
		TitleKey: "web.new.title", Nav: "issue", Writable: true, HelperAvailable: true,
		Days: s.cfg.DefaultDays, PlatformChoices: buildPlatformChoices(core.DefaultPlatforms()), P12Profile: string(s.cfg.P12Profile),
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
	rawPlatforms := r.Form["platform"]
	if len(rawPlatforms) == 0 {
		rawPlatforms = splitPlatforms(r.FormValue("platforms"))
	}
	platforms, err := core.NormalizePlatforms(rawPlatforms)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "平台选择无效: "+err.Error())
		return
	}
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
		if op == helper.OpTrash {
			http.Redirect(w, r, "/certs?notice=trashed", http.StatusSeeOther)
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

type platformChoice struct {
	Value    string
	LabelKey string
	Selected bool
}

var platformChoiceDefs = []struct{ value, labelKey string }{
	{core.PlatformWindows, "web.platform.windows"},
	{core.PlatformLinux, "web.platform.linux"},
	{core.PlatformMacOS, "web.platform.macos"},
	{core.PlatformAndroid, "web.platform.android"},
	{core.PlatformIOS, "web.platform.ios"},
}

func buildPlatformChoices(selected []string) []platformChoice {
	chosen := make(map[string]bool, len(selected))
	for _, value := range selected {
		chosen[value] = true
	}
	choices := make([]platformChoice, 0, len(platformChoiceDefs))
	for _, def := range platformChoiceDefs {
		choices = append(choices, platformChoice{Value: def.value, LabelKey: def.labelKey, Selected: chosen[def.value]})
	}
	return choices
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
	case "trashed":
		return "web.notice.trashed"
	case "updated":
		return "web.notice.settings_updated"
	default:
		return ""
	}
}
