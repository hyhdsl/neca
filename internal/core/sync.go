// 本文件同步索引、meta 与客户端证书文件。
package core

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// metaJob 记录一次待写的元数据，等索引落盘成功后再写，避免半套状态。
type metaJob struct {
	cn      string
	expires string
	serial  string
}

// SyncIndex 复刻旧脚本 sync_index()：为 PKI_DIR 下未入索引的证书补索引，
// 为缺失元数据的 CN 补 meta。
//
// 与旧脚本的差异只在写入时机：先在全局锁内一次性追加索引，再写 meta；
// 成功/失败语义与输出文案保持一致。
func SyncIndex(ctx context.Context, cfg *Config, store ports.IndexStore, oss *OpenSSL) (*SyncResult, error) {
	rows, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		existing[row.Serial] = struct{}{}
	}

	certs, err := listCertFiles(cfg.PKIDir)
	if err != nil {
		return nil, err
	}
	result := &SyncResult{}
	var adds []ports.Row
	var metaJobs []metaJob
	for _, path := range certs {
		base := filepath.Base(path)
		if !isManagedCert(base) {
			continue
		}
		serial, err := oss.Serial(ctx, path)
		if err != nil {
			// 旧脚本读不到序列号就跳过该证书。
			continue
		}
		end, rawEnd, err := oss.EndDate(ctx, path)
		if err != nil {
			continue
		}

		if _, ok := existing[serial]; !ok {
			subject, err := oss.Subject(ctx, path)
			if err != nil {
				continue
			}
			adds = append(adds, ports.Row{
				Status:    ports.IndexValid,
				Expires:   IndexExpiry(end),
				RevokedAt: "",
				Serial:    serial,
				File:      "unknown",
				Subject:   subject,
			})
			existing[serial] = struct{}{}
			result.AddedIndex = append(result.AddedIndex, base)
		}

		cn, err := oss.CN(ctx, path)
		if err != nil {
			continue
		}
		metaPath := MetaPath(cfg.MetaDir, cn)
		if _, err := os.Stat(metaPath); errors.Is(err, fs.ErrNotExist) {
			metaJobs = append(metaJobs, metaJob{cn: cn, expires: rawEnd, serial: serial})
		}
	}

	if len(adds) > 0 {
		if err := store.Update(ctx, func(idx *ports.Index) error {
			idx.Rows = append(idx.Rows, adds...)
			return nil
		}); err != nil {
			return nil, err
		}
	}

	for _, job := range metaJobs {
		metaPath := MetaPath(cfg.MetaDir, job.cn)
		if err := WriteLegacyMeta(metaPath, job.cn, "unknown", time.Now().Format("2006-01-02"), job.expires, job.serial); err != nil {
			return nil, err
		}
		result.AddedMeta = append(result.AddedMeta, job.cn)
	}
	return result, nil
}
