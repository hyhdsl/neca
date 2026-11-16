package core

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"
)

// AppendAudit 追加一行审计日志（开发文档 §8.7）：
// RFC3339 uid:user 动作 CN 序列号 结果。
//
// 旧调用保持原格式；新调用使用 AppendAuditWithOrigin。
func AppendAudit(cfg *Config, action, cn, serial, result string) error {
	return appendAudit(cfg, "", action, cn, serial, result)
}

// AppendAuditWithOrigin 追加带来源的审计行，例如 origin=cli/helper/web。
func AppendAuditWithOrigin(cfg *Config, origin, action, cn, serial, result string) error {
	return appendAudit(cfg, origin, action, cn, serial, result)
}

func appendAudit(cfg *Config, origin, action, cn, serial, result string) error {
	if cfg == nil || strings.TrimSpace(cfg.AuditFile) == "" {
		return nil
	}
	username := os.Getenv("USER")
	if username == "" {
		if current, err := user.Current(); err == nil && current.Username != "" {
			username = current.Username
		}
	}
	if username == "" {
		if os.Getuid() == 0 {
			username = "root"
		} else {
			username = "unknown"
		}
	}
	originPart := ""
	if strings.TrimSpace(origin) != "" {
		originPart = " origin=" + sanitizeAudit(origin)
	}
	line := fmt.Sprintf("%s %d:%s%s %s %s %s %s\n",
		time.Now().UTC().Format(time.RFC3339),
		os.Getuid(), sanitizeAudit(username), originPart,
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
