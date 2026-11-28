// 本文件实现可选 necap 模式下的 Web 登录与第二因素流程。
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

const mfaCookieName = "neca_mfa"

type pendingMFA struct {
	PendingToken string
	UserID       string
	Username     string
	ClientCN     string
	Next         string
	Methods      map[string]bool
	Expires      time.Time
}

type necapChallengeResponse struct {
	Complete          bool     `json:"complete"`
	UserID            string   `json:"userId"`
	Username          string   `json:"username"`
	PendingToken      string   `json:"pendingToken"`
	Methods           []string `json:"methods"`
	RecoveryRemaining int      `json:"recoveryRemaining"`
}

type necapCompleteResponse struct {
	Complete        bool   `json:"complete"`
	UserID          string `json:"userId"`
	Username        string `json:"username"`
	Method          string `json:"method"`
	ManagementToken string `json:"managementToken"`
}

func (s *Server) necapCall(ctx context.Context, path string, payload any, out any) error {
	if !s.necapEnabled || s.helperClient == nil {
		return fmt.Errorf("necap 未启用")
	}
	status, body, err := s.helperClient.NeCap(ctx, path, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("necap 返回 HTTP %d", status)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("解析 necap 响应: %w", err)
		}
	}
	return nil
}

func (s *Server) handleNeCapLogin(w http.ResponseWriter, r *http.Request, clientCN, next, password string) {
	var challenge necapChallengeResponse
	err := s.necapCall(r.Context(), "/v1/auth/password", map[string]any{
		"username": s.necapUsername, "password": password,
	}, &challenge)
	if err != nil {
		s.limiter.fail(clientIP(r), s.now())
		s.audit.log("login-failed", clientIP(r), clientCN, "necap")
		s.renderStatus(w, r, http.StatusUnauthorized, "login", pageData{
			TitleKey: "web.login.title", Nav: "login", Login: true, Next: next,
			Error: s.translatorFor(r).T("web.login.error"),
		})
		return
	}
	if challenge.Complete {
		s.completeWebLogin(w, r, clientCN, next, "password")
		return
	}
	if challenge.PendingToken == "" || len(challenge.Methods) == 0 {
		s.audit.log("login-failed", clientIP(r), clientCN, "necap-no-factor")
		s.renderStatus(w, r, http.StatusUnauthorized, "login", pageData{
			TitleKey: "web.login.title", Nav: "login", Login: true, Next: next,
			Error: s.translatorFor(r).T("web.login.error"),
		})
		return
	}
	token := randomToken(24)
	pending := pendingMFA{
		PendingToken: challenge.PendingToken, UserID: challenge.UserID, Username: challenge.Username,
		ClientCN: clientCN, Next: next, Methods: methodMap(challenge.Methods), Expires: s.now().Add(5 * time.Minute),
	}
	s.pendingMu.Lock()
	s.pending[token] = pending
	s.pendingMu.Unlock()
	setMFACookie(w, token, s.secureCookies, 5*time.Minute)
	http.Redirect(w, r, "/login/2fa", http.StatusSeeOther)
}

func methodMap(methods []string) map[string]bool {
	result := map[string]bool{}
	for _, method := range methods {
		result[method] = true
	}
	return result
}

