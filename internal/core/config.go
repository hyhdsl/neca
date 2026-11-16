package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// 默认值集中在这里，对应旧脚本顶部的 ${VAR:-default}（开发文档 §7）。
// 环境变量为空字符串时按"未设置"处理，与 bash 的 :- 语义一致。
const (
	defaultPKIDir          = "/opt/ipsec/pki"
	defaultCRLDir          = "/opt/ipsec/swanctl/x509crl"
	defaultClientsDir      = "/opt/ipsec/clients"
	defaultExportDir       = "/opt/ipsec/exports"
	defaultContainer       = "ipsec-vpn"
	defaultDockerBin       = "docker"
	defaultServerID        = "vpn.example.com"
	defaultCACN            = "ExampleOrg VPN CA"
	defaultORG             = "ExampleOrg"
	defaultDays            = 365
	defaultKeyBits         = 3072
	defaultReloadMode      = "creds"
	defaultDownloadsDir    = "/opt/ipsec/downloads"
	defaultDownloadTTL     = 30 * time.Minute
	defaultDownloadMaxFile = 50
)

// configEnvKeys 是允许从环境变量覆盖的键，名称与旧脚本完全一致。
var configEnvKeys = []string{
	"PKI_DIR", "CA_CERT", "CA_KEY", "CADB_DIR", "INDEX_FILE", "CRLNUM_FILE",
	"CA_CONF", "CRL_DIR", "CRL_FILE", "CLIENTS_DIR", "META_DIR", "ARCHIVE_DIR",
	"EXPORT_DIR", "CONTAINER", "DOCKER_BIN", "SERVER_ID", "ORG", "CA_CN",
	"DEFAULT_DAYS", "KEY_BITS", "NECA_RELOAD_MODE", "NECA_LOCK_FILE",
	"NECA_DOWNLOAD_DIR", "NECA_DOWNLOAD_TTL", "NECA_DOWNLOAD_MAX_FILES",
	"NECA_P12_PROFILE", "NECA_INSTALLERS", "NECA_AUDIT_LOG", "NECA_LANG",
	"NECA_WEB_ENABLED", "NECA_WEB_LISTEN", "NECA_WEB_READONLY",
	"NECA_WEB_IDLE_TIMEOUT", "NECA_WEB_SESSION_EPOCH", "NECA_WEB_EXPORT_ENABLED",
	"NECA_WEB_ALLOW_HIGH_RISK_SETTINGS", "NECA_WEB_ADMIN_CNS",
	"NECA_WEB_PASSKEY_ENABLED", "NECA_WEB_PASSKEY_RPID", "NECA_WEB_PASSKEY_RPNAME",
}

// Config 是 neca 的全部运行时路径与开关。
//
// 架构约束：路径与开关只在这里出现一次，其他包一律通过 Config 取值，
// 不直接读环境变量、不硬编码 /opt/ipsec。
type Config struct {
	PKIDir      string // CA 与客户端证书根目录
	CACert      string // CA 证书
	CAKey       string // CA 私钥（仅 root 可读）
	CADBDir     string // openssl ca 数据库目录
	IndexFile   string // index.txt，CRL 的唯一事实源
	CRLNumFile  string // crlnumber
	CAConf      string // openssl ca 配置
	CRLDir      string // strongSwan 的 x509crl 目录
	CRLFile     string // ca.crl.pem
	ClientsDir  string // 客户端包根目录
	MetaDir     string // 元数据目录
	ArchiveDir  string // 归档目录
	ExportDir   string // CSV 导出目录
	Container   string // strongSwan 容器名
	DockerBin   string // docker 可执行文件
	ServerID    string // VPN 服务器标识
	CACN        string // CA Common Name
	ORG         string // 证书 O 字段
	DefaultDays int    // 默认有效期天数
	KeyBits     int    // RSA 位数
	ReloadMode  string // creds | restart（M2 使用）
	LockFile    string // 全局 flock 文件

	DownloadsDir     string        // zip 下载目录
	DownloadTTL      time.Duration // zip 有效期
	DownloadMaxFiles int           // zip 数量上限
	P12Profile       P12Profile    // compat | modern
	Installers       string        // none | all | 平台列表
	AuditFile        string        // 追加式审计日志
	Language         string        // 控制台语言偏好
	ConfigFile       string        // 实际读取的 JSON 配置路径
	Web              WebConfig     // Web 面板配置；lite 只读取不使用
}

