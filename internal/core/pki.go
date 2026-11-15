package core

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

var cnPattern = regexp.MustCompile("^[A-Za-z0-9._-]+$")

// IssueRequest 是签发/重签/平台包生成的输入。
type IssueRequest struct {
	CN         string
	Days       int
	Platforms  []string
	Installers InstallerSet
	P12Profile P12Profile
	NoReload   bool
}

// CertificateResult 是写命令的结构化结果。
type CertificateResult struct {
	CN             string
	Serial         string
	Platforms      []string
	Subject        string
	Summary        []string
	Messages       []string
	Infos          []string
	Warnings       []string
	CRLPath        string
	ArchivedSerial string
	Already        bool
}

// IssueCertificate 签发新证书。
func IssueCertificate(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, req IssueRequest) (*CertificateResult, error) {
	if err := validateCN(req.CN); err != nil {
		return nil, err
	}
	if err := normalizeIssueRequest(cfg, &req); err != nil {
		return nil, err
	}
	var result *CertificateResult
	err := withPkiLock(cfg.LockFile, func() error {
		var err error
		result, err = issueLocked(ctx, cfg, store, oss, req)
		return err
	})
	return result, err
}

// ReissueCertificate 归档旧证书并重签，然后更新 CRL 与引擎。
func ReissueCertificate(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, engine *Engine, req IssueRequest) (*CertificateResult, error) {
	if err := validateCN(req.CN); err != nil {
		return nil, err
	}
	if err := normalizeIssueRequest(cfg, &req); err != nil {
		return nil, err
	}
	var result *CertificateResult
	err := withPkiLock(cfg.LockFile, func() error {
		certPath := filepath.Join(cfg.PKIDir, req.CN+".crt")
		keyPath := filepath.Join(cfg.PKIDir, req.CN+".key")
		if _, err := os.Stat(certPath); err == nil {
			serial, err := oss.Serial(ctx, certPath)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(cfg.ArchiveDir, 0o755); err != nil {
				return runtimef("归档旧证书", "%v", err)
			}
			if err := copyFileMode(certPath, filepath.Join(cfg.ArchiveDir, req.CN+"-"+serial+".crt"), 0o644); err != nil {
				return err
			}
			if _, err := os.Stat(keyPath); err == nil {
				if err := copyFileMode(keyPath, filepath.Join(cfg.ArchiveDir, req.CN+"-"+serial+".key"), 0o600); err != nil {
					return err
				}
			}
			if err := markIndexStatus(ctx, cfg, store, oss, certPath, ports.IndexRevoked, time.Now().UTC()); err != nil {
				return err
			}
			_ = os.Remove(certPath)
			_ = os.Remove(keyPath)
			_ = os.Remove(filepath.Join(cfg.PKIDir, req.CN+".csr"))
			_ = os.Remove(filepath.Join(cfg.PKIDir, req.CN+".ext"))
			result = &CertificateResult{Infos: []string{"旧证书已归档并吊销（" + serial + "）"}, ArchivedSerial: serial}
		}
		issued, err := issueLocked(ctx, cfg, store, oss, req)
		if err != nil {
			return err
		}
		if result != nil {
			issued.Infos = append(result.Infos, issued.Infos...)
			issued.ArchivedSerial = result.ArchivedSerial
		}
		result = issued
		crl, reload, err := regenerateCRL(ctx, cfg, oss, engine, req.NoReload)
		if err != nil {
			return err
		}
		result.CRLPath = crl
		result.Warnings = append(result.Warnings, reload.Warnings...)
		return nil
	})
	return result, err
}

