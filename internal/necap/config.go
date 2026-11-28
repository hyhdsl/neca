// 本文件加载 necap 独立配置并校验 RP/Origin、数据库、密钥和 socket 路径。
package necap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	CurrentConfigSchemaVersion = 1
	DefaultConfigPath          = "/opt/ipsec/necap/necap.json"
	DefaultSocketPath          = "/run/necap/necap.sock"
	DefaultDatabasePath        = "/opt/ipsec/necap/necap.db"
	DefaultMasterKeyPath       = "/opt/ipsec/necap/master.key"
)

// Config 是 necap 的独立配置；不包含密码、TOTP secret、私钥或数据库密钥内容。
type Config struct {
	SchemaVersion    int      `json:"schemaVersion"`
	RPID             string   `json:"rpId"`
	RPName           string   `json:"rpName"`
	Origins          []string `json:"origins"`
	UserVerification string   `json:"userVerification,omitempty"`
	DatabasePath     string   `json:"database"`
	MasterKeyFile    string   `json:"masterKeyFile"`
	SocketPath       string   `json:"socket"`
	ChallengeTTL     string   `json:"challengeTtl,omitempty"`
	SessionTTL       string   `json:"sessionTtl,omitempty"`
}

// RuntimeConfig 是完成默认值与时长解析后的配置。
type RuntimeConfig struct {
	Config
	ChallengeDuration time.Duration
	SessionDuration   time.Duration
}

// ConfigFilePath 返回 NECAP_CONFIG 或默认路径。
func ConfigFilePath() string {
	if value := strings.TrimSpace(os.Getenv("NECAP_CONFIG")); value != "" {
		return value
	}
	return DefaultConfigPath
}

// LoadConfig 从 NECAP_CONFIG/默认路径读取并严格校验配置。
func LoadConfig() (*RuntimeConfig, error) {
	path := ConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 necap 配置 %s: %w", path, err)
	}
	cfg, err := ParseConfig(data, path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// ParseConfig 解码并校验 necap 配置；未知字段、尾随内容和相对敏感路径都会失败。
func ParseConfig(data []byte, source string) (*RuntimeConfig, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("解析 necap 配置 %s: %w", source, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("解析 necap 配置 %s: 只允许一个 JSON 对象", source)
	}
	if cfg.SchemaVersion != CurrentConfigSchemaVersion {
		return nil, fmt.Errorf("解析 necap 配置 %s: 不支持的 schemaVersion=%d", source, cfg.SchemaVersion)
	}
	cfg.RPID = strings.TrimSpace(cfg.RPID)
	cfg.RPName = strings.TrimSpace(cfg.RPName)
	cfg.DatabasePath = strings.TrimSpace(cfg.DatabasePath)
	cfg.MasterKeyFile = strings.TrimSpace(cfg.MasterKeyFile)
	cfg.SocketPath = strings.TrimSpace(cfg.SocketPath)
	if cfg.RPID == "" || strings.ContainsAny(cfg.RPID, "/: ?#") {
		return nil, fmt.Errorf("解析 necap 配置 %s: rpId 非法", source)
	}
	if cfg.RPName == "" {
		return nil, fmt.Errorf("解析 necap 配置 %s: rpName 不能为空", source)
	}
	if len(cfg.Origins) == 0 {
		return nil, fmt.Errorf("解析 necap 配置 %s: origins 不能为空", source)
	}
	for i, origin := range cfg.Origins {
		normalized, err := validateOrigin(origin, cfg.RPID)
		if err != nil {
			return nil, fmt.Errorf("解析 necap 配置 %s: origins[%d]: %w", source, i, err)
		}
		cfg.Origins[i] = normalized
	}
	switch cfg.UserVerification {
	case "":
		cfg.UserVerification = "preferred"
	case "required", "preferred", "discouraged":
	default:
		return nil, fmt.Errorf("解析 necap 配置 %s: userVerification 只能是 required/preferred/discouraged", source)
	}
	for name, value := range map[string]string{
		"database": cfg.DatabasePath, "masterKeyFile": cfg.MasterKeyFile, "socket": cfg.SocketPath,
	} {
		if value == "" || !filepath.IsAbs(value) {
			return nil, fmt.Errorf("解析 necap 配置 %s: %s 必须是绝对路径", source, name)
		}
	}
	if cfg.DatabasePath == cfg.MasterKeyFile || cfg.DatabasePath == cfg.SocketPath || cfg.MasterKeyFile == cfg.SocketPath {
		return nil, fmt.Errorf("解析 necap 配置 %s: database/masterKeyFile/socket 必须是不同路径", source)
	}
	challengeTTL, err := parseDuration(cfg.ChallengeTTL, 5*time.Minute, "challengeTtl")
	if err != nil {
		return nil, fmt.Errorf("解析 necap 配置 %s: %w", source, err)
	}
	sessionTTL, err := parseDuration(cfg.SessionTTL, 10*time.Minute, "sessionTtl")
	if err != nil {
		return nil, fmt.Errorf("解析 necap 配置 %s: %w", source, err)
	}
	return &RuntimeConfig{Config: cfg, ChallengeDuration: challengeTTL, SessionDuration: sessionTTL}, nil
}

func validateOrigin(raw, rpID string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return "", fmt.Errorf("必须是完整 Origin，不能带路径、查询、片段或用户信息")
	}
	host := strings.ToLower(parsed.Hostname())
	if !strings.EqualFold(host, rpID) {
		return "", fmt.Errorf("host %q 与 rpId %q 不一致", host, rpID)
	}
	if parsed.Scheme == "https" {
		return strings.TrimSuffix(raw, "/"), nil
	}
	if parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return strings.TrimSuffix(raw, "/"), nil
	}
	return "", fmt.Errorf("只允许 https，或 localhost 的 http")
}

func parseDuration(raw string, fallback time.Duration, name string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s 必须是正时长", name)
	}
	return value, nil
}
