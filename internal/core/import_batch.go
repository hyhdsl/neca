// 本文件实现目录优先的证书批量导入：发现、可选 sidecar 与逐项执行。
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	batchFileP12        = "client.p12"
	batchFileCert       = "client.crt"
	batchFileKey        = "client.key"
	batchFileSidecar    = "entry.json"
	batchSidecarMax     = 64 << 10
	batchMaxEntries     = 200
	batchDefaultP12Pass = ".pass"
	batchDefaultKeyPass = ".key.pass"
	batchLegacyP12Pass  = ".p12.pass"
)

// BatchDefaults 是批次级默认值；单个 CN 的 entry.json 可覆盖非敏感项。
type BatchDefaults struct {
	Platforms    []string
	Installers   InstallerSet
	P12Profile   P12Profile
	AllowExpired bool
}

// BatchOptions 是一次批量导入的输入。
type BatchOptions struct {
	Dir         string
	PasswordDir string
	Defaults    BatchDefaults
	DryRun      bool
}

// BatchEntryResult 是单个 CN 的导入结果；批量允许部分成功。
type BatchEntryResult struct {
	CN       string       `json:"cn"`
	Source   ImportSource `json:"source"`
	Status   string       `json:"status"`
	Serial   string       `json:"serial,omitempty"`
	Expires  string       `json:"expires,omitempty"`
	Expired  bool         `json:"expired,omitempty"`
	Error    string       `json:"error,omitempty"`
	Warnings []string     `json:"warnings,omitempty"`
}

