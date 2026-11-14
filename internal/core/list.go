package core

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// ListCerts 读取 index.txt 与证书文件，生成 list 命令的结构化结果。
//
// 复刻旧脚本 list_certs()：
//   - 遍历 PKI_DIR/*.crt（跳过 ca.crt/server.crt），每张证书一行；
//   - 状态取 index.txt 中同序列号的第一个匹配行，缺失显示"未索引"；
//   - 平台取 meta 的 PLATFORMS，缺失显示 "-"；
//   - 索引里已吊销、但证书文件（含归档）已不存在的记录补一行"文件已删除"；
//   - 最后统计 CRL 吊销条目数。
func ListCerts(ctx context.Context, cfg *Config, store ports.IndexStore, oss *OpenSSL) (*ListResult, error) {
	rows, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]ports.Row, len(rows))
	for _, row := range rows {
		if _, exists := index[row.Serial]; !exists {
			index[row.Serial] = row
		}
	}

	certs, err := listCertFiles(cfg.PKIDir)
	if err != nil {
		return nil, err
	}
	result := &ListResult{}
	for _, path := range certs {
		base := filepath.Base(path)
		if !isManagedCert(base) {
			continue
		}

		cn, err := oss.CN(ctx, path)
		if err != nil {
			return nil, err
		}
		serial, err := oss.Serial(ctx, path)
		if err != nil {
			return nil, err
		}

		status := "未索引"
		if row, ok := index[serial]; ok {
			if row.Status == ports.IndexValid {
				status = "有效"
			} else {
				// 旧脚本对 E（过期）也归入"已吊销"分支，保持一致。
				status = "已吊销"
			}
		}

		_, rawEnd, err := oss.EndDate(ctx, path)
		if err != nil {
			return nil, err
		}

		result.Certs = append(result.Certs, CertInfo{
			CN:        cn,
			Status:    status,
			Serial:    serial,
			Platforms: listPlatforms(cfg.MetaDir, cn),
			Expires:   strings.TrimSuffix(rawEnd, " GMT"),
		})
	}

	// 索引中已吊销、但任何证书文件都不存在的记录。
	serialPaths, err := scanSerialPaths(ctx, oss, cfg.ArchiveDir, cfg.PKIDir)
	if err != nil {
		return nil, err
	}
	for _, serial := range revokedSerials(rows) {
		if _, exists := serialPaths[serial]; exists {
			continue
		}
		result.Certs = append(result.Certs, CertInfo{
			CN:        "(证书已删除)",
			Status:    "已吊销",
			Serial:    serial,
			Platforms: "-",
			Expires:   "-",
			Note:      "文件已删除",
		})
	}

	crlCount, _ := oss.CRLRevokedCount(ctx, cfg.CRLFile)
	result.CRLRevoked = crlCount
	return result, nil
}

// listPlatforms 返回列表用的平台标注；缺失或空值统一显示 "-"。
func listPlatforms(metaDir, cn string) string {
	meta, err := LoadMeta(MetaPath(metaDir, cn))
	if err != nil || meta == nil {
		return "-"
	}
	if value := meta.Get("PLATFORMS"); value != "" {
		return value
	}
	return "-"
}

// revokedSerials 返回 index 中状态为 R 的序列号（去重、升序）。
func revokedSerials(rows []ports.Row) []string {
	seen := make(map[string]struct{})
	var serials []string
	for _, row := range rows {
		if row.Status != ports.IndexRevoked {
			continue
		}
		if _, exists := seen[row.Serial]; exists {
			continue
		}
		seen[row.Serial] = struct{}{}
		serials = append(serials, row.Serial)
	}
	sort.Strings(serials)
	return serials
}
