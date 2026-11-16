package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DefaultConfigFile 是未设置 NECA_CONFIG 时读取的默认 JSON 配置路径。
const DefaultConfigFile = "/opt/ipsec/neca.json"

// fileConfig 是 neca.json 的 schema；只存路径与普通配置，不存私钥/密码。
type fileConfig struct {
	SchemaVersion int    `json:"schemaVersion"`
	ServerID      string `json:"serverId,omitempty"`
	Org           string `json:"org,omitempty"`
	CACN          string `json:"caCn,omitempty"`
	Language      string `json:"language,omitempty"`
	AuditLog      string `json:"auditLog,omitempty"`
	Audit         struct {
		MaxBytes int64 `json:"maxBytes,omitempty"`
		Keep     int   `json:"keep,omitempty"`
	} `json:"audit"`
	Paths struct {
		PKIDir     string `json:"pkiDir,omitempty"`
		CACert     string `json:"caCert,omitempty"`
		CAKey      string `json:"caKey,omitempty"`
		CADBDir    string `json:"caDbDir,omitempty"`
		IndexFile  string `json:"indexFile,omitempty"`
		CRLNumFile string `json:"crlNumFile,omitempty"`
		CAConf     string `json:"caConf,omitempty"`
		CRLDir     string `json:"crlDir,omitempty"`
		CRLFile    string `json:"crlFile,omitempty"`
		ClientsDir string `json:"clientsDir,omitempty"`
		MetaDir    string `json:"metaDir,omitempty"`
		ArchiveDir string `json:"archiveDir,omitempty"`
		ExportDir  string `json:"exportDir,omitempty"`
		LockFile   string `json:"lockFile,omitempty"`
	} `json:"paths"`
	Defaults struct {
		Days       int    `json:"days,omitempty"`
		KeyBits    int    `json:"keyBits,omitempty"`
		P12Profile string `json:"p12Profile,omitempty"`
		Installers string `json:"installers,omitempty"`
	} `json:"defaults"`
	Reload struct {
		Mode      string `json:"mode,omitempty"`
		Container string `json:"container,omitempty"`
		DockerBin string `json:"dockerBin,omitempty"`
	} `json:"reload"`
	Download struct {
		Dir      string `json:"dir,omitempty"`
		TTL      string `json:"ttl,omitempty"`
		MaxFiles int    `json:"maxFiles,omitempty"`
	} `json:"download"`
	Trash struct {
		Retention string `json:"retention,omitempty"`
	} `json:"trash"`
	Web webFileConfig `json:"web,omitempty"`
}

// ConfigFilePath 返回当前配置路径：NECA_CONFIG 优先，否则默认路径。
func ConfigFilePath() string {
	if path := strings.TrimSpace(os.Getenv("NECA_CONFIG")); path != "" {
		return path
	}
	return DefaultConfigFile
}

// JSONSource 把 neca.json 映射为内部 KEY=VALUE 层，供 ports.Resolver 使用。
type JSONSource struct {
	Path string
}

// Name 返回带路径的来源名。
func (s JSONSource) Name() string { return "json:" + s.Path }

