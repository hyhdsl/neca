package helper

import (
	"errors"
	"fmt"
	"strings"
)

// SettingsRequest is the narrow, root-only high-risk settings surface.
// adminCNs, passkey and helper configuration are intentionally absent.
type SettingsRequest struct {
	RequestID     string  `json:"requestId"`
	ReadOnly      *bool   `json:"readOnly,omitempty"`
	IdleTimeout   *string `json:"idleTimeout,omitempty"`
	ExportEnabled *bool   `json:"exportEnabled,omitempty"`
}

type SettingsResponse struct {
	OK            bool   `json:"ok"`
	Protocol      string `json:"protocol"`
	ReadOnly      bool   `json:"readOnly"`
	IdleTimeout   string `json:"idleTimeout"`
	ExportEnabled bool   `json:"exportEnabled"`
	Error         *Error `json:"error,omitempty"`
}

func (r *SettingsRequest) Validate() error {
	if r == nil {
		return errors.New("请求为空")
	}
	if !validRequestID(r.RequestID) {
		return fmt.Errorf("requestId 长度必须为 1–%d，且只允许字母数字 . _ : -", MaxRequestIDLen)
	}
	if r.ReadOnly == nil && r.IdleTimeout == nil && r.ExportEnabled == nil {
		return errors.New("至少需要修改一个设置字段")
	}
	if r.IdleTimeout != nil && strings.TrimSpace(*r.IdleTimeout) == "" {
		return errors.New("idleTimeout 不能为空")
	}
	return nil
}

func validRequestID(id string) bool {
	if strings.TrimSpace(id) == "" || len(id) > MaxRequestIDLen {
		return false
	}
	for _, ch := range id {
		if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') &&
			!(ch >= '0' && ch <= '9') && ch != '.' && ch != '_' && ch != ':' && ch != '-' {
			return false
		}
	}
	return true
}