// EnvSource 从环境变量读取配置层。
//
// 空值按未设置处理（与旧脚本 ${VAR:-default} 一致）；Lookup 可注入，
// 测试不必修改进程环境。
type EnvSource struct {
	Lookup func(string) (string, bool)
}

// Name 返回来源名。
func (EnvSource) Name() string { return "env" }

// Load 读取所有已知键中非空的环境变量。
func (s EnvSource) Load(ctx context.Context) (map[string]string, error) {
	lookup := s.Lookup
	if lookup == nil {
		lookup = os.LookupEnv
	}
	values := make(map[string]string, len(configEnvKeys))
	for _, key := range configEnvKeys {
		if value, ok := lookup(key); ok && value != "" {
			values[key] = value
		}
	}
	return values, nil
}

// FileSource 读取 KEY=VALUE 配置文件（每行一条，# 开头为注释）。
//
// M1 只实现并测试该来源，--config 开关留到 M3 的通用参数解析一起接入。
type FileSource struct {
	Path string
}

// Name 返回带路径的来源名，便于错误定位。
func (s FileSource) Name() string { return "file:" + s.Path }

// Load 解析配置文件；不存在的文件返回空层而不是错误，方便可选配置。
func (s FileSource) Load(ctx context.Context) (map[string]string, error) {
	file, err := os.Open(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		record := strings.TrimSpace(scanner.Text())
		if record == "" || strings.HasPrefix(record, "#") {
			continue
		}
		key, value, ok := strings.Cut(record, "=")
		if !ok {
			return nil, fmt.Errorf("第 %d 行缺少 '=': %s", line, record)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// LoadConfig 从环境变量与 JSON 配置文件加载配置。
//
// 优先级：环境变量 > JSON 文件 > 默认值；--config 由 CLI 写入 NECA_CONFIG。
func LoadConfig() (*Config, error) {
	return loadConfig(true)
}

// LoadConfigReadOnly 与 LoadConfig 使用同一优先级，但不创建/修改任何目录或文件。
//
// necad 以非 root 身份运行，且 PKI/配置通常以只读方式挂载；它只需要读取
// 配置和 CA 公钥，不能调用 ensureLayout 的 chmod/MkdirAll。CLI 仍使用
// LoadConfig 保持 M1–M5 的初始化行为。
func LoadConfigReadOnly() (*Config, error) {
	return loadConfig(false)
}

func loadConfig(initLayout bool) (*Config, error) {
	path := ConfigFilePath()
	sources := []ports.Source{EnvSource{}}
	if path != "" {
		sources = append(sources, JSONSource{Path: path})
	}
	cfg, err := loadConfigFrom(sources, initLayout)
	if cfg != nil {
		cfg.ConfigFile = path
	}
	return cfg, err
}

// LoadConfigFrom 从给定来源加载配置，先注册的优先（ports.Resolver 语义）。
//
// 加载后依次补默认值、创建目录骨架、校验 CA 证书；任何一步失败都返回
// 带类别的 core.Error，由 CLI 映射为退出码。
func LoadConfigFrom(sources ...ports.Source) (*Config, error) {
	return loadConfigFrom(sources, true)
}

func loadConfigFrom(sources []ports.Source, initLayout bool) (*Config, error) {
	resolver := ports.NewResolver(sources...)
	values, err := resolver.Resolve(context.Background())
	if err != nil {
		return nil, runtimef("加载配置", "%v", err)
	}

	cfg := &Config{
		PKIDir:     values["PKI_DIR"],
		CACert:     values["CA_CERT"],
		CAKey:      values["CA_KEY"],
		CADBDir:    values["CADB_DIR"],
		IndexFile:  values["INDEX_FILE"],
		CRLNumFile: values["CRLNUM_FILE"],
		CAConf:     values["CA_CONF"],
		CRLDir:     values["CRL_DIR"],
		CRLFile:    values["CRL_FILE"],
		ClientsDir: values["CLIENTS_DIR"],
		MetaDir:    values["META_DIR"],
		ArchiveDir: values["ARCHIVE_DIR"],
		ExportDir:  values["EXPORT_DIR"],
		Container:  values["CONTAINER"],
		DockerBin:  values["DOCKER_BIN"],
		ServerID:   values["SERVER_ID"],
		CACN:       values["CA_CN"],
		ORG:        values["ORG"],
		ReloadMode: values["NECA_RELOAD_MODE"],
		LockFile:   values["NECA_LOCK_FILE"],

		DownloadsDir: values["NECA_DOWNLOAD_DIR"],
		Installers:   values["NECA_INSTALLERS"],
		AuditFile:    values["NECA_AUDIT_LOG"],
		Language:     values["NECA_LANG"],
		Web:          parseWebConfigValues(values),
	}
	cfg.applyDefaults()
	if err := cfg.parseNumbers(values); err != nil {
		return nil, err
	}
	if initLayout {
		if err := cfg.ensureLayout(); err != nil {
			return nil, err
		}
	} else if err := cfg.checkCACert(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyDefaults 在缺失时补齐默认值，并按 PKI_DIR 推导从属路径。
func (c *Config) applyDefaults() {
	if c.PKIDir == "" {
		c.PKIDir = defaultPKIDir
	}
	if c.CACert == "" {
		c.CACert = filepath.Join(c.PKIDir, "ca.crt")
	}
	if c.CAKey == "" {
		c.CAKey = filepath.Join(c.PKIDir, "ca.key")
	}
	if c.CADBDir == "" {
		c.CADBDir = filepath.Join(c.PKIDir, "ca")
	}
	if c.IndexFile == "" {
		c.IndexFile = filepath.Join(c.CADBDir, "index.txt")
	}
	if c.CRLNumFile == "" {
		c.CRLNumFile = filepath.Join(c.CADBDir, "crlnumber")
	}
	if c.CAConf == "" {
		c.CAConf = filepath.Join(c.CADBDir, "openssl.cnf")
	}
	if c.CRLDir == "" {
		c.CRLDir = defaultCRLDir
	}
	if c.CRLFile == "" {
		c.CRLFile = filepath.Join(c.CRLDir, "ca.crl.pem")
	}
	if c.ClientsDir == "" {
		c.ClientsDir = defaultClientsDir
	}
	if c.MetaDir == "" {
		c.MetaDir = filepath.Join(c.PKIDir, "meta")
	}
	if c.ArchiveDir == "" {
		c.ArchiveDir = filepath.Join(c.PKIDir, "archive")
	}
	if c.ExportDir == "" {
		c.ExportDir = defaultExportDir
	}
	if c.Container == "" {
		c.Container = defaultContainer
	}
	if c.DockerBin == "" {
		c.DockerBin = defaultDockerBin
	}
	if c.ServerID == "" {
		c.ServerID = defaultServerID
	}
	if c.CACN == "" {
		c.CACN = defaultCACN
	}
	if c.ORG == "" {
		c.ORG = defaultORG
	}
	if c.ReloadMode == "" {
		c.ReloadMode = defaultReloadMode
	}
	if c.LockFile == "" {
		c.LockFile = filepath.Join(c.PKIDir, ".lock")
	}
	if c.DownloadsDir == "" {
		c.DownloadsDir = defaultDownloadsDir
	}
	if c.DownloadMaxFiles <= 0 {
		c.DownloadMaxFiles = defaultDownloadMaxFile
	}
	if c.Installers == "" {
		c.Installers = "none"
	}
	if c.P12Profile == "" {
		c.P12Profile = P12Compat
	}
	if c.AuditFile == "" {
		c.AuditFile = filepath.Join(c.PKIDir, "audit.log")
	}
	if c.Language == "" {
		c.Language = "zh-CN"
	}
	// 零值 Config（例如 config init 的 ExampleConfigJSON）也必须带安全
	// Web 默认值：关闭、只读、仅回环监听。
	if strings.TrimSpace(c.Web.Listen) == "" {
		c.Web = defaultWebConfig()
	}
}

// parseNumbers 解析整数开关；非法值属于用法错误。
func (c *Config) parseNumbers(values map[string]string) error {
	c.DefaultDays = defaultDays
	if raw := values["DEFAULT_DAYS"]; raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return usagef("配置", "DEFAULT_DAYS 必须是正整数: %q", raw)
		}
		c.DefaultDays = n
	}
	c.KeyBits = defaultKeyBits
	if raw := values["KEY_BITS"]; raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 2048 {
			return usagef("配置", "KEY_BITS 必须 >= 2048: %q", raw)
		}
		c.KeyBits = n
	}
	c.DownloadTTL = defaultDownloadTTL
	if raw := values["NECA_DOWNLOAD_TTL"]; raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return usagef("配置", "NECA_DOWNLOAD_TTL 非法: %q", raw)
		}
		c.DownloadTTL = d
	}
	if raw := values["NECA_DOWNLOAD_MAX_FILES"]; raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return usagef("配置", "NECA_DOWNLOAD_MAX_FILES 必须是正整数: %q", raw)
		}
		c.DownloadMaxFiles = n
	}
	if raw := values["NECA_P12_PROFILE"]; raw != "" {
		profile, err := ParseP12Profile(raw)
		if err != nil {
			return usagef("配置", "%v", err)
		}
		c.P12Profile = profile
	}
	if raw := values["NECA_INSTALLERS"]; raw != "" {
		if _, err := ParseInstallers(raw); err != nil {
			return usagef("配置", "%v", err)
		}
		c.Installers = raw
	}
	return nil
}

