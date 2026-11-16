package helper

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

const (
	ProtocolVersion   = "v1"
	DefaultSocketPath = "/run/neca/web-helper.sock"
	DefaultSocketGID  = 10001
	MaxRequestIDLen   = 128
)

type Op string

const (
	OpIssue    Op = "issue"
	OpRevoke   Op = "revoke"
	OpUnrevoke Op = "unrevoke"
	OpReissue  Op = "reissue"
	OpTrash    Op = "trash"
)

type Request struct {
	RequestID  string   `json:"requestId"`
	Op         Op       `json:"op"`
	CN         string   `json:"cn"`
	Days       int      `json:"days,omitempty"`
	Platforms  []string `json:"platforms,omitempty"`
	P12Profile string   `json:"p12Profile,omitempty"`
}

type Result struct {
	CN             string   `json:"cn,omitempty"`
	Serial         string   `json:"serial,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Platforms      []string `json:"platforms,omitempty"`
	Messages       []string `json:"messages,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	CRLPath        string   `json:"crlPath,omitempty"`
	ArchivedSerial string   `json:"archivedSerial,omitempty"`
	Already        bool     `json:"already,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Response struct {
	OK       bool    `json:"ok"`
	Protocol string  `json:"protocol"`
	Result   *Result `json:"result,omitempty"`
	Error    *Error  `json:"error,omitempty"`
}

type Status struct {
	OK       bool   `json:"ok"`
	Protocol string `json:"protocol"`
	Version  string `json:"version"`
	ReadOnly bool   `json:"readOnly"`
	Socket   string `json:"socket,omitempty"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("请求为空")
	}
	if strings.TrimSpace(r.RequestID) == "" || len(r.RequestID) > MaxRequestIDLen {
		return fmt.Errorf("requestId 长度必须为 1–%d", MaxRequestIDLen)
	}
	for _, ch := range r.RequestID {
		if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') &&
			!(ch >= '0' && ch <= '9') && ch != '.' && ch != '_' && ch != ':' && ch != '-' {
			return errors.New("requestId 只允许字母数字 . _ : -")
		}
	}
	if !supportedOperation(r.Op) {
		return fmt.Errorf("不支持的 op: %q", r.Op)
	}
	if err := core.ValidateCNName(r.CN); err != nil {
		return err
	}
	if r.Days != 0 && (r.Days < 1 || r.Days > 3650) {
		return errors.New("days 必须在 1–3650 之间")
	}
	if len(r.Platforms) > 0 {
		if _, err := core.NormalizePlatforms(r.Platforms); err != nil {
			return err
		}
	}
	if _, err := core.ParseP12Profile(r.P12Profile); err != nil {
		return err
	}
	return nil
}

func ToResult(value *core.CertificateResult) *Result {
	if value == nil {
		return nil
	}
	return &Result{
		CN: value.CN, Serial: value.Serial, Subject: value.Subject,
		Platforms: value.Platforms, Messages: value.Messages, Warnings: value.Warnings,
		CRLPath: value.CRLPath, ArchivedSerial: value.ArchivedSerial, Already: value.Already,
	}
}

const CacheTTL = 24 * time.Hour
