package core

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// AppendAudit 追加一行审计日志（开发文档 §8.7）：
// RFC3339 uid:user 动作 CN 序列号 结果。
//
// 审计失败不应阻断主流程；调用方只把错误转成 [WARN]。
func AppendAudit(cfg *Config, action, cn, serial, result string) error {
	if cfg == nil || strings.TrimSpace(cfg.AuditFile) == "" {
		return nil
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	line := fmt.Sprintf("%s %d:%s %s %s %s %s\n",
		time.Now().UTC().Format(time.RFC3339),
		os.Getuid(), sanitizeAudit(user),
		sanitizeAudit(action), sanitizeAudit(cn), sanitizeAudit(serial), sanitizeAudit(result),
	)
	file, err := os.OpenFile(cfg.AuditFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return runtimef("写审计日志", "%s: %v", cfg.AuditFile, err)
	}
	defer file.Close()
	if _, err := file.WriteString(line); err != nil {
		return runtimef("写审计日志", "%s: %v", cfg.AuditFile, err)
	}
	return nil
}

func sanitizeAudit(value string) string {
	if value == "" {
		return "-"
	}
	replacer := strings.NewReplacer(" ", "_", "\t", "_", "\r", "_", "\n", "_")
	return replacer.Replace(value)
}