// RevokeCertificate 吊销证书；已吊销时返回 Already=true 且不重复操作。
func RevokeCertificate(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, engine *Engine, cn string, noReload bool) (*CertificateResult, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	var result *CertificateResult
	err := withPkiLock(cfg.LockFile, func() error {
		certPath := filepath.Join(cfg.PKIDir, cn+".crt")
		if _, err := os.Stat(certPath); err != nil {
			return preconditionf("吊销", "找不到证书: %s", certPath)
		}
		serial, err := oss.Serial(ctx, certPath)
		if err != nil {
			return err
		}
		rows, err := store.Load(ctx)
		if err != nil {
			return err
		}
		status := indexStatus(rows, serial)
		result = &CertificateResult{CN: cn, Serial: serial}
		if status == ports.IndexRevoked {
			result.Already = true
			return nil
		}
		if err := markIndexStatus(ctx, cfg, store, oss, certPath, ports.IndexRevoked, time.Now().UTC()); err != nil {
			return err
		}
		crl, reload, err := regenerateCRL(ctx, cfg, oss, engine, noReload)
		if err != nil {
			return err
		}
		result.CRLPath = crl
		result.Warnings = append(result.Warnings, reload.Warnings...)
		return nil
	})
	return result, err
}

// UnrevokeCertificate 恢复已吊销证书。
func UnrevokeCertificate(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, engine *Engine, cn string, noReload bool) (*CertificateResult, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	var result *CertificateResult
	err := withPkiLock(cfg.LockFile, func() error {
		certPath := filepath.Join(cfg.PKIDir, cn+".crt")
		if _, err := os.Stat(certPath); err != nil {
			return preconditionf("恢复吊销", "找不到证书: %s", certPath)
		}
		serial, err := oss.Serial(ctx, certPath)
		if err != nil {
			return err
		}
		rows, err := store.Load(ctx)
		if err != nil {
			return err
		}
		status := indexStatus(rows, serial)
		result = &CertificateResult{CN: cn, Serial: serial}
		if status == ports.IndexValid {
			result.Already = true
			return nil
		}
		if err := markIndexStatus(ctx, cfg, store, oss, certPath, ports.IndexValid, time.Time{}); err != nil {
			return err
		}
		crl, reload, err := regenerateCRL(ctx, cfg, oss, engine, noReload)
		if err != nil {
			return err
		}
		result.CRLPath = crl
		result.Warnings = append(result.Warnings, reload.Warnings...)
		return nil
	})
	return result, err
}

// DeleteCertificate 删除证书、客户端包、meta 与归档；默认拒绝删除已吊销证书。
func DeleteCertificate(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, engine *Engine, cn string, purgeRevoked, noReload bool) (*CertificateResult, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	var result *CertificateResult
	err := withPkiLock(cfg.LockFile, func() error {
		certPath := filepath.Join(cfg.PKIDir, cn+".crt")
		serials := map[string]bool{}
		currentSerial := ""
		if _, err := os.Stat(certPath); err == nil {
			if serial, err := oss.Serial(ctx, certPath); err == nil {
				currentSerial = serial
				serials[serial] = true
			}
		}
		archives, _ := filepath.Glob(filepath.Join(cfg.ArchiveDir, cn+"-*.crt"))
		for _, archive := range archives {
			if serial, err := oss.Serial(ctx, archive); err == nil {
				serials[serial] = true
			}
		}
		if len(serials) == 0 {
			if _, err := os.Stat(MetaPath(cfg.MetaDir, cn)); err != nil {
				return preconditionf("删除", "找不到证书: %s", certPath)
			}
		}
		rows, err := store.Load(ctx)
		if err != nil {
			return err
		}
		if !purgeRevoked {
			// 当前证书为 V 时允许删除，即使 archive 中有重签留下的 R 行；
			// 只有当前证书本身是 R（或没有当前证书、只剩 R 归档）才要求显式确认。
			if currentSerial != "" {
				if status := indexStatus(rows, currentSerial); status == ports.IndexRevoked {
					return preconditionf("删除", "证书已吊销，需 --purge-revoked 才会从 CRL 移除: %s", cn)
				}
			} else {
				for _, row := range rows {
					if serials[row.Serial] && row.Status == ports.IndexRevoked {
						return preconditionf("删除", "证书已吊销，需 --purge-revoked 才会从 CRL 移除: %s", cn)
					}
				}
			}
		}
		if err := store.UpdateLocked(ctx, func(idx *ports.Index) error {
			kept := idx.Rows[:0]
			for _, row := range idx.Rows {
				if !serials[row.Serial] {
					kept = append(kept, row)
				}
			}
			idx.Rows = kept
			return nil
		}); err != nil {
			return err
		}
		_ = os.Remove(certPath)
		_ = os.Remove(filepath.Join(cfg.PKIDir, cn+".key"))
		_ = os.Remove(filepath.Join(cfg.PKIDir, cn+".csr"))
		_ = os.Remove(filepath.Join(cfg.PKIDir, cn+".ext"))
		_ = os.RemoveAll(filepath.Join(cfg.ClientsDir, cn))
		_ = os.Remove(MetaPath(cfg.MetaDir, cn))
		for _, archive := range archives {
			_ = os.Remove(archive)
		}
		keyArchives, _ := filepath.Glob(filepath.Join(cfg.ArchiveDir, cn+"-*.key"))
		for _, archive := range keyArchives {
			_ = os.Remove(archive)
		}
		_ = cleanupDownloadsForCN(cfg, cn)
		crl, reload, err := regenerateCRL(ctx, cfg, oss, engine, noReload)
		if err != nil {
			return err
		}
		result = &CertificateResult{CN: cn, CRLPath: crl, Warnings: reload.Warnings}
		return nil
	})
	return result, err
}

