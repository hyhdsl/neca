// 本文件定义配置 schema 版本、v2 严格校验与 v1→v2 迁移。
package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"strings"
	"time"
)

// 配置 schema 版本。v1/0 保持宽容读取；v2 对未知字段和语义做严格校验。
const (
	ConfigSchemaV1             = 1
	ConfigSchemaV2             = 2
	CurrentConfigSchemaVersion = ConfigSchemaV2

	// configSchemaVersionKey 是 JSON 层写入内部值层的只读版本标记。
	configSchemaVersionKey = "NECA_CONFIG_SCHEMA_VERSION"
)

// configHeader 只读取 schemaVersion，用于选择 v1 宽容或 v2 严格解码。
type configHeader struct {
	SchemaVersion int `json:"schemaVersion"`
}

// ConfigMigration 描述一次 schema 迁移检查/写入的结果。
type ConfigMigration struct {
	Path           string
	From           int
	To             int
	NeedsMigration bool
	Written        bool
	BackupPath     string
}

func decodeFileConfig(data []byte, path string) (*fileConfig, error) {
	var header configHeader
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&header); err != nil {
		return nil, usagef("解析配置", "%s: %v", path, err)
	}
	switch header.SchemaVersion {
	case 0, ConfigSchemaV1:
		// v1 兼容：保留旧字段与未知扩展字段，不在这里引入更严格的限制。
		var cfg fileConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, usagef("解析配置", "%s: %v", path, err)
		}
		return &cfg, nil
	case ConfigSchemaV2:
		return decodeStrictFileConfig(data, path)
	default:
		return nil, usagef("解析配置", "%s: 不支持的 schemaVersion=%d（当前支持 1/2）", path, header.SchemaVersion)
	}
}

// decodeStrictFileConfig 是 v2 的唯一读取入口：拒绝未知字段、尾随内容和非法语义。
func decodeStrictFileConfig(data []byte, path string) (*fileConfig, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, usagef("解析配置", "%s: schema v2 严格校验失败: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var cfg fileConfig
	if err := decoder.Decode(&cfg); err != nil {
		return nil, usagef("解析配置", "%s: schema v2 严格校验失败: %v", path, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, usagef("解析配置", "%s: schema v2 只允许一个 JSON 对象", path)
		}
		return nil, usagef("解析配置", "%s: schema v2 尾随内容无效: %v", path, err)
	}
	if cfg.SchemaVersion != ConfigSchemaV2 {
		return nil, usagef("解析配置", "%s: schema v2 文件必须声明 schemaVersion=2", path)
	}
	if err := validateFileConfigV2(&cfg); err != nil {
		return nil, usagef("解析配置", "%s: %v", path, err)
	}
	return &cfg, nil
}

// validateFileConfigV2 校验 v2 的枚举、范围、时长与 Web 安全边界。
//
// 缺失值仍由 applyDefaults 补齐；这里只拒绝显式写错的值，避免把
// “未配置”误判为错误。
func validateFileConfigV2(cfg *fileConfig) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{"paths.pkiDir", cfg.Paths.PKIDir},
		{"paths.caCert", cfg.Paths.CACert},
		{"paths.caKey", cfg.Paths.CAKey},
		{"paths.caDbDir", cfg.Paths.CADBDir},
		{"paths.indexFile", cfg.Paths.IndexFile},
		{"paths.crlNumFile", cfg.Paths.CRLNumFile},
		{"paths.caConf", cfg.Paths.CAConf},
		{"paths.crlDir", cfg.Paths.CRLDir},
		{"paths.crlFile", cfg.Paths.CRLFile},
		{"paths.clientsDir", cfg.Paths.ClientsDir},
		{"paths.metaDir", cfg.Paths.MetaDir},
		{"paths.archiveDir", cfg.Paths.ArchiveDir},
		{"paths.exportDir", cfg.Paths.ExportDir},
		{"paths.lockFile", cfg.Paths.LockFile},
		{"auditLog", cfg.AuditLog},
		{"download.dir", cfg.Download.Dir},
	} {
		if err := validateConfigPath(field.name, field.value); err != nil {
			return err
		}
	}
	if cfg.Defaults.Days < 0 {
		return usagef("配置校验", "defaults.days 不能为负数: %d", cfg.Defaults.Days)
	}
	if cfg.Defaults.KeyBits != 0 && cfg.Defaults.KeyBits < 2048 {
		return usagef("配置校验", "defaults.keyBits 必须 >= 2048: %d", cfg.Defaults.KeyBits)
	}
	if cfg.Defaults.P12Profile != "" {
		if _, err := ParseP12Profile(cfg.Defaults.P12Profile); err != nil {
			return usagef("配置校验", "%v", err)
		}
	}
	if cfg.Defaults.Installers != "" {
		if _, err := ParseInstallers(cfg.Defaults.Installers); err != nil {
			return usagef("配置校验", "%v", err)
		}
	}
	if cfg.Reload.Mode != "" && cfg.Reload.Mode != "creds" && cfg.Reload.Mode != "restart" {
		return usagef("配置校验", "reload.mode 只能是 creds 或 restart: %q", cfg.Reload.Mode)
	}
	if cfg.Download.TTL != "" {
		ttl, err := time.ParseDuration(cfg.Download.TTL)
		if err != nil || ttl <= 0 {
			return usagef("配置校验", "download.ttl 必须是正时长: %q", cfg.Download.TTL)
		}
	}
	if cfg.Download.MaxFiles < 0 {
		return usagef("配置校验", "download.maxFiles 不能为负数: %d", cfg.Download.MaxFiles)
	}
	if cfg.Trash.Retention != "" {
		retention, err := time.ParseDuration(cfg.Trash.Retention)
		if err != nil || retention < 0 {
			return usagef("配置校验", "trash.retention 必须是非负时长: %q", cfg.Trash.Retention)
		}
	}
	if cfg.Audit.MaxBytes < 0 {
		return usagef("配置校验", "audit.maxBytes 不能为负数: %d", cfg.Audit.MaxBytes)
	}
	if cfg.Audit.Keep < 0 {
		return usagef("配置校验", "audit.keep 不能为负数: %d", cfg.Audit.Keep)
	}
	if err := webConfigFromFile(cfg.Web).Validate(); err != nil {
		return usagef("配置校验", "web: %v", err)
	}
	return nil
}

