// 本文件实现 p12 客户端证书链的 OpenSSL 校验。
package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// PKCS12Chain 提取 p12 中的客户端证书并用 CA 证书校验证书链。
//
// 只提取公钥证书（-clcerts -nokeys），不落盘私钥；临时目录仅在本次调用内
// 存在。失败时返回可读错误，由 verify 命令转成告警而不是中止整批检查。
func (o *OpenSSL) PKCS12Chain(ctx context.Context, p12Path, passFile, caFile string) (string, error) {
	if _, err := os.Stat(caFile); err != nil {
		return "", runtimef("校验证书链", "CA 证书不可读: %s: %v", caFile, err)
	}
	dir, err := os.MkdirTemp("", "neca-p12-chain-")
	if err != nil {
		return "", runtimef("校验证书链", "创建临时目录: %v", err)
	}
	defer os.RemoveAll(dir)

	certPath := filepath.Join(dir, "client.pem")
	if _, err := o.run(ctx, "pkcs12", "-in", p12Path, "-clcerts", "-nokeys", "-passin", "file:"+passFile, "-out", certPath); err != nil {
		return "", runtimef("提取 p12 客户端证书", "%v", err)
	}
	res, err := o.run(ctx, "verify", "-CAfile", caFile, certPath)
	if err != nil {
		detail := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
		detail = strings.ReplaceAll(detail, certPath, "client.pem")
		if detail == "" {
			detail = err.Error()
		}
		return detail, runtimef("校验证书链", "%s", detail)
	}
	return strings.TrimSpace(res.Stdout), nil
}
