// 本文件原子写入证书、密钥和配置文件。
package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateRSAKey 生成客户端私钥，权限 0600。
func (o *OpenSSL) GenerateRSAKey(ctx context.Context, keyPath string, bits int) error {
	if _, err := o.run(ctx, "genrsa", "-out", keyPath, fmt.Sprintf("%d", bits)); err != nil {
		return err
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return runtimef("设置私钥权限", "%s: %v", keyPath, err)
	}
	return nil
}

// GenerateCSR 生成 CSR，subject 固定 /C=CN/O=<org>/CN=<cn>。
func (o *OpenSSL) GenerateCSR(ctx context.Context, keyPath, csrPath, cn, org string) error {
	subject := fmt.Sprintf("/C=CN/O=%s/CN=%s", org, cn)
	_, err := o.run(ctx, "req", "-new", "-key", keyPath, "-subj", subject, "-out", csrPath)
	return err
}

// SignCSR 用 CA 签发客户端证书。
func (o *OpenSSL) SignCSR(ctx context.Context, csrPath, certPath, caCert, caKey, extPath string, days int) error {
	_, err := o.run(ctx, "x509", "-req", "-in", csrPath,
		"-CA", caCert, "-CAkey", caKey, "-CAcreateserial",
		"-days", fmt.Sprintf("%d", days), "-sha256",
		"-extfile", extPath, "-out", certPath)
	return err
}

// ExportPKCS12 导出 p12；算法由 profile 决定，密码从 passFile 读取。
func (o *OpenSSL) ExportPKCS12(ctx context.Context, keyPath, certPath, p12Path, passFile, name string, profile P12Profile) error {
	_, err := o.run(ctx, "pkcs12", "-export",
		"-inkey", keyPath, "-in", certPath, "-name", name,
		"-keypbe", profile.KeyPBE(),
		"-certpbe", profile.CertPBE(),
		"-macalg", profile.MACAlg(),
		"-out", p12Path,
		"-passout", "file:"+passFile,
	)
	return err
}

// GenerateCRL 调用 openssl ca -gencrl，先写临时文件再原子替换。
func (o *OpenSSL) GenerateCRL(ctx context.Context, confPath, crlPath string) error {
	dir := filepath.Dir(crlPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return runtimef("生成 CRL", "创建目录 %s: %v", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".crl-*.tmp")
	if err != nil {
		return runtimef("生成 CRL", "创建临时文件: %v", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if _, err := o.run(ctx, "ca", "-config", confPath, "-gencrl", "-out", tmpPath, "-batch"); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return runtimef("生成 CRL", "chmod: %v", err)
	}
	if err := replaceFile(tmpPath, crlPath); err != nil {
		return runtimef("生成 CRL", "替换: %v", err)
	}
	return nil
}

// RenderExt 返回带 CN 的客户端扩展文件内容（与旧脚本一致）。
func RenderExt(serverID, cn string) string {
	return "basicConstraints=critical,CA:FALSE\n" +
		"keyUsage=critical,digitalSignature,keyEncipherment\n" +
		"extendedKeyUsage=clientAuth\n" +
		"subjectAltName=DNS:" + serverID + ",DNS:" + cn + "\n" +
		"subjectKeyIdentifier=hash\n" +
		"authorityKeyIdentifier=keyid,issuer\n"
}

// CertsSummary 生成旧脚本 issue_cert 末尾的 subject/dates 输出行。
func (o *OpenSSL) CertsSummary(ctx context.Context, certPath string) ([]string, error) {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-subject", "-dates")
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(res.Stdout, "\r\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
