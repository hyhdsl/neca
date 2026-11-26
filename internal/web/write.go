// 本文件将 Web 写操作转发给 helper。
package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/hyhdsl/neca/internal/helper"
)

type helperState struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	ReadOnly   bool   `json:"readOnly"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

type writeFailure struct {
	Status  int
	Code    string
	Message string
}

func (e *writeFailure) Error() string { return e.Code + ": " + e.Message }

func (s *Server) helperState(ctx context.Context) helperState {
	state := helperState{Configured: s.helperClient != nil}
	if s.helperClient == nil {
		return state
	}
	status, err := s.helperClient.Status(ctx)
	if err != nil {
		state.Error = err.Error()
		return state
	}
	state.Available = true
	state.ReadOnly = status.ReadOnly
	state.Version = status.Version
	return state
}

func (s *Server) canWrite(ctx context.Context) bool {
	if s.readOnly() {
		return false
	}
	state := s.helperState(ctx)
	return state.Available && !state.ReadOnly
}

// executeWrite 是 Web 写请求的唯一出口：readOnly、helper 可用性、CSRF
// 都由调用方先检查；此处再做一次 readOnly/helper 双检查。
func (s *Server) executeWrite(ctx context.Context, req helper.Request) (*helper.Response, error) {
	if s.readOnly() {
		return nil, &writeFailure{Status: http.StatusForbidden, Code: "read_only", Message: "Web 面板当前为只读模式"}
	}
	if s.helperClient == nil {
		return nil, &writeFailure{Status: http.StatusServiceUnavailable, Code: "helper_unavailable", Message: "root helper 未安装"}
	}
	state := s.helperState(ctx)
	if !state.Available || state.ReadOnly {
		message := "root helper 不可用"
		if state.Error != "" {
			message += ": " + state.Error
		} else if state.ReadOnly {
			message = "Web 配置仍为只读"
		}
		return nil, &writeFailure{Status: http.StatusServiceUnavailable, Code: "helper_unavailable", Message: message}
	}
	response, err := s.helperClient.Execute(ctx, req)
	if err != nil {
		return nil, &writeFailure{Status: http.StatusBadGateway, Code: "helper_error", Message: err.Error()}
	}
	if response == nil {
		return nil, &writeFailure{Status: http.StatusBadGateway, Code: "helper_error", Message: "helper 返回空响应"}
	}
	if !response.OK {
		status := http.StatusBadGateway
		code := "helper_error"
		message := "helper 操作失败"
		if response.Error != nil {
			code, message = response.Error.Code, response.Error.Message
			switch code {
			case "read_only":
				status = http.StatusForbidden
			case "bad_request":
				status = http.StatusBadRequest
			case "precondition", "request_id_conflict":
				status = http.StatusConflict
			}
		}
		return nil, &writeFailure{Status: status, Code: code, Message: message}
	}
	return response, nil
}

func newWriteRequestID() string {
	return "web-" + randomToken(12)
}

func writeRequestID(r *http.Request, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if value := strings.TrimSpace(r.Header.Get("X-Request-ID")); value != "" {
		return value
	}
	return newWriteRequestID()
}

type issueWriteRequest struct {
	RequestID  string   `json:"requestId"`
	CN         string   `json:"cn"`
	Days       int      `json:"days"`
	Platforms  []string `json:"platforms"`
	P12Profile string   `json:"p12Profile"`
}

type actionWriteRequest struct {
	RequestID string `json:"requestId"`
}

func (s *Server) handleAPIIssue(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		writeJSONError(w, http.StatusForbidden, "csrf", "CSRF token 无效")
		return
	}
	var body issueWriteRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad_request", "请求 JSON 非法: "+err.Error())
		return
	}
	response, err := s.executeWrite(r.Context(), helper.Request{
		RequestID: writeRequestID(r, body.RequestID), Op: helper.OpIssue, CN: body.CN,
		Days: body.Days, Platforms: body.Platforms, P12Profile: body.P12Profile,
	})
	if err != nil {
		writeWriteFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": response.Result})
}

func (s *Server) handleAPICertAction(op helper.Op) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validCSRF(r) {
			writeJSONError(w, http.StatusForbidden, "csrf", "CSRF token 无效")
			return
		}
		var body actionWriteRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeJSONError(w, http.StatusBadRequest, "bad_request", "请求 JSON 非法: "+err.Error())
			return
		}
		response, err := s.executeWrite(r.Context(), helper.Request{
			RequestID: writeRequestID(r, body.RequestID), Op: op, CN: r.PathValue("cn"),
		})
		if err != nil {
			writeWriteFailure(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": response.Result})
	}
}

func writeWriteFailure(w http.ResponseWriter, err error) {
	var failure *writeFailure
	if errors.As(err, &failure) {
		writeJSONError(w, failure.Status, failure.Code, failure.Message)
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "internal", err.Error())
}
