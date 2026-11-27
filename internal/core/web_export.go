// 本文件提供 Web 容器可用的证书 CSV 导出。
package core

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// ExportWebCerts 从 clients/<CN>/<CN>.crt 与 index.txt 导出证书。
// Web 容器不挂载 pki 根目录，因此不能复用扫描 PKI 证书文件的 ExportCerts。
func ExportWebCerts(ctx context.Context, cfg *Config, store ports.IndexStore, oss *OpenSSL, kind ExportKind, outPath string) (*ExportResult, error) {
	if kind != ExportIssued && kind != ExportRevoked {
		return nil, usagef("导出", "未知导出类型: %s", kind)
	}
	rows, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]ports.Row, len(rows))
	for _, row := range rows {
		upper := strings.ToUpper(row.Serial)
		if _, exists := index[upper]; !exists {
			index[upper] = row
		}
	}
	entries, err := os.ReadDir(cfg.ClientsDir)
	if os.IsNotExist(err) {
		entries = nil
		err = nil
	}
	if err != nil {
		return nil, runtimef("扫描客户端目录", "%s: %v", cfg.ClientsDir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	header := exportIssuedHeader
	if kind == ExportRevoked {
		header = exportRevokedHeader
	}
	csvRows := make([][]string, 0, len(names))
	for _, name := range names {
		certPath := filepath.Join(cfg.ClientsDir, name, name+".crt")
		if _, err := os.Stat(certPath); err != nil {
			continue
		}
		cert, err := webCertFromFile(ctx, cfg, index, oss, certPath)
		if err != nil {
			return nil, err
		}
		if (kind == ExportIssued && cert.Status != "valid") || (kind == ExportRevoked && cert.Status != "revoked") {
			continue
		}
		row, err := exportRow(ctx, cfg, oss, certPath, kind)
		if err != nil {
			return nil, err
		}
		csvRows = append(csvRows, row)
	}
	if outPath == "" {
		outPath = filepath.Join(cfg.ExportDir, "web-"+string(kind)+".csv")
	}
	if err := WriteCSVFile(outPath, header, csvRows); err != nil {
		return nil, err
	}
	return &ExportResult{Kind: kind, Path: outPath, Count: len(csvRows)}, nil
}
