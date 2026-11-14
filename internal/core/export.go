package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// exportIssuedHeader 是已签发导出的表头（与旧脚本逐字一致）。
var exportIssuedHeader = []string{"CN", "序列号", "状态", "生效", "到期", "剩余天数", "SAN", "平台", "SHA256指纹", "P12路径"}

// exportRevokedHeader 是已吊销导出的表头；注意最后一列旧脚本命名为"说明"，
// 但存在证书文件时实际写入的是 p12 路径（需逐字节兼容）。
var exportRevokedHeader = []string{"CN", "序列号", "状态", "生效", "到期", "剩余天数", "SAN", "平台", "SHA256指纹", "说明"}

// ExportCerts 把 index 中 V/R 状态的证书导出为 CSV。
//
// outPath 为空时按旧脚本命名规则生成 EXPORT_DIR/certs-<kind>-YYYYmmdd-HHMMSS.csv。
func ExportCerts(ctx context.Context, cfg *Config, store ports.IndexStore, oss *OpenSSL, kind ExportKind, outPath string) (*ExportResult, error) {
	if kind != ExportIssued && kind != ExportRevoked {
		return nil, usagef("导出", "未知导出类型: %s", kind)
	}

	rows, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	want := ports.IndexValid
	header := exportIssuedHeader
	if kind == ExportRevoked {
		want = ports.IndexRevoked
		header = exportRevokedHeader
	}
	serials := uniqueSerials(rows, want)

	serialPaths, err := scanSerialPaths(ctx, oss, cfg.ArchiveDir, cfg.PKIDir)
	if err != nil {
		return nil, err
	}

	if outPath == "" {
		outPath = filepath.Join(cfg.ExportDir, fmt.Sprintf("certs-%s-%s.csv", kind, time.Now().Format("20060102-150405")))
	}

	csvRows := make([][]string, 0, len(serials))
	for _, serial := range serials {
		path, found := serialPaths[serial]
		if !found {
			if kind == ExportIssued {
				continue
			}
			// 已吊销但证书文件已删除：旧脚本仍输出一行"说明=证书文件已删除"。
			csvRows = append(csvRows, []string{"(已删除)", serial, "已吊销", "", "", "", "", "", "", "证书文件已删除"})
			continue
		}
		row, err := exportRow(ctx, cfg, oss, path, kind)
		if err != nil {
			return nil, err
		}
		csvRows = append(csvRows, row)
	}

	if err := WriteCSVFile(outPath, header, csvRows); err != nil {
		return nil, err
	}
	return &ExportResult{Kind: kind, Path: outPath, Count: len(csvRows)}, nil
}

// exportRow 读取单张证书并生成一行导出数据。
func exportRow(ctx context.Context, cfg *Config, oss *OpenSSL, certPath string, kind ExportKind) ([]string, error) {
	cn, err := oss.CN(ctx, certPath)
	if err != nil {
		return nil, err
	}
	serial, err := oss.Serial(ctx, certPath)
	if err != nil {
		return nil, err
	}
	start, _, err := oss.StartDate(ctx, certPath)
	if err != nil {
		return nil, err
	}
	end, _, err := oss.EndDate(ctx, certPath)
	if err != nil {
		return nil, err
	}
	san, err := oss.SAN(ctx, certPath)
	if err != nil {
		return nil, err
	}
	fingerprint, err := oss.FingerprintSHA256(ctx, certPath)
	if err != nil {
		return nil, err
	}

	platforms := exportPlatforms(cfg.MetaDir, cn)
	p12Path := filepath.Join(cfg.ClientsDir, cn, cn+".p12")
	if _, err := os.Stat(p12Path); err != nil {
		p12Path = "(无)"
	}

	// 旧脚本用 date -d +%F（本地时区）与 $(( (e - now) / 86400 ))，保持同样语义。
	now := time.Now()
	return []string{
		cn,
		serial,
		csvStatus(kind),
		start.Local().Format("2006-01-02"),
		end.Local().Format("2006-01-02"),
		strconv.FormatInt(daysUntil(end, now), 10),
		san,
		platforms,
		fingerprint,
		p12Path,
	}, nil
}

// exportPlatforms 返回导出用的平台标注：meta 缺失时为 "unknown"，
// 与旧脚本 meta_get ... || echo unknown 一致。
func exportPlatforms(metaDir, cn string) string {
	meta, err := LoadMeta(MetaPath(metaDir, cn))
	if err != nil || meta == nil {
		return "unknown"
	}
	return meta.Get("PLATFORMS")
}

// uniqueSerials 返回指定状态的全部序列号（去重、升序）。
func uniqueSerials(rows []ports.Row, want ports.IndexStatus) []string {
	seen := make(map[string]struct{})
	var serials []string
	for _, row := range rows {
		if row.Status != want {
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