func sortedMethods(methods map[string]bool) []string {
	result := make([]string, 0, len(methods))
	for method := range methods {
		if method == "totp" || method == "passkey" || method == "recovery" {
			result = append(result, method)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Server) pendingFromRequest(r *http.Request) (string, pendingMFA, bool) {
	cookie, err := r.Cookie(mfaCookieName)
	if err != nil || cookie.Value == "" {
		return "", pendingMFA{}, false
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	pending, ok := s.pending[cookie.Value]
	if !ok || s.now().After(pending.Expires) {
		delete(s.pending, cookie.Value)
		return "", pendingMFA{}, false
	}
	return cookie.Value, pending, true
}

func (s *Server) clearPending(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(mfaCookieName); err == nil {
		s.pendingMu.Lock()
		delete(s.pending, cookie.Value)
		s.pendingMu.Unlock()
	}
	clearMFACookie(w, s.secureCookies)
}

func (s *Server) completeWebLogin(w http.ResponseWriter, r *http.Request, clientCN, next, method string) {
	token, _, err := s.sessions.issue(clientCN)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token, s.secureCookies, core.AbsoluteWebSessionTimeout)
	s.clearPending(w, r)
	s.limiter.reset(clientIP(r))
	s.audit.log("login-success", clientIP(r), clientCN, method)
	s.redirect(w, r, next)
}

func (s *Server) handleLogin2FA(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.checkProxyIdentity(r); !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if token, ok := sessionFromRequest(r); ok {
		if _, err := s.sessions.validate(token); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	_, pending, ok := s.pendingFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	errorText := ""
	if r.URL.Query().Get("error") != "" {
		errorText = s.translatorFor(r).T("web.login.2fa.error")
	}
	s.render(w, r, "login_2fa", pageData{
		TitleKey: "web.login.2fa.title", Nav: "login", Login: true, Next: pending.Next,
		Methods: sortedMethods(pending.Methods), TOTPAvailable: pending.Methods["totp"],
		PasskeyAvailable: pending.Methods["passkey"], RecoveryAvailable: pending.Methods["recovery"],
		Error: errorText,
	})
}

func (s *Server) handleLoginTOTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		http.Error(w, "CSRF token 无效", http.StatusForbidden)
		return
	}
	_, pending, ok := s.pendingFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !s.limiter.allow(clientIP(r), s.now()) {
		http.Error(w, "登录失败次数过多，请稍后再试", http.StatusTooManyRequests)
		return
	}
	var result necapCompleteResponse
	err := s.necapCall(r.Context(), "/v1/auth/totp", map[string]any{
		"pendingToken": pending.PendingToken, "code": strings.TrimSpace(r.FormValue("code")),
	}, &result)
	if err != nil || !result.Complete {
		s.limiter.fail(clientIP(r), s.now())
		s.audit.log("login-failed", clientIP(r), "", "totp")
		s.redirectWithError(w, r, "totp")
		return
	}
	s.completeWebLogin(w, r, pending.ClientCN, pending.Next, "totp")
}

func (s *Server) handleLoginRecovery(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.validCSRF(r) {
		http.Error(w, "CSRF token 无效", http.StatusForbidden)
		return
	}
	_, pending, ok := s.pendingFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !s.limiter.allow(clientIP(r), s.now()) {
		http.Error(w, "登录失败次数过多，请稍后再试", http.StatusTooManyRequests)
		return
	}
	var result necapCompleteResponse
	err := s.necapCall(r.Context(), "/v1/auth/recovery", map[string]any{
		"pendingToken": pending.PendingToken, "code": strings.TrimSpace(r.FormValue("code")),
	}, &result)
	if err != nil || !result.Complete {
		s.limiter.fail(clientIP(r), s.now())
		s.audit.log("login-failed", clientIP(r), "", "recovery")
		s.redirectWithError(w, r, "recovery")
		return
	}
	s.completeWebLogin(w, r, pending.ClientCN, pending.Next, "recovery")
}

func (s *Server) redirectWithError(w http.ResponseWriter, r *http.Request, _ string) {
	http.Redirect(w, r, "/login/2fa?error=1", http.StatusSeeOther)
}

func (s *Server) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "csrf"})
		return
	}
	_, pending, ok := s.pendingFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "pending_expired"})
		return
	}
	var response map[string]json.RawMessage
	if err := s.necapCall(r.Context(), "/v1/auth/passkey/begin", map[string]any{"pendingToken": pending.PendingToken}, &response); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "necap_unavailable"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "csrf"})
		return
	}
	_, pending, ok := s.pendingFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "pending_expired"})
		return
	}
	var request struct {
		ChallengeID string          `json:"challengeId"`
		Response    json.RawMessage `json:"response"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	var result necapCompleteResponse
	if err := s.necapCall(r.Context(), "/v1/auth/passkey/finish", map[string]any{
		"pendingToken": pending.PendingToken, "challengeId": request.ChallengeID, "response": request.Response,
	}, &result); err != nil || !result.Complete {
		s.audit.log("login-failed", clientIP(r), "", "passkey")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_passkey"})
		return
	}
	token, _, err := s.sessions.issue(pending.ClientCN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal"})
		return
	}
	setSessionCookie(w, token, s.secureCookies, core.AbsoluteWebSessionTimeout)
	s.clearPending(w, r)
	s.limiter.reset(clientIP(r))
	s.audit.log("login-success", clientIP(r), "", "passkey")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "redirect": pending.Next})
}

func setMFACookie(w http.ResponseWriter, token string, secure bool, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: mfaCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(ttl.Seconds()), Expires: time.Now().UTC().Add(ttl),
	})
}

func clearMFACookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: mfaCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC(),
	})
}