// AddPlatforms 为已有证书补平台包，并合并 meta PLATFORMS。
func AddPlatforms(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, cn string, platforms []string, installers InstallerSet) (*CertificateResult, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	platforms, err := NormalizePlatforms(platforms)
	if err != nil {
		return nil, usagef("平台", "%v", err)
	}
	var result *CertificateResult
	err = withPkiLock(cfg.LockFile, func() error {
		certPath := filepath.Join(cfg.PKIDir, cn+".crt")
		if _, err := os.Stat(certPath); err != nil {
			return preconditionf("增加平台", "找不到证书: %s", certPath)
		}
		if _, err := os.Stat(filepath.Join(cfg.ClientsDir, cn, cn+".p12")); err != nil {
			if err := exportP12Locked(ctx, cfg, oss, cn, P12Compat); err != nil {
				return err
			}
		}
		bundles, err := RenderBundles(ctx, cfg, cn, platforms, installers)
		if err != nil {
			return err
		}
		current := metaPlatforms(cfg.MetaDir, cn)
		merged := mergePlatforms(current, platforms)
		if err := writeMeta(cfg, cn, merged, "", ""); err != nil {
			return err
		}
		result = &CertificateResult{CN: cn, Platforms: platforms, Messages: bundles.Messages, Warnings: bundles.Warnings}
		return nil
	})
	return result, err
}

