package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Web 面板的默认值与硬上限。绝对会话上限固定 24h，不允许配置成无限会话。
const (
	DefaultWebListen          = "127.0.0.1:18080"
	DefaultWebIdleTimeout     = 30 * time.Minute
	MinWebIdleTimeout         = 5 * time.Minute
	MaxWebIdleTimeout         = 24 * time.Hour
	AbsoluteWebSessionTimeout = 24 * time.Hour
)

// PasskeyConfig 是 WebAuthn/Passkey 的 feature flag。
//
// M6a 只保存配置并保留接口，不注册路由、不引入 WebAuthn 依赖。
type PasskeyConfig struct {
	Enabled bool   `json:"enabled"`
	RPID    string `json:"rpId,omitempty"`
	RPName  string `json:"rpName,omitempty"`
}

// WebConfig 是 neca.json 的 web 段在内存中的表示。
//
// 该结构不包含密码哈希、会话密钥、mTLS 私钥或数据库密码；这些敏感值
// 通过环境变量/0600 文件注入，避免随配置快照或日志泄露。
type WebConfig struct {
	Enabled        bool
	Listen         string
	ReadOnly       bool
	SessionEpoch   int
	IdleTimeoutRaw string
	AdminCNs       []string
	Passkey        PasskeyConfig
}

// defaultWebConfig 返回安全默认值：关闭、只读、仅回环监听。
func defaultWebConfig() WebConfig {
	return WebConfig{
		Enabled:        false,
		Listen:         DefaultWebListen,
		ReadOnly:       true,
		IdleTimeoutRaw: DefaultWebIdleTimeout.String(),
		Passkey: PasskeyConfig{
			Enabled: false,
			RPName:  "neca Admin",
		},
	}
}

// parseWebConfigValues 把配置层 key/value 转成 WebConfig。
//
// 解析保持宽容：非法布尔值退回默认值，idleTimeout 保留原始字符串，
// 由 web 子命令或 necad 调用 Validate 报告。这样 lite CLI 的其他命令
// 不会因为 Web 配置写错而完全不可用（开发文档 §4）。
func parseWebConfigValues(values map[string]string) WebConfig {
	cfg := defaultWebConfig()
	if value, ok := values["NECA_WEB_ENABLED"]; ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
			cfg.Enabled = parsed
		}
	}
	if value := strings.TrimSpace(values["NECA_WEB_LISTEN"]); value != "" {
		cfg.Listen = value
	}
	if value, ok := values["NECA_WEB_READONLY"]; ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
			cfg.ReadOnly = parsed
		}
	}
	if value := strings.TrimSpace(values["NECA_WEB_SESSION_EPOCH"]); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.SessionEpoch = parsed
		}
	}
	if value := strings.TrimSpace(values["NECA_WEB_IDLE_TIMEOUT"]); value != "" {
		cfg.IdleTimeoutRaw = value
	}
	if value := strings.TrimSpace(values["NECA_WEB_ADMIN_CNS"]); value != "" {
		for _, cn := range strings.Split(value, ",") {
			if cn = strings.TrimSpace(cn); cn != "" {
				cfg.AdminCNs = append(cfg.AdminCNs, cn)
			}
		}
	}
	if value, ok := values["NECA_WEB_PASSKEY_ENABLED"]; ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
			cfg.Passkey.Enabled = parsed
		}
	}
	if value := strings.TrimSpace(values["NECA_WEB_PASSKEY_RPID"]); value != "" {
		cfg.Passkey.RPID = value
	}
	if value := strings.TrimSpace(values["NECA_WEB_PASSKEY_RPNAME"]); value != "" {
		cfg.Passkey.RPName = value
	}
	return cfg
}

