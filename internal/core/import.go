// 本文件实现 p12 客户端证书导入、校验与事务回滚。
package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// ImportSource 标识导入输入类型；v1 只实现 p12。
type ImportSource string

const (
	// ImportSourceP12 是 p12 来源。
	ImportSourceP12 ImportSource = "p12"
	// ImportSourceCertKey 是 PEM 证书 + 私钥来源。
	ImportSourceCertKey ImportSource = "cert-key"
)

// ImportRequest 是证书导入的结构化输入。
type ImportRequest struct {
	CN              string
	Source          ImportSource
	P12Path         string
	PasswordFile    string
	CertPath        string
	KeyPath         string
	KeyPasswordFile string
	Platforms       []string
	Installers      InstallerSet
	P12Profile      P12Profile
	AllowExpired    bool
	DryRun          bool
}

// ImportValidation 是导入前的只读检查结果。
type ImportValidation struct {
	CN        string
	Serial    string
	Subject   string
	NotBefore string
	Expires   string
	Expired   bool
	Source    ImportSource
	Platforms []string
	Warnings  []string
}

// ImportResult 是成功导入的结构化结果。
type ImportResult struct {
	CN          string
	Serial      string
	Subject     string
	NotBefore   string
	Expires     string
	Created     string
	Expired     bool
	Source      ImportSource
	Platforms   []string
	Messages    []string
	Warnings    []string
	NextActions []string
	IndexAdded  bool
	MetaWritten bool
	DryRun      bool
}

type importPrepared struct {
	dir         string
	certPath    string
	keyPath     string
	cn          string
	serial      string
	subject     string
	notBefore   string
	expires     string
	notBeforeAt time.Time
	expiresAt   time.Time
	expired     bool
}

func (p *importPrepared) cleanup() {
	if p != nil && p.dir != "" {
		_ = os.RemoveAll(p.dir)
	}
}

// ValidateImport 执行只读校验，返回可展示的导入计划。
func ValidateImport(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, req ImportRequest) (*ImportValidation, error) {
	if err := validateCN(req.CN); err != nil {
		return nil, err
	}
	if err := normalizeImportRequest(cfg, &req); err != nil {
		return nil, err
	}
	if err := checkPendingImportTxn(cfg); err != nil {
		return nil, err
	}
	prep, err := prepareImport(ctx, cfg, oss, req)
	if err != nil {
		return nil, err
	}
	defer prep.cleanup()
	if err := checkImportConflicts(ctx, cfg, store, oss, prep.cn, prep.serial); err != nil {
		return nil, err
	}
	return &ImportValidation{
		CN: prep.cn, Serial: prep.serial, Subject: prep.subject,
		NotBefore: prep.notBefore, Expires: prep.expires, Expired: prep.expired,
		Source: req.Source, Platforms: req.Platforms,
	}, nil
}