// SetPlatforms 只更新 meta 的平台标注。
func SetPlatforms(cfg *Config, cn string, platforms []string) (*CertificateResult, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	normalized, err := NormalizePlatforms(platforms)
	if err != nil {
		return nil, usagef("平台", "%v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.PKIDir, cn+".crt")); err != nil {
		return nil, preconditionf("设置平台", "找不到证书: %s", cn)
	}
	if err := writeMeta(cfg, cn, strings.Join(normalized, ","), "", ""); err != nil {
		return nil, err
	}
	return &CertificateResult{CN: cn, Platforms: normalized}, nil
}

// RenderPackage 从现有证书重建平台包（不重签）。
func RenderPackage(ctx context.Context, cfg *Config, oss *OpenSSL, cn string, platforms []string, installers InstallerSet, profile P12Profile) (*CertificateResult, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	if len(platforms) == 0 {
		platforms = metaPlatforms(cfg.MetaDir, cn)
	}
	if len(platforms) == 0 {
		platforms = []string{PlatformWindows, PlatformAndroid}
	}
	normalized, err := NormalizePlatforms(platforms)
	if err != nil {
		return nil, usagef("平台", "%v", err)
	}
	if profile == "" {
		profile = cfg.P12Profile
	}
	var result *CertificateResult
	err = withPkiLock(cfg.LockFile, func() error {
		if _, err := os.Stat(filepath.Join(cfg.PKIDir, cn+".crt")); err != nil {
			return preconditionf("生成平台包", "找不到证书: %s", cn)
		}
		if err := exportP12Locked(ctx, cfg, oss, cn, profile); err != nil {
			return err
		}
		bundles, err := RenderBundles(ctx, cfg, cn, normalized, installers)
		if err != nil {
			return err
		}
		result = &CertificateResult{CN: cn, Platforms: normalized, Messages: bundles.Messages, Warnings: bundles.Warnings}
		return nil
	})
	return result, err
}

// issueLocked 在已持有 PKI 锁的前提下完成签发流程，失败时清理已生成产物。
func issueLocked(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, req IssueRequest) (result *CertificateResult, err error) {
	certPath := filepath.Join(cfg.PKIDir, req.CN+".crt")
	keyPath := filepath.Join(cfg.PKIDir, req.CN+".key")
	csrPath := filepath.Join(cfg.PKIDir, req.CN+".csr")
	extPath := filepath.Join(cfg.PKIDir, req.CN+".ext")
	if _, statErr := os.Stat(certPath); statErr == nil {
		return nil, preconditionf("签发", "证书已存在: %s（请用重签发或先删除）", certPath)
	}

	clientsDir := filepath.Join(cfg.ClientsDir, req.CN)
	metaPath := MetaPath(cfg.MetaDir, req.CN)
	clientsExisted := fileExists(clientsDir)
	metaExisted := fileExists(metaPath)
	created := []string{keyPath, csrPath, extPath, certPath}
	indexAdded := false
	success := false
	defer func() {
		if success {
			return
		}
		for _, path := range created {
			_ = os.Remove(path)
		}
		if indexAdded {
			_ = store.UpdateLocked(context.Background(), func(idx *ports.Index) error {
				kept := idx.Rows[:0]
				for _, row := range idx.Rows {
					if row.Serial != resultSerial(result) {
						kept = append(kept, row)
					}
				}
				idx.Rows = kept
				return nil
			})
		}
		if !clientsExisted {
			_ = os.RemoveAll(clientsDir)
		}
		if !metaExisted {
			_ = os.Remove(metaPath)
		}
	}()

	if err := oss.GenerateRSAKey(ctx, keyPath, cfg.KeyBits); err != nil {
		return nil, err
	}
	if err := oss.GenerateCSR(ctx, keyPath, csrPath, req.CN, cfg.ORG); err != nil {
		return nil, err
	}
	if err := os.WriteFile(extPath, []byte(RenderExt(cfg.ServerID, req.CN)), 0o644); err != nil {
		return nil, runtimef("写扩展文件", "%s: %v", extPath, err)
	}
	if err := oss.SignCSR(ctx, csrPath, certPath, cfg.CACert, cfg.CAKey, extPath, req.Days); err != nil {
		return nil, err
	}
	serial, err := oss.Serial(ctx, certPath)
	if err != nil {
		return nil, err
	}
	end, rawEnd, err := oss.EndDate(ctx, certPath)
	if err != nil {
		return nil, err
	}
	subject, err := oss.Subject(ctx, certPath)
	if err != nil {
		return nil, err
	}
	if err := store.UpdateLocked(ctx, func(idx *ports.Index) error {
		idx.Rows = append(idx.Rows, ports.Row{
			Status: ports.IndexValid, Expires: IndexExpiry(end), Serial: serial,
			File: "unknown", Subject: subject,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	indexAdded = true
	// 先写入 serial 供 defer 回滚使用。
	result = &CertificateResult{CN: req.CN, Serial: serial}
	if err := writeMeta(cfg, req.CN, strings.Join(req.Platforms, ","), rawEnd, serial); err != nil {
		return nil, err
	}
	if req.P12Profile != "" && req.P12Profile != P12Compat {
		if err := addMetaValue(cfg, req.CN, "P12_PROFILE", string(req.P12Profile)); err != nil {
			return nil, err
		}
	}
	if err := exportP12Locked(ctx, cfg, oss, req.CN, req.P12Profile); err != nil {
		return nil, err
	}
	p12Message, p12Warnings, err := p12Check(ctx, cfg, oss, req.CN)
	if err != nil {
		return nil, err
	}
	bundles, err := RenderBundles(ctx, cfg, req.CN, req.Platforms, req.Installers)
	if err != nil {
		return nil, err
	}
	summary, err := oss.CertsSummary(ctx, certPath)
	if err != nil {
		return nil, err
	}
	result.Subject = subject
	result.Platforms = req.Platforms
	result.Summary = summary
	if p12Message != "" {
		result.Messages = append(result.Messages, p12Message)
	}
	result.Messages = append(result.Messages, bundles.Messages...)
	result.Warnings = append(result.Warnings, p12Warnings...)
	result.Warnings = append(result.Warnings, bundles.Warnings...)
	success = true
	return result, nil
}

// p12Check 复用 verify 逻辑，返回给 issue 输出的 p12 检查行与告警。
func p12Check(ctx context.Context, cfg *Config, oss *OpenSSL, cn string) (string, []string, error) {
	res, err := VerifyClients(ctx, cfg, oss, []string{cn})
	if err != nil {
		return "", nil, err
	}
	if len(res.Reports) == 0 {
		return "", nil, nil
	}
	return res.Reports[0].OKMessage, res.Reports[0].Warnings, nil
}

// normalizeIssueRequest 补默认值并校验。
func normalizeIssueRequest(cfg *Config, req *IssueRequest) error {
	if req.Days <= 0 {
		req.Days = cfg.DefaultDays
	}
	if req.P12Profile == "" {
		req.P12Profile = cfg.P12Profile
	}
	if req.Platforms == nil {
		req.Platforms = DefaultPlatforms()
	}
	platforms, err := NormalizePlatforms(req.Platforms)
	if err != nil {
		return usagef("平台", "%v", err)
	}
	req.Platforms = platforms
	return nil
}

// exportP12Locked 导出 p12 并复制 ca.crt/证书到 clients/<CN>/。
func exportP12Locked(ctx context.Context, cfg *Config, oss *OpenSSL, cn string, profile P12Profile) error {
	if profile == "" {
		profile = P12Compat
	}
	dir := filepath.Join(cfg.ClientsDir, cn)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return runtimef("导出 p12", "创建目录 %s: %v", dir, err)
	}
	passPath := filepath.Join(dir, cn+".p12.pass")
	if _, err := EnsurePasswordFile(passPath); err != nil {
		return err
	}
	p12Path := filepath.Join(dir, cn+".p12")
	if err := oss.ExportPKCS12(ctx, filepath.Join(cfg.PKIDir, cn+".key"), filepath.Join(cfg.PKIDir, cn+".crt"), p12Path, passPath, cn, profile); err != nil {
		return err
	}
	for _, path := range []string{p12Path, passPath} {
		if err := os.Chmod(path, 0o600); err != nil {
			return runtimef("设置 p12 权限", "%s: %v", path, err)
		}
	}
	if err := copyFileMode(cfg.CACert, filepath.Join(dir, "ca.crt"), 0o644); err != nil {
		return err
	}
	if err := copyFileMode(filepath.Join(cfg.PKIDir, cn+".crt"), filepath.Join(dir, cn+".crt"), 0o644); err != nil {
		return err
	}
	return nil
}

// markIndexStatus 按证书序列号改状态；缺失时补一行。
func markIndexStatus(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, certPath string, status ports.IndexStatus, revokedAt time.Time) error {
	serial, err := oss.Serial(ctx, certPath)
	if err != nil {
		return err
	}
	end, rawEnd, err := oss.EndDate(ctx, certPath)
	if err != nil {
		return err
	}
	subject, err := oss.Subject(ctx, certPath)
	if err != nil {
		return err
	}
	revoked := ""
	if status == ports.IndexRevoked {
		revoked = IndexExpiry(revokedAt)
	}
	return store.UpdateLocked(ctx, func(idx *ports.Index) error {
		found := false
		for i := range idx.Rows {
			if idx.Rows[i].Serial != serial {
				continue
			}
			found = true
			idx.Rows[i].Status = status
			if status == ports.IndexRevoked {
				idx.Rows[i].RevokedAt = revoked
			} else {
				idx.Rows[i].RevokedAt = ""
			}
		}
		if !found {
			idx.Rows = append(idx.Rows, ports.Row{Status: status, Expires: IndexExpiry(end), RevokedAt: revoked, Serial: serial, File: "unknown", Subject: subject})
		}
		_ = rawEnd
		return nil
	})
}

// regenerateCRL 生成 CRL 并按配置重载引擎。
func regenerateCRL(ctx context.Context, cfg *Config, oss *OpenSSL, engine *Engine, noReload bool) (string, ReloadResult, error) {
	if err := oss.GenerateCRL(ctx, cfg.CAConf, cfg.CRLFile); err != nil {
		return "", ReloadResult{}, err
	}
	reload := ReloadResult{}
	if engine != nil {
		reload = engine.Reload(ctx, noReload)
	}
	return cfg.CRLFile, reload, nil
}

// writeMeta 写 meta；expires/serial 为空时保留旧值。
func writeMeta(cfg *Config, cn, platforms, expires, serial string) error {
	path := MetaPath(cfg.MetaDir, cn)
	meta, err := LoadMeta(path)
	if err != nil {
		return err
	}
	if meta == nil {
		meta = NewMeta()
	}
	if meta.Get("CN") == "" {
		meta.Set("CN", cn)
	}
	meta.Set("PLATFORMS", platforms)
	if meta.Get("CREATED") == "" {
		meta.Set("CREATED", time.Now().Format("2006-01-02"))
	}
	if expires != "" {
		meta.Set("EXPIRES", expires)
	}
	if serial != "" {
		meta.Set("SERIAL", serial)
	}
	return meta.WriteFile(path)
}

// addMetaValue 给 meta 补一个可选键。
func addMetaValue(cfg *Config, cn, key, value string) error {
	path := MetaPath(cfg.MetaDir, cn)
	meta, err := LoadMeta(path)
	if err != nil {
		return err
	}
	if meta == nil {
		meta = NewMeta()
	}
	meta.Set(key, value)
	return meta.WriteFile(path)
}

// metaPlatforms 读取 meta PLATFORMS 并拆分。
func metaPlatforms(metaDir, cn string) []string {
	meta, err := LoadMeta(MetaPath(metaDir, cn))
	if err != nil || meta == nil {
		return nil
	}
	raw := meta.Get("PLATFORMS")
	if raw == "" {
		return nil
	}
	var platforms []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			platforms = append(platforms, part)
		}
	}
	return platforms
}

// mergePlatforms 合并两个逗号列表并去重。
func mergePlatforms(current, added []string) string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{current, added} {
		for _, platform := range list {
			platform = strings.TrimSpace(platform)
			if platform != "" && !seen[platform] {
				seen[platform] = true
				out = append(out, platform)
			}
		}
	}
	return strings.Join(out, ",")
}

func indexStatus(rows []ports.Row, serial string) ports.IndexStatus {
	for _, row := range rows {
		if strings.EqualFold(row.Serial, serial) {
			return row.Status
		}
	}
	return ""
}

func validateCN(cn string) error {
	if cn == "" {
		return usagef("CN", "CN 不能为空")
	}
	if !cnPattern.MatchString(cn) {
		return usagef("CN", "CN 只允许字母数字 . _ -: %q", cn)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resultSerial 安全读取结果中的序列号（defer 清理用）。
func resultSerial(result *CertificateResult) string {
	if result == nil {
		return ""
	}
	return result.Serial
}