// IdleTimeout 解析 idleTimeout 并校验 5m–24h 范围。
func (w WebConfig) IdleTimeout() (time.Duration, error) {
	raw := strings.TrimSpace(w.IdleTimeoutRaw)
	if raw == "" {
		return DefaultWebIdleTimeout, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("web.idleTimeout 非法: %q", w.IdleTimeoutRaw)
	}
	if timeout < MinWebIdleTimeout || timeout > MaxWebIdleTimeout {
		return 0, fmt.Errorf("web.idleTimeout 必须在 %s–%s 之间: %q", MinWebIdleTimeout, MaxWebIdleTimeout, w.IdleTimeoutRaw)
	}
	return timeout, nil
}

// Validate 校验 Web 配置的安全边界。
func (w WebConfig) Validate() error {
	host, port, err := net.SplitHostPort(w.Listen)
	if err != nil || port == "" {
		return usagef("Web 配置", "web.listen 必须是 host:port: %q", w.Listen)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return usagef("Web 配置", "web.listen 不能监听所有地址，必须绑定 127.0.0.1 或 ::1: %q", w.Listen)
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return usagef("Web 配置", "web.listen 必须绑定回环地址（127.0.0.1/::1），实际为 %q", host)
		}
	}
	if w.SessionEpoch < 0 {
		return usagef("Web 配置", "web.sessionEpoch 不能为负数: %d", w.SessionEpoch)
	}
	if _, err := w.IdleTimeout(); err != nil {
		return usagef("Web 配置", "%v", err)
	}
	for _, cn := range w.AdminCNs {
		if err := validateCN(cn); err != nil {
			return usagef("Web 配置", "web.adminCNs 含非法 CN %q", cn)
		}
	}
	if w.Passkey.Enabled {
		if strings.TrimSpace(w.Passkey.RPID) == "" {
			return usagef("Web 配置", "passkey.enabled=true 时必须设置 passkey.rpId")
		}
		if strings.ContainsAny(w.Passkey.RPID, "/: ") {
			return usagef("Web 配置", "passkey.rpId 必须是域名或 localhost，不能包含协议/端口: %q", w.Passkey.RPID)
		}
	}
	return nil
}

// webFileConfig 是 JSON 文件中的 web 对象；布尔值用指针区分“未设置”和 false。
type webFileConfig struct {
	Enabled      *bool                `json:"enabled,omitempty"`
	Listen       string               `json:"listen,omitempty"`
	ReadOnly     *bool                `json:"readOnly,omitempty"`
	SessionEpoch int                  `json:"sessionEpoch,omitempty"`
	IdleTimeout  string               `json:"idleTimeout,omitempty"`
	AdminCNs     []string             `json:"adminCNs,omitempty"`
	Passkey      webPasskeyFileConfig `json:"passkey,omitempty"`
}

type webPasskeyFileConfig struct {
	Enabled *bool  `json:"enabled,omitempty"`
	RPID    string `json:"rpId,omitempty"`
	RPName  string `json:"rpName,omitempty"`
}

// webConfigFromFile 将 JSON 对象转换为带安全默认值的 WebConfig。
func webConfigFromFile(file webFileConfig) WebConfig {
	cfg := defaultWebConfig()
	if file.Enabled != nil {
		cfg.Enabled = *file.Enabled
	}
	if strings.TrimSpace(file.Listen) != "" {
		cfg.Listen = strings.TrimSpace(file.Listen)
	}
	if file.ReadOnly != nil {
		cfg.ReadOnly = *file.ReadOnly
	}
	cfg.SessionEpoch = file.SessionEpoch
	if strings.TrimSpace(file.IdleTimeout) != "" {
		cfg.IdleTimeoutRaw = strings.TrimSpace(file.IdleTimeout)
	}
	if file.AdminCNs != nil {
		cfg.AdminCNs = append([]string(nil), file.AdminCNs...)
	}
	if file.Passkey.Enabled != nil {
		cfg.Passkey.Enabled = *file.Passkey.Enabled
	}
	if strings.TrimSpace(file.Passkey.RPID) != "" {
		cfg.Passkey.RPID = strings.TrimSpace(file.Passkey.RPID)
	}
	if strings.TrimSpace(file.Passkey.RPName) != "" {
		cfg.Passkey.RPName = strings.TrimSpace(file.Passkey.RPName)
	}
	return cfg
}

