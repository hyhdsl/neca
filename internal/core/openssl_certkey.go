// 本文件实现 cert+key 导入所需的私钥解密与算法校验。
package core

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// DecryptPrivateKey 使用 0600 口令文件把加密私钥解密到 0600 临时文件。
func (o *OpenSSL) DecryptPrivateKey(ctx context.Context, keyPath, passFile, outPath string) error {
	if _, err := o.run(ctx, "pkey", "-in", keyPath, "-passin", "file:"+passFile, "-out", outPath); err != nil {
		return runtimef("解密导入私钥", "%v", err)
	}
	if err := os.Chmod(outPath, 0o600); err != nil {
		_ = os.Remove(outPath)
		return runtimef("设置解密私钥权限", "%v", err)
	}
	return nil
}

// ValidatePrivateKey 拒绝不受支持的算法与过小的 RSA 密钥。
func (o *OpenSSL) ValidatePrivateKey(ctx context.Context, keyPath string) error {
	res, err := o.run(ctx, "pkey", "-in", keyPath, "-noout", "-text")
	if err != nil {
		return runtimef("读取导入私钥", "%v", err)
	}
	text := strings.ToLower(res.Stdout)
	if strings.Contains(text, "ed25519") || strings.Contains(text, "ed448") || strings.Contains(text, "dsa") {
		return preconditionf("导入证书", "不支持的私钥算法；仅支持 RSA 或 ECDSA P-256/P-384")
	}
	if strings.Contains(text, "asn1 oid:") {
		if !strings.Contains(text, "prime256v1") && !strings.Contains(text, "secp384r1") {
			return preconditionf("导入证书", "不支持的 ECDSA 曲线；仅支持 P-256/P-384")
		}
		return nil
	}
	bits := 0
	for _, line := range strings.Split(res.Stdout, "\n") {
		idx := strings.Index(line, "Private-Key: (")
		if idx < 0 {
			continue
		}
		rest := line[idx+len("Private-Key: ("):]
		end := strings.Index(rest, " bit")
		if end <= 0 {
			continue
		}
		if n, parseErr := strconv.Atoi(strings.TrimSpace(rest[:end])); parseErr == nil {
			bits = n
			break
		}
	}
	if bits == 0 {
		return preconditionf("导入证书", "无法识别私钥算法；仅支持 RSA 或 ECDSA P-256/P-384")
	}
	if bits < 2048 {
		return preconditionf("导入证书", "RSA 私钥至少需要 2048 bit: %d", bits)
	}
	return nil
}
