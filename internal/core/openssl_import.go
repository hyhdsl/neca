// 本文件实现证书导入所需的严格 OpenSSL 检查。
package core

import (
	"context"
	"os"
	"strings"
)

// ExtractPKCS12 从 p12 中分别提取客户端证书和未加密私钥到临时文件。
//
// 私钥只写入调用方提供的临时目录；调用方必须在完成后删除目录。
func (o *OpenSSL) ExtractPKCS12(ctx context.Context, p12Path, passFile, certPath, keyPath string) error {
	if _, err := o.run(ctx, "pkcs12", "-in", p12Path, "-clcerts", "-nokeys", "-passin", "file:"+passFile, "-out", certPath); err != nil {
		return runtimef("提取 p12 证书", "%v", err)
	}
	if _, err := o.run(ctx, "pkcs12", "-in", p12Path, "-nocerts", "-nodes", "-passin", "file:"+passFile, "-out", keyPath); err != nil {
		return runtimef("提取 p12 私钥", "%v", err)
	}
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return runtimef("读取 p12 证书", "%v", err)
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return runtimef("读取 p12 私钥", "%v", err)
	}
	if strings.Count(string(certData), "BEGIN CERTIFICATE") != 1 {
		return preconditionf("导入证书", "p12 必须恰好包含一个客户端证书")
	}
	keyCount := strings.Count(string(keyData), "BEGIN PRIVATE KEY") +
		strings.Count(string(keyData), "BEGIN RSA PRIVATE KEY") +
		strings.Count(string(keyData), "BEGIN EC PRIVATE KEY")
	if keyCount != 1 {
		return preconditionf("导入证书", "p12 必须恰好包含一个客户端私钥")
	}
	return nil
}

// CheckKeyMatch 比较证书公钥和私钥导出的公钥是否一致。
func (o *OpenSSL) CheckKeyMatch(ctx context.Context, certPath, keyPath string) error {
	certRes, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-pubkey")
	if err != nil {
		return runtimef("读取证书公钥", "%v", err)
	}
	keyRes, err := o.run(ctx, "pkey", "-in", keyPath, "-pubout")
	if err != nil {
		return runtimef("读取私钥公钥", "%v", err)
	}
	certPub := strings.Join(strings.Fields(certRes.Stdout), "")
	keyPub := strings.Join(strings.Fields(keyRes.Stdout), "")
	if certPub == "" || keyPub == "" || certPub != keyPub {
		return preconditionf("导入证书", "p12 中私钥与证书不匹配")
	}
	return nil
}

// ValidateClientCertificate 拒绝 CA 证书和缺少 clientAuth 用途的证书。
func (o *OpenSSL) ValidateClientCertificate(ctx context.Context, certPath string) error {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-text")
	if err != nil {
		return runtimef("读取证书扩展", "%v", err)
	}
	text := res.Stdout
	lower := strings.ToLower(text)
	if strings.Contains(lower, "ca:true") || !strings.Contains(lower, "ca:false") {
		return preconditionf("导入证书", "证书必须是 CA:FALSE 的客户端证书")
	}
	if !strings.Contains(lower, "tls web client authentication") {
		return preconditionf("导入证书", "证书缺少 clientAuth 扩展用途")
	}
	return nil
}

// VerifyCertificateChain 使用当前 CA 校验证书链；过期导入路径可关闭时间检查。
func (o *OpenSSL) VerifyCertificateChain(ctx context.Context, certPath, caPath string, noCheckTime bool) error {
	args := []string{"verify"}
	if noCheckTime {
		args = append(args, "-no_check_time")
	}
	args = append(args, "-CAfile", caPath, certPath)
	if _, err := o.run(ctx, args...); err != nil {
		return runtimef("校验证书链", "%v", err)
	}
	return nil
}

// CRLSerials 返回 CRL 中的吊销序列号集合（大写十六进制）。
func (o *OpenSSL) CRLSerials(ctx context.Context, crlPath string) (map[string]bool, error) {
	res, err := o.run(ctx, "crl", "-in", crlPath, "-noout", "-text")
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Serial Number:") {
			continue
		}
		serial := strings.TrimSpace(strings.TrimPrefix(line, "Serial Number:"))
		if serial != "" {
			out[strings.ToUpper(serial)] = true
		}
	}
	return out, nil
}