// webFileFromConfig 将运行时配置还原为 JSON 对象。
func webFileFromConfig(cfg WebConfig) webFileConfig {
	enabled, readOnly := cfg.Enabled, cfg.ReadOnly
	passkeyEnabled := cfg.Passkey.Enabled
	adminCNs := append([]string{}, cfg.AdminCNs...)
	return webFileConfig{
		Enabled:      &enabled,
		Listen:       cfg.Listen,
		ReadOnly:     &readOnly,
		SessionEpoch: cfg.SessionEpoch,
		IdleTimeout:  cfg.IdleTimeoutRaw,
		AdminCNs:     adminCNs,
		Passkey: webPasskeyFileConfig{
			Enabled: &passkeyEnabled,
			RPID:    cfg.Passkey.RPID,
			RPName:  cfg.Passkey.RPName,
		},
	}
}

// UpdateWebConfig 在配置锁内读-改-写 neca.json 的 web 段。
//
// 保留顶层未知字段与原有文件 mode/属主/组（详见 WriteConfigFile）。
// mutate 返回错误时文件保持原样，避免 readonly 切换出现半状态。
func UpdateWebConfig(path string, mutate func(*WebConfig) error) (*WebConfig, error) {
	if strings.TrimSpace(path) == "" {
		return nil, usagef("更新 Web 配置", "未指定配置文件")
	}
	var updated WebConfig
	err := withPkiLock(path+".lock", func() error {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return preconditionf("更新 Web 配置", "配置文件不存在: %s（请先 neca config init）", path)
		}
		if err != nil {
			return runtimef("更新 Web 配置", "读取 %s: %v", path, err)
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(data, &root); err != nil {
			return usagef("更新 Web 配置", "%s: %v", path, err)
		}
		var document struct {
			SchemaVersion int           `json:"schemaVersion"`
			Web           webFileConfig `json:"web"`
		}
		if err := json.Unmarshal(data, &document); err != nil {
			return usagef("更新 Web 配置", "%s: %v", path, err)
		}
		if document.SchemaVersion < 0 || document.SchemaVersion > 1 {
			return usagef("更新 Web 配置", "%s: 不支持的 schemaVersion=%d（当前为 1）", path, document.SchemaVersion)
		}
		web := webConfigFromFile(document.Web)
		if err := mutate(&web); err != nil {
			return err
		}
		if err := web.Validate(); err != nil {
			return err
		}
		raw, err := json.Marshal(webFileFromConfig(web))
		if err != nil {
			return runtimef("更新 Web 配置", "%v", err)
		}
		root["web"] = raw
		output, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return runtimef("更新 Web 配置", "%v", err)
		}
		output = append(output, '\n')
		if err := WriteConfigFile(path, output); err != nil {
			return err
		}
		updated = web
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// UpdateWebReadOnly 是 readonly on/off 的专用入口，避免调用方直接改结构。
func UpdateWebReadOnly(path string, readOnly bool) (*WebConfig, error) {
	return UpdateWebConfig(path, func(cfg *WebConfig) error {
		cfg.ReadOnly = readOnly
		return nil
	})
}

// UpdateWebIdleTimeout 是 idle-timeout 的专用入口。
func UpdateWebIdleTimeout(path, raw string) (*WebConfig, error) {
	timeout, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return nil, usagef("设置空闲超时", "非法时长: %q", raw)
	}
	if timeout < MinWebIdleTimeout || timeout > MaxWebIdleTimeout {
		return nil, usagef("设置空闲超时", "必须在 %s–%s 之间: %q", MinWebIdleTimeout, MaxWebIdleTimeout, raw)
	}
	return UpdateWebConfig(path, func(cfg *WebConfig) error {
		cfg.IdleTimeoutRaw = timeout.String()
		return nil
	})
}