// BatchImportResult 是整批结果；不承诺跨 CN 全量回滚。
type BatchImportResult struct {
	Entries   []BatchEntryResult `json:"entries"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	DryRun    bool               `json:"dryRun"`
}

type batchCandidate struct {
	req ImportRequest
	err string
}

type batchSidecar struct {
	Platforms    []string        `json:"platforms,omitempty"`
	Installers   json.RawMessage `json:"installers,omitempty"`
	P12Profile   string          `json:"p12Profile,omitempty"`
	AllowExpired *bool           `json:"allowExpired,omitempty"`
}

// ImportBatch 按目录逐项执行导入；单条失败不回滚其他成功项。
func ImportBatch(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, opts BatchOptions) (*BatchImportResult, error) {
	candidates, err := discoverBatch(opts)
	if err != nil {
		return nil, err
	}
	result := &BatchImportResult{Entries: make([]BatchEntryResult, 0, len(candidates)), DryRun: opts.DryRun}
	for _, candidate := range candidates {
		entry := BatchEntryResult{CN: candidate.req.CN, Source: candidate.req.Source}
		if candidate.err != "" {
			entry.Status = "failed"
			entry.Error = candidate.err
			result.Failed++
			result.Entries = append(result.Entries, entry)
			continue
		}
		req := candidate.req
		req.DryRun = opts.DryRun
		if opts.DryRun {
			validation, err := ValidateImport(ctx, cfg, store, oss, req)
			if err != nil {
				entry.Status = "failed"
				entry.Error = err.Error()
				result.Failed++
				result.Entries = append(result.Entries, entry)
				continue
			}
			if validation.Expired && !req.AllowExpired {
				entry.Status = "failed"
				entry.Error = "证书已过期，需 allowExpired/--allow-expired"
				entry.Serial = validation.Serial
				entry.Expires = validation.Expires
				entry.Expired = true
				result.Failed++
				result.Entries = append(result.Entries, entry)
				continue
			}
			entry.Status = "validated"
			entry.Serial = validation.Serial
			entry.Expires = validation.Expires
			entry.Expired = validation.Expired
			result.Succeeded++
			result.Entries = append(result.Entries, entry)
			continue
		}
		imported, err := ImportCertificate(ctx, cfg, store, oss, req)
		if err != nil {
			entry.Status = "failed"
			entry.Error = err.Error()
			result.Failed++
			result.Entries = append(result.Entries, entry)
			continue
		}
		entry.Status = "imported"
		entry.Serial = imported.Serial
		entry.Expires = imported.Expires
		entry.Expired = imported.Expired
		entry.Warnings = imported.Warnings
		result.Succeeded++
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func discoverBatch(opts BatchOptions) ([]batchCandidate, error) {
	rootInfo, err := os.Lstat(opts.Dir)
	if err != nil {
		return nil, usagef("批量导入", "读取批次目录: %s: %v", opts.Dir, err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, usagef("批量导入", "批次路径必须是普通目录: %s", opts.Dir)
	}
	if opts.PasswordDir != "" {
		info, err := os.Lstat(opts.PasswordDir)
		if err != nil {
			return nil, usagef("批量导入", "读取密码目录: %s: %v", opts.PasswordDir, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, usagef("批量导入", "密码路径必须是普通目录: %s", opts.PasswordDir)
		}
	}
	entries, err := os.ReadDir(opts.Dir)
	if err != nil {
		return nil, runtimef("批量导入", "读取批次目录: %v", err)
	}
	var candidates []batchCandidate
	seen := map[string]bool{}
	for _, dirEntry := range entries {
		name := dirEntry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := os.Lstat(filepath.Join(opts.Dir, name))
		if err != nil {
			return nil, runtimef("批量导入", "检查条目 %s: %v", name, err)
		}
		if !info.IsDir() {
			continue
		}
		if len(candidates) >= batchMaxEntries {
			return nil, usagef("批量导入", "单批最多 %d 个 CN", batchMaxEntries)
		}
		cn := strings.TrimSpace(name)
		if err := validateCN(cn); err != nil {
			candidates = append(candidates, batchCandidate{req: ImportRequest{CN: cn}, err: err.Error()})
			continue
		}
		key := strings.ToLower(cn)
		if seen[key] {
			candidates = append(candidates, batchCandidate{req: ImportRequest{CN: cn}, err: "批次内存在重复 CN（大小写不敏感）"})
			continue
		}
		seen[key] = true
		candidate, err := discoverBatchEntry(opts, cn)
		if err != nil {
			candidate = batchCandidate{req: ImportRequest{CN: cn}, err: err.Error()}
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return nil, usagef("批量导入", "批次目录中没有 CN 子目录: %s", opts.Dir)
	}
	return candidates, nil
}

func discoverBatchEntry(opts BatchOptions, cn string) (batchCandidate, error) {
	dir := filepath.Join(opts.Dir, cn)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return batchCandidate{}, runtimef("批量导入", "读取条目 %s: %v", cn, err)
	}
	found := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil {
			return batchCandidate{}, runtimef("批量导入", "检查条目文件 %s: %v", name, err)
		}
		if info.IsDir() {
			return batchCandidate{}, preconditionf("批量导入", "%s 下不允许子目录: %s", cn, name)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return batchCandidate{}, preconditionf("批量导入", "%s 下不允许符号链接: %s", cn, name)
		}
		switch name {
		case batchFileP12, batchFileCert, batchFileKey, batchFileSidecar:
			found[name] = path
		default:
			return batchCandidate{}, preconditionf("批量导入", "%s 下存在未识别文件: %s", cn, name)
		}
	}
	sidecar, err := loadBatchSidecar(found[batchFileSidecar])
	if err != nil {
		return batchCandidate{}, err
	}
	platforms := opts.Defaults.Platforms
	if len(sidecar.Platforms) > 0 {
		normalized, err := NormalizePlatforms(sidecar.Platforms)
		if err != nil {
			return batchCandidate{}, usagef("批量导入", "%s 的 platforms: %v", cn, err)
		}
		platforms = normalized
	}
	installers := opts.Defaults.Installers
	if len(sidecar.Installers) > 0 {
		parsed, err := parseBatchInstallers(sidecar.Installers)
		if err != nil {
			return batchCandidate{}, usagef("批量导入", "%s 的 installers: %v", cn, err)
		}
		installers = parsed
	}
	profile := opts.Defaults.P12Profile
	if sidecar.P12Profile != "" {
		parsed, err := ParseP12Profile(sidecar.P12Profile)
		if err != nil {
			return batchCandidate{}, usagef("批量导入", "%s 的 p12Profile: %v", cn, err)
		}
		profile = parsed
	}
	allowExpired := opts.Defaults.AllowExpired
	if sidecar.AllowExpired != nil {
		allowExpired = *sidecar.AllowExpired
	}
	p12 := found[batchFileP12]
	cert := found[batchFileCert]
	key := found[batchFileKey]
	if p12 != "" && (cert != "" || key != "") {
		return batchCandidate{}, preconditionf("批量导入", "%s 不能同时包含 p12 与 cert/key", cn)
	}
	req := ImportRequest{CN: cn, Platforms: platforms, Installers: installers, P12Profile: profile, AllowExpired: allowExpired}
	switch {
	case p12 != "":
		pass := batchPasswordPath(opts.PasswordDir, cn, batchDefaultP12Pass, batchLegacyP12Pass)
		if pass == "" {
			return batchCandidate{}, preconditionf("批量导入", "%s 缺少 p12 密码文件: %s%s 或 %s%s", cn, filepath.Join(opts.PasswordDir, cn), batchDefaultP12Pass, filepath.Join(opts.PasswordDir, cn), batchLegacyP12Pass)
		}
		req.Source = ImportSourceP12
		req.P12Path = p12
		req.PasswordFile = pass
	case cert != "" && key != "":
		req.Source = ImportSourceCertKey
		req.CertPath = cert
		req.KeyPath = key
		req.KeyPasswordFile = batchPasswordPath(opts.PasswordDir, cn, batchDefaultKeyPass)
	case cert != "" || key != "":
		return batchCandidate{}, preconditionf("批量导入", "%s 必须同时提供 %s 与 %s", cn, batchFileCert, batchFileKey)
	default:
		return batchCandidate{}, preconditionf("批量导入", "%s 必须包含 %s，或 %s + %s", cn, batchFileP12, batchFileCert, batchFileKey)
	}
	return batchCandidate{req: req}, nil
}

func batchPasswordPath(dir, cn string, suffixes ...string) string {
	if dir == "" {
		return ""
	}
	for _, suffix := range suffixes {
		path := filepath.Join(dir, cn+suffix)
		if _, err := os.Lstat(path); err == nil {
			return path
		}
	}
	return ""
}

func loadBatchSidecar(path string) (batchSidecar, error) {
	if path == "" {
		return batchSidecar{}, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return batchSidecar{}, runtimef("批量导入", "读取 %s: %v", batchFileSidecar, err)
	}
	if !info.Mode().IsRegular() || info.Size() > batchSidecarMax {
		return batchSidecar{}, preconditionf("批量导入", "%s 必须是不超过 %d 字节的普通文件", batchFileSidecar, batchSidecarMax)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return batchSidecar{}, runtimef("批量导入", "读取 %s: %v", batchFileSidecar, err)
	}
	var sidecar batchSidecar
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sidecar); err != nil {
		return batchSidecar{}, preconditionf("批量导入", "%s 解析失败: %v", batchFileSidecar, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return batchSidecar{}, preconditionf("批量导入", "%s 只能包含一个 JSON 对象", batchFileSidecar)
	}
	return sidecar, nil
}

func parseBatchInstallers(raw json.RawMessage) (InstallerSet, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return ParseInstallers(text)
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("必须是字符串或字符串数组")
	}
	return ParseInstallers(strings.Join(list, ","))
}