// ImportCertificate 在 PKI 锁内完成最终检查、导入和失败回滚。
func ImportCertificate(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, req ImportRequest) (result *ImportResult, err error) {
	if err := validateCN(req.CN); err != nil {
		return nil, err
	}
	if err := normalizeImportRequest(cfg, &req); err != nil {
		return nil, err
	}
	err = withPkiLock(cfg.LockFile, func() error {
		if recoverErr := recoverImportTransactionsLocked(ctx, cfg, store); recoverErr != nil {
			return recoverErr
		}
		prep, prepErr := prepareImport(ctx, cfg, oss, req)
		if prepErr != nil {
			return prepErr
		}
		defer prep.cleanup()
		if conflictErr := checkImportConflicts(ctx, cfg, store, oss, prep.cn, prep.serial); conflictErr != nil {
			return conflictErr
		}
		if prep.expired && !req.AllowExpired {
			return preconditionf("导入证书", "证书已过期，需 --allow-expired 并二次确认: %s", prep.cn)
		}

		created := time.Now().Format("2006-01-02")
		result = &ImportResult{
			CN: prep.cn, Serial: prep.serial, Subject: prep.subject,
			NotBefore: prep.notBefore, Expires: prep.expires, Created: created,
			Expired: prep.expired, Source: req.Source, Platforms: req.Platforms,
			NextActions: []string{}, DryRun: req.DryRun,
		}
		if prep.expired {
			result.Warnings = append(result.Warnings, "证书已过期，已按过期状态导入（index E）")
			result.NextActions = []string{
				"neca cert reissue " + prep.cn,
				"neca cert revoke " + prep.cn,
			}
		}
		if req.DryRun {
			return nil
		}

		certTarget := filepath.Join(cfg.PKIDir, prep.cn+".crt")
		keyTarget := filepath.Join(cfg.PKIDir, prep.cn+".key")
		clientsDir := filepath.Join(cfg.ClientsDir, prep.cn)
		metaPath := MetaPath(cfg.MetaDir, prep.cn)
		cleanupOK := true
		txn, txnErr := beginImportTxn(cfg, prep.cn, prep.serial)
		if txnErr != nil {
			return txnErr
		}
		defer func() {
			if cleanupOK {
				_ = txn.remove()
			}
		}()
		indexAdded := false
		success := false
		defer func() {
			if success {
				return
			}
			if indexAdded {
				if rollbackErr := store.UpdateLocked(context.Background(), func(idx *ports.Index) error {
					kept := idx.Rows[:0]
					for _, row := range idx.Rows {
						if !strings.EqualFold(row.Serial, prep.serial) {
							kept = append(kept, row)
						}
					}
					idx.Rows = kept
					return nil
				}); rollbackErr != nil {
					cleanupOK = false
				}
			}
			for _, path := range []string{certTarget, keyTarget, metaPath} {
				if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
					cleanupOK = false
				}
			}
			if removeErr := os.RemoveAll(clientsDir); removeErr != nil {
				cleanupOK = false
			}
		}()

		if err := installImportFile(prep.certPath, certTarget, 0o644); err != nil {
			return err
		}
		if err := txn.setPhase("cert"); err != nil {
			return err
		}
		if err := installImportFile(prep.keyPath, keyTarget, 0o600); err != nil {
			return err
		}
		if err := txn.setPhase("key"); err != nil {
			return err
		}

		status := ports.IndexValid
		if prep.expired {
			status = ports.IndexExpired
		}
		if err := store.UpdateLocked(ctx, func(idx *ports.Index) error {
			for _, row := range idx.Rows {
				if strings.EqualFold(row.Serial, prep.serial) {
					return preconditionf("导入证书", "序列号已存在: %s", prep.serial)
				}
				if row.CN() == prep.cn {
					return preconditionf("导入证书", "已有 CN 的索引记录: %s", prep.cn)
				}
			}
			idx.Rows = append(idx.Rows, ports.Row{
				Status: status, Expires: IndexExpiry(prep.expiresAt), Serial: prep.serial,
				File: "unknown", Subject: prep.subject,
			})
			return nil
		}); err != nil {
			return err
		}
		indexAdded = true
		result.IndexAdded = true
		if err := txn.setPhase("index"); err != nil {
			return err
		}
		if err := writeImportMeta(cfg, prep, req); err != nil {
			return err
		}
		result.MetaWritten = true
		if err := txn.setPhase("meta"); err != nil {
			return err
		}
		if err := exportP12Locked(ctx, cfg, oss, prep.cn, req.P12Profile); err != nil {
			return err
		}
		if err := txn.setPhase("clients"); err != nil {
			return err
		}
		bundles, err := RenderBundles(ctx, cfg, prep.cn, req.Platforms, req.Installers)
		if err != nil {
			return err
		}
		result.Messages = bundles.Messages
		result.Warnings = append(result.Warnings, bundles.Warnings...)
		if err := txn.commit(); err != nil {
			return err
		}
		success = true
		return nil
	})
	return result, err
}

func normalizeImportRequest(cfg *Config, req *ImportRequest) error {
	if req.Source == "" {
		switch {
		case req.P12Path != "":
			req.Source = ImportSourceP12
		case req.CertPath != "" || req.KeyPath != "":
			req.Source = ImportSourceCertKey
		default:
			return usagef("导入证书", "必须提供 --p12，或 --cert 与 --key")
		}
	}
	switch req.Source {
	case ImportSourceP12:
		if req.CertPath != "" || req.KeyPath != "" || req.KeyPasswordFile != "" {
			return usagef("导入证书", "--p12 不能与 --cert/--key/--key-password-file 同时使用")
		}
		if req.P12Path == "" {
			return usagef("导入证书", "必须提供 --p12")
		}
		if req.PasswordFile == "" {
			return usagef("导入证书", "必须提供 --password-file")
		}
	case ImportSourceCertKey:
		if req.P12Path != "" || req.PasswordFile != "" {
			return usagef("导入证书", "--cert/--key 不能与 --p12/--password-file 同时使用")
		}
		if req.CertPath == "" || req.KeyPath == "" {
			return usagef("导入证书", "--cert 与 --key 必须同时提供")
		}
	default:
		return usagef("导入证书", "不支持的导入来源: %s", req.Source)
	}
	if req.Platforms == nil {
		req.Platforms = DefaultPlatforms()
	}
	platforms, err := NormalizePlatforms(req.Platforms)
	if err != nil {
		return usagef("平台", "%v", err)
	}
	req.Platforms = platforms
	if req.P12Profile == "" {
		req.P12Profile = cfg.P12Profile
	}
	if req.P12Profile == "" {
		req.P12Profile = P12Compat
	}
	return nil
}