// Load 读取 JSON；文件不存在返回空层，便于默认路径可选。
func (s JSONSource) Load(ctx context.Context) (map[string]string, error) {
	cfg, err := readFileConfig(s.Path)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return map[string]string{}, nil
	}
	values := map[string]string{}
	set := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			values[key] = value
		}
	}
	set("SERVER_ID", cfg.ServerID)
	set("ORG", cfg.Org)
	set("CA_CN", cfg.CACN)
	set("NECA_LANG", cfg.Language)
	set("NECA_AUDIT_LOG", cfg.AuditLog)
	if cfg.Audit.MaxBytes > 0 {
		set("NECA_AUDIT_MAX_BYTES", strconv.FormatInt(cfg.Audit.MaxBytes, 10))
	}
	if cfg.Audit.Keep > 0 {
		set("NECA_AUDIT_KEEP", strconv.Itoa(cfg.Audit.Keep))
	}
	set("PKI_DIR", cfg.Paths.PKIDir)
	set("CA_CERT", cfg.Paths.CACert)
	set("CA_KEY", cfg.Paths.CAKey)
	set("CADB_DIR", cfg.Paths.CADBDir)
	set("INDEX_FILE", cfg.Paths.IndexFile)
	set("CRLNUM_FILE", cfg.Paths.CRLNumFile)
	set("CA_CONF", cfg.Paths.CAConf)
	set("CRL_DIR", cfg.Paths.CRLDir)
	set("CRL_FILE", cfg.Paths.CRLFile)
	set("CLIENTS_DIR", cfg.Paths.ClientsDir)
	set("META_DIR", cfg.Paths.MetaDir)
	set("ARCHIVE_DIR", cfg.Paths.ArchiveDir)
	set("EXPORT_DIR", cfg.Paths.ExportDir)
	set("NECA_LOCK_FILE", cfg.Paths.LockFile)
	set("NECA_RELOAD_MODE", cfg.Reload.Mode)
	set("CONTAINER", cfg.Reload.Container)
	set("DOCKER_BIN", cfg.Reload.DockerBin)
	set("NECA_DOWNLOAD_DIR", cfg.Download.Dir)
	set("NECA_DOWNLOAD_TTL", cfg.Download.TTL)
	set("NECA_TRASH_RETENTION", cfg.Trash.Retention)
	set("NECA_P12_PROFILE", cfg.Defaults.P12Profile)
	set("NECA_INSTALLERS", cfg.Defaults.Installers)
	if cfg.Web.Enabled != nil {
		set("NECA_WEB_ENABLED", strconv.FormatBool(*cfg.Web.Enabled))
	}
	set("NECA_WEB_LISTEN", cfg.Web.Listen)
	if cfg.Web.ReadOnly != nil {
		set("NECA_WEB_READONLY", strconv.FormatBool(*cfg.Web.ReadOnly))
	}
	if cfg.Web.SessionEpoch != 0 {
		set("NECA_WEB_SESSION_EPOCH", strconv.Itoa(cfg.Web.SessionEpoch))
	}
	if cfg.Web.ExportEnabled {
		set("NECA_WEB_EXPORT_ENABLED", "true")
	}
	if cfg.Web.AllowHighRiskSettings {
		set("NECA_WEB_ALLOW_HIGH_RISK_SETTINGS", "true")
	}
	if len(cfg.Web.Features) > 0 {
		set("NECA_WEB_FEATURES", featureEnv(cfg.Web.Features))
	}
	set("NECA_WEB_IDLE_TIMEOUT", cfg.Web.IdleTimeout)
	if len(cfg.Web.AdminCNs) > 0 {
		set("NECA_WEB_ADMIN_CNS", strings.Join(cfg.Web.AdminCNs, ","))
	}
	if cfg.Web.Passkey.Enabled != nil {
		set("NECA_WEB_PASSKEY_ENABLED", strconv.FormatBool(*cfg.Web.Passkey.Enabled))
	}
	set("NECA_WEB_PASSKEY_RPID", cfg.Web.Passkey.RPID)
	set("NECA_WEB_PASSKEY_RPNAME", cfg.Web.Passkey.RPName)
	if cfg.Defaults.Days > 0 {
		set("DEFAULT_DAYS", strconv.Itoa(cfg.Defaults.Days))
	}
	if cfg.Defaults.KeyBits > 0 {
		set("KEY_BITS", strconv.Itoa(cfg.Defaults.KeyBits))
	}
	if cfg.Download.MaxFiles > 0 {
		set("NECA_DOWNLOAD_MAX_FILES", strconv.Itoa(cfg.Download.MaxFiles))
	}
	return values, nil
}

func readFileConfig(path string) (*fileConfig, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, runtimef("读取配置", "%s: %v", path, err)
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, usagef("解析配置", "%s: %v", path, err)
	}
	if cfg.SchemaVersion < 0 || cfg.SchemaVersion > 1 {
		return nil, usagef("解析配置", "%s: 不支持的 schemaVersion=%d（当前为 1）", path, cfg.SchemaVersion)
	}
	return &cfg, nil
}

// ReadConfigLanguage 只读取配置中的 language，供 CLI 在加载完整配置前选择语言。
func ReadConfigLanguage(path string) (string, error) {
	cfg, err := readFileConfig(path)
	if err != nil || cfg == nil {
		return "", err
	}
	return cfg.Language, nil
}

func featureEnv(features map[string]bool) string {
	if len(features) == 0 {
		return ""
	}
	keys := make([]string, 0, len(features))
	for key := range features {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.FormatBool(features[key]))
	}
	return strings.Join(parts, ",")
}

