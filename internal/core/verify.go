// 本文件验证证书、密钥和文件完整性。
package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// VerifyClients 检查每个客户端目录里的 p12（复刻旧脚本 verify_p12）。
//
// 检查项与告警顺序：
//  1. p12 缺失 -> [WARN] p12 不存在: CN（不输出 OK 行）；
//  2. 使用 PBES2/AES -> 提示 MIUI 可能卡住；
//  3. 未见 3DES -> 告警；
//  4. 证书数不等于 1 -> 告警；
//  5. 密码文件长度小于 8 -> 告警；
//  6. 最后输出固定的 OK 行（即使有告警，与旧脚本一致）。
func VerifyClients(ctx context.Context, cfg *Config, oss *OpenSSL, only []string) (*VerifyResult, error) {
	cns, err := clientCNs(cfg.ClientsDir, only)
	if err != nil {
		return nil, err
	}
	result := &VerifyResult{}
	for _, cn := range cns {
		p12Path := filepath.Join(cfg.ClientsDir, cn, cn+".p12")
		passPath := filepath.Join(cfg.ClientsDir, cn, cn+".p12.pass")

		report := VerifyReport{CN: cn}
		if _, err := os.Stat(p12Path); err != nil {
			report.Warnings = append(report.Warnings, "p12 不存在: "+cn)
			result.Reports = append(result.Reports, report)
			continue
		}

		info, _ := oss.PKCS12Info(ctx, p12Path, passPath)
		lower := strings.ToLower(info)
		if strings.Contains(lower, "pbes2") || strings.Contains(lower, "aes256") || strings.Contains(lower, "aes128") {
			report.Warnings = append(report.Warnings, cn+": p12 使用 PBES2/AES（MIUI 可能卡在“提取证书”）")
		}
		if !strings.Contains(info, "pbeWithSHA1And3-KeyTripleDES-CBC") {
			report.Warnings = append(report.Warnings, cn+": p12 未见 3DES 加密")
		}

		count, _ := oss.PKCS12CertCount(ctx, p12Path, passPath)
		report.CertCount = count
		if count != 1 {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: p12 内含 %d 张证书（MIUI 建议仅客户端证书，CA 单独安装）", cn, count))
		}

		// 旧脚本用 wc -c 统计密码文件字节数（含换行），这里用 len(bytes) 对齐。
		if data, err := os.ReadFile(passPath); err == nil {
			report.PasswordLen = len(data)
		}
		if report.PasswordLen < 8 {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: p12 密码长度 %d 过短", cn, report.PasswordLen))
		}

		profileLabel := "未知"
		if strings.Contains(info, "pbeWithSHA1And3-KeyTripleDES-CBC") {
			profileLabel = "3DES ✓"
		} else if strings.Contains(lower, "pbes2") || strings.Contains(lower, "aes256") || strings.Contains(lower, "aes128") {
			profileLabel = "AES ✓"
		}
		report.OKMessage = fmt.Sprintf("p12 检查: %s (%s, 证书 %d 张, 密码 %d 位)", cn, profileLabel, count, report.PasswordLen)
		result.Reports = append(result.Reports, report)
	}
	return result, nil
}

// clientCNs 返回待检查的客户端 CN：指定了 only 则按给定顺序，否则扫描
// CLIENTS_DIR 下的子目录并排序（旧脚本依赖 shell glob 的排序）。
func clientCNs(clientsDir string, only []string) ([]string, error) {
	if len(only) > 0 {
		return append([]string(nil), only...), nil
	}
	entries, err := os.ReadDir(clientsDir)
	if err != nil {
		return nil, runtimef("扫描客户端目录", "%s: %v", clientsDir, err)
	}
	var cns []string
	for _, entry := range entries {
		if entry.IsDir() {
			cns = append(cns, entry.Name())
		}
	}
	sort.Strings(cns)
	return cns, nil
}