func prepareImport(ctx context.Context, cfg *Config, oss *OpenSSL, req ImportRequest) (*importPrepared, error) {
	if req.Source == ImportSourceCertKey {
		return prepareCertKeyImport(ctx, cfg, oss, req)
	}
	if err := checkRegularFile(req.P12Path, 16<<20); err != nil {
		return nil, err
	}
	if err := checkPasswordFile(req.PasswordFile); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(cfg.PKIDir, ".import-")
	if err != nil {
		return nil, runtimef("导入证书", "创建临时目录: %v", err)
	}
	prep := &importPrepared{dir: dir, certPath: filepath.Join(dir, "client.crt"), keyPath: filepath.Join(dir, "client.key")}
	if err := oss.ExtractPKCS12(ctx, req.P12Path, req.PasswordFile, prep.certPath, prep.keyPath); err != nil {
		prep.cleanup()
		return nil, err
	}
	cn, err := oss.CN(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	if cn != req.CN {
		prep.cleanup()
		return nil, preconditionf("导入证书", "证书 CN 与命令 CN 不一致: %s != %s", cn, req.CN)
	}
	serial, err := oss.Serial(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	subject, err := oss.Subject(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	notBeforeAt, notBeforeRaw, err := oss.StartDate(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	expiresAt, expiresRaw, err := oss.EndDate(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	if notBeforeAt.After(time.Now()) {
		prep.cleanup()
		return nil, preconditionf("导入证书", "证书尚未生效: %s", cn)
	}
	expired := expiresAt.Before(time.Now())
	if err := oss.CheckKeyMatch(ctx, prep.certPath, prep.keyPath); err != nil {
		prep.cleanup()
		return nil, err
	}
	if err := oss.ValidateClientCertificate(ctx, prep.certPath); err != nil {
		prep.cleanup()
		return nil, err
	}
	if err := oss.VerifyCertificateChain(ctx, prep.certPath, cfg.CACert, expired); err != nil {
		prep.cleanup()
		return nil, err
	}
	return &importPrepared{
		dir: dir, certPath: prep.certPath, keyPath: prep.keyPath, cn: cn, serial: serial,
		subject: subject, notBefore: notBeforeRaw, expires: expiresRaw,
		notBeforeAt: notBeforeAt, expiresAt: expiresAt, expired: expired,
	}, nil
}

func checkImportConflicts(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, cn, serial string) error {
	for _, path := range []string{
		filepath.Join(cfg.PKIDir, cn+".crt"),
		filepath.Join(cfg.PKIDir, cn+".key"),
		filepath.Join(cfg.ClientsDir, cn),
		MetaPath(cfg.MetaDir, cn),
	} {
		if _, err := os.Lstat(path); err == nil {
			return preconditionf("导入证书", "目标已存在: %s", path)
		} else if !os.IsNotExist(err) {
			return runtimef("检查导入目标", "%s: %v", path, err)
		}
	}
	rows, err := store.Load(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if strings.EqualFold(row.Serial, serial) {
			return preconditionf("导入证书", "序列号已存在: %s", serial)
		}
		if row.CN() == cn {
			return preconditionf("导入证书", "已有 CN 的索引记录: %s", cn)
		}
	}
	if _, err := os.Stat(cfg.CRLFile); err == nil {
		revoked, err := oss.CRLSerials(ctx, cfg.CRLFile)
		if err != nil {
			return err
		}
		if revoked[strings.ToUpper(serial)] {
			return preconditionf("导入证书", "证书序列号已在 CRL 中: %s", serial)
		}
	}
	return nil
}

// installImportFile 使用 O_EXCL 复制，避免检查后目标被并发创建时覆盖。
func installImportFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return runtimef("写入导入文件", "%s: %v", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return runtimef("写入导入文件", "%s: %v", dst, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return runtimef("写入导入文件", "%s: %v", dst, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return runtimef("写入导入文件", "%s: %v", dst, closeErr)
	}
	if err := os.Chmod(dst, mode); err != nil {
		_ = os.Remove(dst)
		return runtimef("设置导入文件权限", "%s: %v", dst, err)
	}
	_ = os.Remove(src)
	return nil
}

func checkRegularFile(path string, max int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return usagef("导入证书", "读取输入文件: %s: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		return usagef("导入证书", "输入必须是普通文件: %s", path)
	}
	if info.Size() <= 0 || info.Size() > max {
		return usagef("导入证书", "输入文件大小不合法: %s", path)
	}
	return nil
}

func checkPasswordFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return usagef("导入证书", "读取密码文件: %s: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		return usagef("导入证书", "密码文件必须是普通文件: %s", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return usagef("导入证书", "密码文件权限必须为 0600: %s", path)
	}
	return nil
}

func writeImportMeta(cfg *Config, prep *importPrepared, req ImportRequest) error {
	meta := NewMeta()
	meta.Set("CN", prep.cn)
	meta.Set("PLATFORMS", strings.Join(req.Platforms, ","))
	meta.Set("CREATED", time.Now().Format("2006-01-02"))
	meta.Set("NOT_BEFORE", prep.notBefore)
	meta.Set("EXPIRES", prep.expires)
	meta.Set("SERIAL", prep.serial)
	meta.Set("IMPORT_SOURCE", strings.ToUpper(string(req.Source)))
	if req.P12Profile != "" && req.P12Profile != P12Compat {
		meta.Set("P12_PROFILE", string(req.P12Profile))
	}
	return meta.WriteFile(MetaPath(cfg.MetaDir, prep.cn))
}