// ensureLayout 复刻旧脚本 init_env() 的目录与文件初始化：
// 建目录、补空 index.txt、crlnumber=1000、默认 openssl.cnf，最后校验 CA 证书。
func (c *Config) ensureLayout() error {
	dirs := []string{c.PKIDir, c.CADBDir, c.CRLDir, c.ClientsDir, c.MetaDir, c.ArchiveDir, c.ExportDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return runtimef("创建目录", "%s: %v", dir, err)
		}
	}
	// 下载目录含 p12/pass，权限收紧到 0700。
	if err := os.MkdirAll(c.DownloadsDir, 0o700); err != nil {
		return runtimef("创建目录", "%s: %v", c.DownloadsDir, err)
	}
	if err := os.Chmod(c.DownloadsDir, 0o700); err != nil {
		return runtimef("设置目录权限", "%s: %v", c.DownloadsDir, err)
	}

	if err := ensureFile(c.IndexFile, ""); err != nil {
		return err
	}
	if err := ensureFile(c.CRLNumFile, "1000\n"); err != nil {
		return err
	}
	if err := ensureFile(c.CAConf, c.defaultCAConf()); err != nil {
		return err
	}
	return c.checkCACert()
}

// checkCACert 只校验 CA 公钥存在且可读；不做任何写入。
func (c *Config) checkCACert() error {
	if _, err := os.Stat(c.CACert); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return preconditionf("加载配置", "找不到 CA 证书: %s", c.CACert)
		}
		return runtimef("加载配置", "读取 CA 证书 %s: %v", c.CACert, err)
	}
	return nil
}

// ensureFile 在文件不存在时写入初始内容；已存在则保持原样。
func ensureFile(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return runtimef("初始化文件", "%s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return runtimef("初始化文件", "%s: %v", path, err)
	}
	return nil
}

// defaultCAConf 生成与旧脚本一致的最小 openssl ca 配置。
func (c *Config) defaultCAConf() string {
	return fmt.Sprintf(`[ ca ]
default_ca = CA_default
[ CA_default ]
dir = %s
database = %s
crlnumber = %s
certificate = %s
private_key = %s
default_md = sha256
default_crl_days = 3650
crl_extensions = crl_ext
[ crl_ext ]
authorityKeyIdentifier = keyid:always
`, c.CADBDir, c.IndexFile, c.CRLNumFile, c.CACert, c.CAKey)
}
