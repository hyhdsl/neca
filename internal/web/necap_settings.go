// 本文件实现 necap 模式下的 Web 安全设置：再认证、状态、TOTP 注册/删除和恢复码。
package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

type necapAdminGrant struct {
	Token   string
	UserID  string
	Expires time.Time
}

type necapSetup struct {
	AdminToken  string
	UserID      string
	ChallengeID string
	URI         string
	Recovery    []string
	Expires     time.Time
}

type necapFactorStatus struct {
	Policy        string `json:"policy"`
	TOTP          bool   `json:"totp"`
	Passkeys      int    `json:"passkeys"`
	RecoveryCodes int    `json:"recoveryCodes"`
}

type necapPasskeyInfo struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
}

type necapPasskeyListResponse struct {
	Passkeys []necapPasskeyInfo `json:"passkeys"`
}

type necapTOTPEnrollment struct {
	ChallengeID string `json:"challengeId"`
	URI         string `json:"uri"`
}

func (s *Server) issueNeCapSession(w http.ResponseWriter, clientCN, userID string) error {
	token, claims, err := s.sessions.issue(clientCN)
	if err != nil {
		return err
	}
	setSessionCookie(w, token, s.secureCookies, core.AbsoluteWebSessionTimeout)
	if userID != "" {
		s.pendingMu.Lock()
		s.necapUsers[claims.ID] = userID
		s.pendingMu.Unlock()
	}
	return nil
}

func (s *Server) necapUserForRequest(r *http.Request) (string, bool) {
	claims, ok := sessionClaimsFrom(r.Context())
	if !ok {
		return "", false
	}
	s.pendingMu.Lock()
	userID := s.necapUsers[claims.ID]
	s.pendingMu.Unlock()
	return userID, userID != ""
}

func (s *Server) necapAdminForRequest(r *http.Request) (necapAdminGrant, bool) {
	claims, ok := sessionClaimsFrom(r.Context())
	if !ok {
		return necapAdminGrant{}, false
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	grant, ok := s.necapAdmins[claims.ID]
	if !ok || s.now().After(grant.Expires) {
		delete(s.necapAdmins, claims.ID)
		return necapAdminGrant{}, false
	}
	return grant, true
}

func (s *Server) handleNeCapSecurity(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.necapUserForRequest(r)
	admin, ok := s.necapAdminForRequest(r)
	data := pageData{
		TitleKey: "web.settings.security_title", Nav: "settings-security",
		Web: &s.cfg.Web, SettingsEnabled: true,
		NeCapEnabled: true, NeCapAdmin: ok,
		ReadOnly: s.readOnly(), Writable: s.writable(r.Context()), HelperAvailable: s.helperAvailable(r),
	}
	if !ok || userID == "" {
		s.render(w, r, "settings_security_necap", data)
		return
	}
	var status necapFactorStatus
	if err := s.necapCall(r.Context(), "/v1/factors/status", map[string]any{"token": admin.Token, "userId": admin.UserID}, &status); err != nil {
		data.NeCapError = err.Error()
		s.render(w, r, "settings_security_necap", data)
		return
	}
	data.NeCapStatus = &status
	var passkeys necapPasskeyListResponse
	if err := s.necapCall(r.Context(), "/v1/factors/passkeys", map[string]any{"token": admin.Token, "userId": admin.UserID}, &passkeys); err == nil {
		data.NeCapPasskeys = passkeys.Passkeys
	}
	s.pendingMu.Lock()
	if setup, exists := s.necapSetups[admin.UserID]; exists && s.now().Before(setup.Expires) {
		data.NeCapTotpURI = setup.URI
		data.NeCapRecoveryCodes = append([]string(nil), setup.Recovery...)
	}
	s.pendingMu.Unlock()
	s.render(w, r, "settings_security_necap", data)
}

func (s *Server) handleNeCapReauth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderNeCapSecurityError(w, r, "表单非法")
		return
	}
	if !s.validCSRF(r) {
		s.renderNeCapSecurityError(w, r, "CSRF token 无效")
		return
	}
	ip := clientIP(r)
	if !s.limiter.allow(ip, s.now()) {
		s.renderNeCapSecurityError(w, r, "尝试过多，请稍后再试")
		return
	}
	password := r.FormValue("password")
	if password == "" {
		s.renderNeCapSecurityError(w, r, "请输入密码")
		return
	}
	var challenge necapChallengeResponse
	if err := s.necapCall(r.Context(), "/v1/auth/password", map[string]any{
		"username": s.necapUsername, "password": password, "reauth": true,
	}, &challenge); err != nil {
		s.limiter.fail(ip, s.now())
		s.renderNeCapSecurityError(w, r, s.translatorFor(r).T("web.login.error"))
		return
	}
	if challenge.Complete {
		if challenge.PendingToken == "" || challenge.UserID == "" {
			s.renderNeCapSecurityError(w, r, "管理凭据不完整")
			return
		}
		s.limiter.reset(ip)
		s.storeNeCapAdmin(w, r, challenge.PendingToken, challenge.UserID)
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	if code == "" {
		s.renderNeCapSecurityError(w, r, "请输入 TOTP 或恢复码")
		return
	}
	if methodMap(challenge.Methods)["totp"] {
		var result necapCompleteResponse
		if err := s.necapCall(r.Context(), "/v1/auth/totp", map[string]any{
			"pendingToken": challenge.PendingToken, "code": code,
		}, &result); err == nil && result.Complete && result.ManagementToken != "" {
			s.limiter.reset(ip)
			s.storeNeCapAdmin(w, r, result.ManagementToken, result.UserID)
			http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
			return
		}
	}
	if methodMap(challenge.Methods)["recovery"] {
		var result necapCompleteResponse
		if err := s.necapCall(r.Context(), "/v1/auth/recovery", map[string]any{
			"pendingToken": challenge.PendingToken, "code": code,
		}, &result); err == nil && result.Complete && result.ManagementToken != "" {
			s.limiter.reset(ip)
			s.storeNeCapAdmin(w, r, result.ManagementToken, result.UserID)
			http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
			return
		}
	}
	s.limiter.fail(ip, s.now())
	s.renderNeCapSecurityError(w, r, s.translatorFor(r).T("web.login.2fa.error"))
}