// ConfigSnapshot 返回可安全展示的有效配置（只有路径/开关，无密钥内容）。
func ConfigSnapshot(cfg *Config) map[string]any {
	if cfg == nil {
		return nil
	}
	return map[string]any{
		"configFile":       cfg.ConfigFile,
		"pkiDir":           cfg.PKIDir,
		"caCert":           cfg.CACert,
		"caKey":            cfg.CAKey,
		"caDbDir":          cfg.CADBDir,
		"indexFile":        cfg.IndexFile,
		"crlNumFile":       cfg.CRLNumFile,
		"caConf":           cfg.CAConf,
		"crlDir":           cfg.CRLDir,
		"crlFile":          cfg.CRLFile,
		"clientsDir":       cfg.ClientsDir,
		"metaDir":          cfg.MetaDir,
		"archiveDir":       cfg.ArchiveDir,
		"exportDir":        cfg.ExportDir,
		"container":        cfg.Container,
		"dockerBin":        cfg.DockerBin,
		"serverId":         cfg.ServerID,
		"org":              cfg.ORG,
		"caCn":             cfg.CACN,
		"defaultDays":      cfg.DefaultDays,
		"keyBits":          cfg.KeyBits,
		"reloadMode":       cfg.ReloadMode,
		"lockFile":         cfg.LockFile,
		"downloadsDir":     cfg.DownloadsDir,
		"downloadTTL":      cfg.DownloadTTL.String(),
		"downloadMaxFiles": cfg.DownloadMaxFiles,
		"trashRetention":   cfg.TrashRetention.String(),
		"p12Profile":       cfg.P12Profile,
		"installers":       cfg.Installers,
		"auditFile":        cfg.AuditFile,
		"auditMaxBytes":    cfg.AuditMaxBytes,
		"auditKeep":        cfg.AuditKeep,
		"language":         cfg.Language,
		"web": map[string]any{
			"enabled":               cfg.Web.Enabled,
			"listen":                cfg.Web.Listen,
			"readOnly":              cfg.Web.ReadOnly,
			"sessionEpoch":          cfg.Web.SessionEpoch,
			"exportEnabled":         cfg.Web.ExportEnabled,
			"features":              cfg.Web.Features,
			"allowHighRiskSettings": cfg.Web.AllowHighRiskSettings,
			"idleTimeout":           cfg.Web.IdleTimeoutRaw,
			"adminCNs":              cfg.Web.AdminCNs,
			"passkey": map[string]any{
				"enabled": cfg.Web.Passkey.Enabled,
				"rpId":    cfg.Web.Passkey.RPID,
				"rpName":  cfg.Web.Passkey.RPName,
			},
		},
	}
}

// ExampleConfigJSON 生成不含生产值的配置模板（用于 config init）。
func ExampleConfigJSON() ([]byte, error) {
	cfg := &Config{}
	cfg.applyDefaults()
	cfg.ConfigFile = DefaultConfigFile
	return ConfigFileJSON(cfg)
}

// ConfigEnvMap 返回与 configEnvKeys 对应的有效设置，用于 config export-env。
func ConfigEnvMap(cfg *Config) map[string]string {
	if cfg == nil {
		return nil
	}
	return map[string]string{
		"PKI_DIR": cfg.PKIDir, "CA_CERT": cfg.CACert, "CA_KEY": cfg.CAKey,
		"CADB_DIR": cfg.CADBDir, "INDEX_FILE": cfg.IndexFile, "CRLNUM_FILE": cfg.CRLNumFile,
		"CA_CONF": cfg.CAConf, "CRL_DIR": cfg.CRLDir, "CRL_FILE": cfg.CRLFile,
		"CLIENTS_DIR": cfg.ClientsDir, "META_DIR": cfg.MetaDir, "ARCHIVE_DIR": cfg.ArchiveDir,
		"EXPORT_DIR": cfg.ExportDir, "CONTAINER": cfg.Container, "DOCKER_BIN": cfg.DockerBin,
		"SERVER_ID": cfg.ServerID, "ORG": cfg.ORG, "CA_CN": cfg.CACN,
		"DEFAULT_DAYS": strconv.Itoa(cfg.DefaultDays), "KEY_BITS": strconv.Itoa(cfg.KeyBits),
		"NECA_RELOAD_MODE": cfg.ReloadMode, "NECA_LOCK_FILE": cfg.LockFile,
		"NECA_DOWNLOAD_DIR": cfg.DownloadsDir, "NECA_DOWNLOAD_TTL": cfg.DownloadTTL.String(),
		"NECA_DOWNLOAD_MAX_FILES": strconv.Itoa(cfg.DownloadMaxFiles), "NECA_TRASH_RETENTION": cfg.TrashRetention.String(),
		"NECA_P12_PROFILE": string(cfg.P12Profile), "NECA_INSTALLERS": cfg.Installers,
		"NECA_AUDIT_LOG": cfg.AuditFile, "NECA_AUDIT_MAX_BYTES": strconv.FormatInt(cfg.AuditMaxBytes, 10), "NECA_AUDIT_KEEP": strconv.Itoa(cfg.AuditKeep), "NECA_LANG": cfg.Language,
		"NECA_WEB_ENABLED": strconv.FormatBool(cfg.Web.Enabled),
		"NECA_WEB_LISTEN":  cfg.Web.Listen, "NECA_WEB_READONLY": strconv.FormatBool(cfg.Web.ReadOnly),
		"NECA_WEB_IDLE_TIMEOUT": cfg.Web.IdleTimeoutRaw, "NECA_WEB_SESSION_EPOCH": strconv.Itoa(cfg.Web.SessionEpoch),
		"NECA_WEB_EXPORT_ENABLED":           strconv.FormatBool(cfg.Web.ExportEnabled),
		"NECA_WEB_ALLOW_HIGH_RISK_SETTINGS": strconv.FormatBool(cfg.Web.AllowHighRiskSettings),
		"NECA_WEB_FEATURES":                 featureEnv(cfg.Web.Features),
		"NECA_WEB_ADMIN_CNS":                strings.Join(cfg.Web.AdminCNs, ","),
		"NECA_WEB_PASSKEY_ENABLED":          strconv.FormatBool(cfg.Web.Passkey.Enabled),
		"NECA_WEB_PASSKEY_RPID":             cfg.Web.Passkey.RPID, "NECA_WEB_PASSKEY_RPNAME": cfg.Web.Passkey.RPName,
	}
}

