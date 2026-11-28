// 本文件实现 necap 的本地 HTTP API；服务只应挂到 Unix socket。
package necap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	auth *AuthService
	now  func() time.Time
}

func NewServer(auth *AuthService) *Server {
	return &Server{auth: auth, now: time.Now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("POST /v1/auth/password", s.handlePassword)
	mux.HandleFunc("POST /v1/auth/totp", s.handleTOTP)
	mux.HandleFunc("POST /v1/auth/recovery", s.handleRecovery)
	mux.HandleFunc("POST /v1/auth/passkey/begin", s.handlePasskeyBegin)
	mux.HandleFunc("POST /v1/auth/passkey/finish", s.handlePasskeyFinish)
	mux.HandleFunc("POST /v1/factors/status", s.handleStatus)
	mux.HandleFunc("POST /v1/factors/totp/begin", s.handleTOTPBegin)
	mux.HandleFunc("POST /v1/factors/totp/finish", s.handleTOTPFinish)
	mux.HandleFunc("POST /v1/factors/passkey/begin", s.handlePasskeyEnrollBegin)
	mux.HandleFunc("POST /v1/factors/passkey/finish", s.handlePasskeyEnrollFinish)
	mux.HandleFunc("POST /v1/factors/totp/delete", s.handleTOTPDelete)
	mux.HandleFunc("POST /v1/factors/passkey/delete", s.handlePasskeyDelete)
	mux.HandleFunc("POST /v1/factors/recovery/regenerate", s.handleRecoveryRegenerate)
	mux.HandleFunc("POST /v1/factors/policy", s.handlePolicy)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type passwordRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Reauth   bool   `json:"reauth"`
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if !decodeBody(w, r, &req) {
		return
	}
	var challenge *LoginChallenge
	var err error
	if req.Reauth {
		challenge, err = s.auth.BeginReauth(r.Context(), req.Username, req.Password)
	} else {
		challenge, err = s.auth.AuthenticatePassword(r.Context(), req.Username, req.Password)
	}
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, challenge)
}

type factorRequest struct {
	PendingToken string          `json:"pendingToken"`
	Code         string          `json:"code"`
	ChallengeID  string          `json:"challengeId"`
	Response     json.RawMessage `json:"response"`
}

func (s *Server) handleTOTP(w http.ResponseWriter, r *http.Request) {
	var req factorRequest
	if !decodeBody(w, r, &req) {
		return
	}
	result, grant, err := s.auth.CompleteTOTP(r.Context(), req.PendingToken, req.Code)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeAuthCompletion(w, result, grant)
}

func (s *Server) handleRecovery(w http.ResponseWriter, r *http.Request) {
	var req factorRequest
	if !decodeBody(w, r, &req) {
		return
	}
	result, grant, err := s.auth.CompleteRecovery(r.Context(), req.PendingToken, req.Code)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeAuthCompletion(w, result, grant)
}

func (s *Server) handlePasskeyBegin(w http.ResponseWriter, r *http.Request) {
	var req factorRequest
	if !decodeBody(w, r, &req) {
		return
	}
	options, challengeID, err := s.auth.BeginPasskey(r.Context(), req.PendingToken)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challengeId": challengeID, "options": options})
}

func (s *Server) handlePasskeyFinish(w http.ResponseWriter, r *http.Request) {
	var req factorRequest
	if !decodeBody(w, r, &req) {
		return
	}
	result, grant, err := s.auth.FinishPasskey(r.Context(), req.PendingToken, req.ChallengeID, req.Response)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeAuthCompletion(w, result, grant)
}

type managementRequest struct {
	Token       string          `json:"token"`
	UserID      string          `json:"userId"`
	ChallengeID string          `json:"challengeId"`
	Code        string          `json:"code"`
	Label       string          `json:"label"`
	ID          string          `json:"id"`
	Response    json.RawMessage `json:"response"`
	Mode        string          `json:"mode"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.auth.RequireManagement(req.Token, req.UserID); err != nil {
		writeAuthError(w, err)
		return
	}
	status, err := s.auth.FactorStatus(r.Context(), req.UserID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleTOTPBegin(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	enrollment, err := s.auth.BeginTOTPEnrollment(r.Context(), req.Token, req.UserID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, enrollment)
}

func (s *Server) handleTOTPFinish(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	codes, err := s.auth.FinishTOTPEnrollment(r.Context(), req.Token, req.UserID, req.ChallengeID, req.Code)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (s *Server) handlePasskeyEnrollBegin(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	options, challengeID, err := s.auth.BeginPasskeyEnrollment(r.Context(), req.Token, req.UserID, req.Label)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challengeId": challengeID, "options": options})
}

func (s *Server) handlePasskeyEnrollFinish(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	codes, err := s.auth.FinishPasskeyEnrollment(r.Context(), req.Token, req.UserID, req.ChallengeID, req.Response, req.Label)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (s *Server) handleTOTPDelete(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.auth.DeleteTOTPWithManagement(r.Context(), req.Token, req.UserID); err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	id, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(req.ID))
	if err != nil || len(id) == 0 {
		writeAuthError(w, fmt.Errorf("Passkey ID 非法"))
		return
	}
	if err := s.auth.DeletePasskeyWithManagement(r.Context(), req.Token, req.UserID, id); err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRecoveryRegenerate(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	codes, err := s.auth.RegenerateRecoveryCodes(r.Context(), req.Token, req.UserID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (s *Server) handlePolicy(w http.ResponseWriter, r *http.Request) {
	var req managementRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.auth.SetPolicyWithManagement(r.Context(), req.Token, req.UserID, PolicyMode(strings.TrimSpace(req.Mode))); err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ListenAndServe 只在 Unix socket 上提供 API；socket 权限 0600。
func (s *Server) ListenAndServe(ctx context.Context, socket string) error {
	socket = strings.TrimSpace(socket)
	if socket == "" {
		return fmt.Errorf("necap socket 路径为空")
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return fmt.Errorf("创建 necap socket 目录: %w", err)
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return fmt.Errorf("监听 necap socket: %w", err)
	}
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("设置 necap socket 权限: %w", err)
	}
	server := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func writeAuthCompletion(w http.ResponseWriter, result *AuthResult, grant *ManagementGrant) {
	response := map[string]any{}
	if result != nil {
		response["complete"] = true
		response["userId"] = result.UserID
		response["username"] = result.Username
		response["method"] = result.Method
	}
	if grant != nil {
		response["managementToken"] = grant.Token
		response["userId"] = grant.UserID
		response["expires"] = grant.Expires
	}
	writeJSON(w, http.StatusOK, response)
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "message": err.Error()})
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "message": "只允许一个 JSON 对象"})
		return false
	}
	return true
}

func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "auth_error"
	switch {
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidFactor), errors.Is(err, ErrPendingExpired):
		status = http.StatusUnauthorized
		code = "authentication_failed"
	case errors.Is(err, ErrManagement):
		status = http.StatusForbidden
		code = "management_forbidden"
	case errors.Is(err, ErrNoFactor):
		status = http.StatusConflict
		code = "no_factor"
	}
	writeJSON(w, status, map[string]any{"error": code, "message": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