func (s *Server) storeNeCapAdmin(w http.ResponseWriter, r *http.Request, token, userID string) {
	claims, ok := sessionClaimsFrom(r.Context())
	if !ok {
		return
	}
	s.pendingMu.Lock()
	s.necapUsers[claims.ID] = userID
	s.necapAdmins[claims.ID] = necapAdminGrant{Token: token, UserID: userID, Expires: s.now().Add(5 * time.Minute)}
	s.pendingMu.Unlock()
}

func (s *Server) renderNeCapSecurityError(w http.ResponseWriter, r *http.Request, message string) {
	s.renderStatus(w, r, http.StatusBadRequest, "settings_security_necap", pageData{
		TitleKey: "web.settings.security_title", Nav: "settings-security",
		Web: &s.cfg.Web, SettingsEnabled: true, NeCapEnabled: true, NeCapError: message,
	})
}

func (s *Server) handleNeCapPasskeyBegin(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "csrf"})
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "reauth_required"})
		return
	}
	var response map[string]json.RawMessage
	if err := s.necapCall(r.Context(), "/v1/factors/passkey/begin", map[string]any{
		"token": admin.Token, "userId": admin.UserID, "label": "Web Passkey",
	}, &response); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "passkey_begin_failed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleNeCapPasskeyFinish(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "csrf"})
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "reauth_required"})
		return
	}
	var request struct {
		ChallengeID string          `json:"challengeId"`
		Response    json.RawMessage `json:"response"`
		Label       string          `json:"label"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	var result struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := s.necapCall(r.Context(), "/v1/factors/passkey/finish", map[string]any{
		"token": admin.Token, "userId": admin.UserID, "challengeId": request.ChallengeID,
		"response": request.Response, "label": request.Label,
	}, &result); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "passkey_finish_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recoveryCodes": result.RecoveryCodes})
}

func (s *Server) handleNeCapPasskeyDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		s.renderNeCapSecurityError(w, r, "CSRF token 无效")
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		s.renderNeCapSecurityError(w, r, "Passkey ID 为空")
		return
	}
	if err := s.necapCall(r.Context(), "/v1/factors/passkey/delete", map[string]any{"token": admin.Token, "userId": admin.UserID, "id": id}, nil); err != nil {
		s.renderNeCapSecurityError(w, r, "删除 Passkey 失败；请确认策略仍允许其他因素")
		return
	}
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}

func (s *Server) handleNeCapTOTPBegin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		s.renderNeCapSecurityError(w, r, "CSRF token 无效")
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	var enrollment necapTOTPEnrollment
	if err := s.necapCall(r.Context(), "/v1/factors/totp/begin", map[string]any{"token": admin.Token, "userId": admin.UserID}, &enrollment); err != nil || enrollment.ChallengeID == "" {
		s.renderNeCapSecurityError(w, r, "无法开始 TOTP 注册")
		return
	}
	s.pendingMu.Lock()
	s.necapSetups[admin.UserID] = necapSetup{AdminToken: admin.Token, UserID: admin.UserID, ChallengeID: enrollment.ChallengeID, URI: enrollment.URI, Expires: s.now().Add(5 * time.Minute)}
	s.pendingMu.Unlock()
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}

func (s *Server) handleNeCapTOTPFinish(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		s.renderNeCapSecurityError(w, r, "CSRF token 无效")
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	s.pendingMu.Lock()
	setup, exists := s.necapSetups[admin.UserID]
	s.pendingMu.Unlock()
	if !exists || s.now().After(setup.Expires) {
		s.renderNeCapSecurityError(w, r, "TOTP 注册已过期")
		return
	}
	var response struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := s.necapCall(r.Context(), "/v1/factors/totp/finish", map[string]any{
		"token": admin.Token, "userId": admin.UserID, "challengeId": setup.ChallengeID, "code": strings.TrimSpace(r.FormValue("code")),
	}, &response); err != nil {
		s.renderNeCapSecurityError(w, r, "TOTP 验证失败")
		return
	}
	s.pendingMu.Lock()
	setup.Recovery = response.RecoveryCodes
	setup.ChallengeID = ""
	setup.URI = ""
	s.necapSetups[admin.UserID] = setup
	s.pendingMu.Unlock()
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}

func (s *Server) handleNeCapTOTPDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		s.renderNeCapSecurityError(w, r, "CSRF token 无效")
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	if err := s.necapCall(r.Context(), "/v1/factors/totp/delete", map[string]any{"token": admin.Token, "userId": admin.UserID}, nil); err != nil {
		s.renderNeCapSecurityError(w, r, "删除 TOTP 失败")
		return
	}
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}

func (s *Server) handleNeCapPolicy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		s.renderNeCapSecurityError(w, r, "CSRF token 无效")
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	mode := strings.TrimSpace(r.FormValue("mode"))
	if err := s.necapCall(r.Context(), "/v1/factors/policy", map[string]any{"token": admin.Token, "userId": admin.UserID, "mode": mode}, nil); err != nil {
		s.renderNeCapSecurityError(w, r, "策略更新失败；请先注册对应因素")
		return
	}
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}

func (s *Server) handleNeCapRecoveryRegenerate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		s.renderNeCapSecurityError(w, r, "CSRF token 无效")
		return
	}
	admin, ok := s.necapAdminForRequest(r)
	if !ok {
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	var response struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := s.necapCall(r.Context(), "/v1/factors/recovery/regenerate", map[string]any{"token": admin.Token, "userId": admin.UserID}, &response); err != nil {
		s.renderNeCapSecurityError(w, r, "生成恢复码失败")
		return
	}
	s.pendingMu.Lock()
	setup := s.necapSetups[admin.UserID]
	setup.UserID = admin.UserID
	setup.Recovery = response.RecoveryCodes
	setup.Expires = s.now().Add(5 * time.Minute)
	s.necapSetups[admin.UserID] = setup
	s.pendingMu.Unlock()
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}
