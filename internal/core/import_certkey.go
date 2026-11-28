// 本文件实现 PEM 证书 + 私钥导入的输入校验与准备。
package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// prepareCertKeyImport 把 PEM cert/key 复制到 0700 临时目录，完成解密、PEM 数量与算法校验。
func prepareCertKeyImport(ctx context.Context, cfg *Config, oss *OpenSSL, req ImportRequest) (*importPrepared, error) {
	if err := checkRegularFile(req.CertPath, 1<<20); err != nil {
		return nil, err
	}
	if err := checkRegularFile(req.KeyPath, 16<<20); err != nil {
		return nil, err
	}
	if err := checkPrivateKeyFile(req.KeyPath); err != nil {
		return nil, err
	}
	if req.KeyPasswordFile != "" {
		if err := checkPasswordFile(req.KeyPasswordFile); err != nil {
			return nil, err
		}
	}
	dir, err := os.MkdirTemp(cfg.PKIDir, ".import-")
	if err != nil {
		return nil, runtimef("导入证书", "创建临时目录: %v", err)
	}
	prep := &importPrepared{dir: dir, certPath: filepath.Join(dir, "client.crt"), keyPath: filepath.Join(dir, "client.key")}
	if err := copyImportInput(req.CertPath, prep.certPath, 0o644); err != nil {
		prep.cleanup()
		return nil, err
	}
	if err := copyImportInput(req.KeyPath, prep.keyPath, 0o600); err != nil {
		prep.cleanup()
		return nil, err
	}
	certPEM, err := os.ReadFile(prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, runtimef("读取导入证书", "%v", err)
	}
	if countPEMBlocks(certPEM, "BEGIN CERTIFICATE") != 1 {
		prep.cleanup()
		return nil, preconditionf("导入证书", "证书必须恰好包含一个 CERTIFICATE")
	}
	keyPEM, err := os.ReadFile(prep.keyPath)
	if err != nil {
		prep.cleanup()
		return nil, runtimef("读取导入私钥", "%v", err)
	}
	if encryptedPEMKey(keyPEM) {
		if req.KeyPasswordFile == "" {
			prep.cleanup()
			return nil, preconditionf("导入证书", "私钥已加密，必须提供 --key-password-file")
		}
		decPath := filepath.Join(dir, "client.dec.key")
		if err := oss.DecryptPrivateKey(ctx, req.KeyPath, req.KeyPasswordFile, decPath); err != nil {
			prep.cleanup()
			return nil, err
		}
		_ = os.Remove(prep.keyPath)
		prep.keyPath = decPath
		keyPEM, err = os.ReadFile(decPath)
		if err != nil {
			prep.cleanup()
			return nil, runtimef("读取解密私钥", "%v", err)
		}
	}
	if countPEMBlocks(keyPEM, "BEGIN PRIVATE KEY")+countPEMBlocks(keyPEM, "BEGIN RSA PRIVATE KEY")+countPEMBlocks(keyPEM, "BEGIN EC PRIVATE KEY") != 1 {
		prep.cleanup()
		return nil, preconditionf("导入证书", "私钥必须恰好包含一个未加密 PRIVATE KEY")
	}
	if err := oss.ValidatePrivateKey(ctx, prep.keyPath); err != nil {
		prep.cleanup()
		return nil, err
	}
	return finalizePreparedImport(ctx, cfg, oss, req, prep)
}

// finalizePreparedImport 执行 cert/key 共用的 CN、有效期、匹配与链校验。
func finalizePreparedImport(ctx context.Context, cfg *Config, oss *OpenSSL, req ImportRequest, prep *importPrepared) (*importPrepared, error) {
	cn, err := oss.CN(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	if cn != req.CN {
		prep.cleanup()
		return nil, preconditionf("导入证书", "证书 CN 与命令 CN 不一致: %s != %s", cn, req.CN)
	}
	prep.cn = cn
	serial, err := oss.Serial(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	prep.serial = serial
	subject, err := oss.Subject(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	prep.subject = subject
	notBeforeAt, notBeforeRaw, err := oss.StartDate(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	prep.notBefore = notBeforeRaw
	expiresAt, expiresRaw, err := oss.EndDate(ctx, prep.certPath)
	if err != nil {
		prep.cleanup()
		return nil, err
	}
	prep.expires = expiresRaw
	if notBeforeAt.After(time.Now()) {
		prep.cleanup()
		return nil, preconditionf("导入证书", "证书尚未生效: %s", cn)
	}
	prep.expired = expiresAt.Before(time.Now())
	if err := oss.CheckKeyMatch(ctx, prep.certPath, prep.keyPath); err != nil {
		prep.cleanup()
		return nil, err
	}
	if err := oss.ValidateClientCertificate(ctx, prep.certPath); err != nil {
		prep.cleanup()
		return nil, err
	}
	if err := oss.VerifyCertificateChain(ctx, prep.certPath, cfg.CACert, prep.expired); err != nil {
		prep.cleanup()
		return nil, err
	}
	return prep, nil
}

func checkPrivateKeyFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return usagef("导入证书", "读取私钥文件: %s: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		return usagef("导入证书", "私钥必须是普通文件: %s", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return usagef("导入证书", "私钥文件权限必须为 0600: %s", path)
	}
	return nil
}

func copyImportInput(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return runtimef("复制导入输入", "%s: %v", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return runtimef("复制导入输入", "%s: %v", dst, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return runtimef("复制导入输入", "%s: %v", dst, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return runtimef("复制导入输入", "%s: %v", dst, closeErr)
	}
	if err := os.Chmod(dst, mode); err != nil {
		_ = os.Remove(dst)
		return runtimef("设置导入输入权限", "%s: %v", dst, err)
	}
	return nil
}

func countPEMBlocks(data []byte, marker string) int {
	return strings.Count(string(data), marker)
}

func encryptedPEMKey(data []byte) bool {
	text := string(data)
	return strings.Contains(text, "BEGIN ENCRYPTED PRIVATE KEY") ||
		strings.Contains(text, "Proc-Type: 4,ENCRYPTED")
}