// ConfigFileJSON 生成用于 config init --from-env 的 JSON 文本。
func ConfigFileJSON(cfg *Config) ([]byte, error) {
	fc := fileConfig{SchemaVersion: 1}
	if cfg != nil {
		fc.ServerID = cfg.ServerID
		fc.Org = cfg.ORG
		fc.CACN = cfg.CACN
		fc.Language = cfg.Language
		fc.AuditLog = cfg.AuditFile
		fc.Audit.MaxBytes = cfg.AuditMaxBytes
		fc.Audit.Keep = cfg.AuditKeep
		fc.Paths.PKIDir = cfg.PKIDir
		fc.Paths.CACert = cfg.CACert
		fc.Paths.CAKey = cfg.CAKey
		fc.Paths.CADBDir = cfg.CADBDir
		fc.Paths.IndexFile = cfg.IndexFile
		fc.Paths.CRLNumFile = cfg.CRLNumFile
		fc.Paths.CAConf = cfg.CAConf
		fc.Paths.CRLDir = cfg.CRLDir
		fc.Paths.CRLFile = cfg.CRLFile
		fc.Paths.ClientsDir = cfg.ClientsDir
		fc.Paths.MetaDir = cfg.MetaDir
		fc.Paths.ArchiveDir = cfg.ArchiveDir
		fc.Paths.ExportDir = cfg.ExportDir
		fc.Paths.LockFile = cfg.LockFile
		fc.Defaults.Days = cfg.DefaultDays
		fc.Defaults.KeyBits = cfg.KeyBits
		fc.Defaults.P12Profile = string(cfg.P12Profile)
		fc.Defaults.Installers = cfg.Installers
		fc.Reload.Mode = cfg.ReloadMode
		fc.Reload.Container = cfg.Container
		fc.Reload.DockerBin = cfg.DockerBin
		fc.Download.Dir = cfg.DownloadsDir
		fc.Download.TTL = cfg.DownloadTTL.String()
		fc.Download.MaxFiles = cfg.DownloadMaxFiles
		if cfg.TrashRetention > 0 {
			fc.Trash.Retention = cfg.TrashRetention.String()
		}
		fc.Web = webFileFromConfig(cfg.Web)
	}
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return nil, runtimef("生成配置", "%v", err)
	}
	return append(data, '\n'), nil
}

// WriteConfigFile 原子写入 JSON 配置；目录 0700，新文件 0600。
//
// 覆盖已有文件时保留其 mode 与属主/组：启用 Web 后配置通常是
// root:neca-web 0640，CLI 重写后不能退化成 root:root 0600。
func WriteConfigFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return runtimef("写入配置", "创建目录 %s: %v", dir, err)
	}
	var existing fs.FileInfo
	if info, err := os.Stat(path); err == nil {
		existing = info
	} else if !errors.Is(err, fs.ErrNotExist) {
		return runtimef("写入配置", "读取 %s: %v", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".neca-*.json")
	if err != nil {
		return runtimef("写入配置", "%v", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return runtimef("写入配置", "%v", err)
	}
	mode := fs.FileMode(0o600)
	if existing != nil {
		mode = existing.Mode().Perm()
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return runtimef("写入配置", "%v", err)
	}
	if err := preserveOwnership(tmp, existing); err != nil {
		tmp.Close()
		return runtimef("写入配置", "恢复属主: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return runtimef("写入配置", "%v", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return runtimef("写入配置", "%v", err)
	}
	return nil
}

var _ = fmt.Sprintf