// MigrateConfigFile 检查或执行 v1/0 → v2 迁移。
//
// 默认只检查；write=true 时先写 0600 备份，再用 WriteConfigFile 原子替换。
// 含 v2 不识别字段的 v1 配置会被拒绝，避免静默丢弃扩展字段。
func MigrateConfigFile(path string, write bool) (*ConfigMigration, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, usagef("迁移配置", "未指定配置文件")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, preconditionf("迁移配置", "配置文件不存在: %s", path)
	}
	if err != nil {
		return nil, runtimef("迁移配置", "读取 %s: %v", path, err)
	}
	var header configHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, usagef("解析配置", "%s: %v", path, err)
	}
	if header.SchemaVersion == ConfigSchemaV2 {
		if _, err := decodeStrictFileConfig(data, path); err != nil {
			return nil, err
		}
		return &ConfigMigration{Path: path, From: ConfigSchemaV2, To: ConfigSchemaV2}, nil
	}
	if header.SchemaVersion < 0 || header.SchemaVersion > ConfigSchemaV2 {
		return nil, usagef("迁移配置", "%s: 不支持的 schemaVersion=%d（当前支持 1/2）", path, header.SchemaVersion)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, usagef("迁移配置", "%s: %v", path, err)
	}
	if _, err := decodeFileConfig(data, path); err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, usagef("迁移配置", "%s: %v", path, err)
	}
	if root == nil {
		return nil, usagef("迁移配置", "%s: 顶层必须是 JSON 对象", path)
	}
	root["schemaVersion"] = json.RawMessage("2")
	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, runtimef("迁移配置", "生成 %s: %v", path, err)
	}
	output = append(output, '\n')
	// 先在内存中用 v2 严格规则验证，再决定是否写盘/备份。
	if _, err := decodeStrictFileConfig(output, path); err != nil {
		return nil, err
	}
	result := &ConfigMigration{
		Path:           path,
		From:           header.SchemaVersion,
		To:             ConfigSchemaV2,
		NeedsMigration: true,
	}
	if !write {
		return result, nil
	}
	backup, err := backupConfigFile(path, data)
	if err != nil {
		return nil, err
	}
	if err := WriteConfigFile(path, output); err != nil {
		return nil, err
	}
	result.Written = true
	result.BackupPath = backup
	return result, nil
}

// backupConfigFile 写入只读备份；使用 O_EXCL 防止覆盖已有备份。
// validateConfigPath 对 v2 的路径字段要求绝对路径；空值仍由默认值补齐。
func validateConfigPath(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if !pathpkg.IsAbs(value) {
		return usagef("配置校验", "%s 必须是绝对路径: %q", name, value)
	}
	return nil
}

// rejectDuplicateJSONKeys 递归检查重复键；encoding/json 默认以后值覆盖前值。
func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("schema v2 只允许一个 JSON 对象")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, where string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("对象键不是字符串: %s", where)
			}
			if seen[key] {
				return fmt.Errorf("重复字段: %s.%s", where, key)
			}
			seen[key] = true
			if err := scanJSONValue(decoder, where+"."+key); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s[%d]", where, index)); err != nil {
				return err
			}
			index++
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("非法 JSON 分隔符: %s", where)
	}
	return nil
}

func backupConfigFile(path string, data []byte) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	base := fmt.Sprintf("%s.schema-v1-%s.bak", path, stamp)
	for attempt := 0; attempt < 100; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s.%d", base, attempt)
		}
		file, err := os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", runtimef("迁移配置", "创建备份 %s: %v", candidate, err)
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			os.Remove(candidate)
			return "", runtimef("迁移配置", "写入备份 %s: %v", candidate, err)
		}
		if err := file.Close(); err != nil {
			os.Remove(candidate)
			return "", runtimef("迁移配置", "关闭备份 %s: %v", candidate, err)
		}
		return candidate, nil
	}
	return "", runtimef("迁移配置", "无法创建唯一备份文件: %s.schema-v1-*.bak", path)
}
