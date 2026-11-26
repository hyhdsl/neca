// 本文件写入 Web 审计日志。
package web

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// auditLogger 写 Web 登录/登出/下载等事件。
//
// 不记录密码、Cookie、CSRF token、p12 密码或私钥；detail 只放业务字段。
type auditLogger struct {
	mu sync.Mutex
	w  io.Writer
}

func newAuditLogger(w io.Writer) *auditLogger {
	if w == nil {
		w = io.Discard
	}
	return &auditLogger{w: w}
}

func (a *auditLogger) log(event, ip, clientCN, detail string) {
	line := fmt.Sprintf("%s web %s ip=%s cn=%s %s\n",
		time.Now().UTC().Format(time.RFC3339), safeAudit(event),
		safeAudit(ip), safeAudit(orDash(clientCN)), safeAudit(orDash(detail)))
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = io.WriteString(a.w, line)
}

func safeAudit(value string) string {
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "\t", "_")
	value = strings.ReplaceAll(value, "\r", "_")
	value = strings.ReplaceAll(value, "\n", "_")
	return value
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
