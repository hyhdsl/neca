// 本文件管理删除证书的回收站。
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// TrashFile records one moved path. Source is the original absolute path;
// Relative is the path inside the entry's files directory.
type TrashFile struct {
	Source   string `json:"source"`
	Relative string `json:"relative"`
}

// TrashEntry is the manifest of one recycled certificate.
type TrashEntry struct {
	ID        string      `json:"id"`
	CN        string      `json:"cn"`
	CreatedAt time.Time   `json:"createdAt"`
	Files     []TrashFile `json:"files"`
	Rows      []ports.Row `json:"rows,omitempty"`
}

// TrashDir returns the minimal recycle-bin directory.
// M11 may make this configurable; keeping it under PKIDir avoids a new
// config field in the M9.4 namespace migration.
func TrashDir(cfg *Config) string {
	return filepath.Join(cfg.PKIDir, "trash")
}

// TrashCertificate moves a certificate and its related files into trash,
// removes matching index rows, and regenerates the CRL.
func TrashCertificate(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, engine *Engine, cn string) (*CertificateResult, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	var result *CertificateResult
	err := withPkiLock(cfg.LockFile, func() error {
		rows, err := store.Load(ctx)
		if err != nil {
			return err
		}
		serials := map[string]bool{}
		certPath := filepath.Join(cfg.PKIDir, cn+".crt")
		if _, err := os.Stat(certPath); err == nil {
			if serial, err := oss.Serial(ctx, certPath); err == nil {
				serials[serial] = true
			}
		}
		archives, _ := filepath.Glob(filepath.Join(cfg.ArchiveDir, cn+"-*.crt"))
		for _, archive := range archives {
			if serial, err := oss.Serial(ctx, archive); err == nil {
				serials[serial] = true
			}
		}
		selected := make([]ports.Row, 0)
		for _, row := range rows {
			if row.CN() == cn || serials[row.Serial] {
				selected = append(selected, row)
			}
		}
		files, err := collectTrashFiles(cfg, cn)
		if err != nil {
			return err
		}
		if len(files) == 0 && len(selected) == 0 {
			return preconditionf("移入回收站", "找不到证书: %s", cn)
		}

		entry, err := createTrashEntry(cfg, cn, files, selected)
		if err != nil {
			return err
		}
		if err := moveTrashFiles(cfg, entry, true); err != nil {
			_ = os.RemoveAll(filepath.Join(TrashDir(cfg), entry.ID))
			return err
		}
		if err := store.UpdateLocked(ctx, func(idx *ports.Index) error {
			kept := idx.Rows[:0]
			for _, row := range idx.Rows {
				remove := row.CN() == cn
				for serial := range serials {
					if strings.EqualFold(row.Serial, serial) {
						remove = true
					}
				}
				if !remove {
					kept = append(kept, row)
				}
			}
			idx.Rows = kept
			return nil
		}); err != nil {
			_ = moveTrashFiles(cfg, entry, false)
			_ = os.RemoveAll(filepath.Join(TrashDir(cfg), entry.ID))
			return err
		}
		_ = cleanupDownloadsForCN(cfg, cn)
		crl, reload, err := regenerateCRL(ctx, cfg, oss, engine, false)
		if err != nil {
			return err
		}
		result = &CertificateResult{CN: cn, CRLPath: crl, Warnings: reload.Warnings}
		return nil
	})
	return result, err
}

// ListTrash returns entries newest-first. Corrupt entries are reported as errors.
func ListTrash(cfg *Config) ([]TrashEntry, error) {
	root := TrashDir(cfg)
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, runtimef("读取回收站", "%s: %v", root, err)
	}
	entries := make([]TrashEntry, 0, len(dirs))
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		entry, err := loadTrashEntry(cfg, dir.Name())
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	return entries, nil
}

// ExpiredTrashIDs returns entries older than cfg.TrashRetention, newest-first.
// Retention 0 means unlimited; no entries are expired.
func ExpiredTrashIDs(cfg *Config, now time.Time) ([]string, error) {
	if cfg == nil || cfg.TrashRetention <= 0 {
		return nil, nil
	}
	entries, err := ListTrash(cfg)
	if err != nil {
		return nil, err
	}
	cutoff := now.UTC().Add(-cfg.TrashRetention)
	ids := make([]string, 0)
	for _, entry := range entries {
		if entry.CreatedAt.Before(cutoff) {
			ids = append(ids, entry.ID)
		}
	}
	return ids, nil
}

// PurgeExpiredTrash permanently removes entries past the configured retention.
// Callers must confirm with the operator first; secure erasure is not promised.
func PurgeExpiredTrash(cfg *Config, now time.Time) ([]string, error) {
	ids, err := ExpiredTrashIDs(cfg, now)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := PurgeTrash(cfg, id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// RestoreTrash restores files and index rows from one entry.
func RestoreTrash(ctx context.Context, cfg *Config, store *FileIndexStore, oss *OpenSSL, engine *Engine, id string) (*TrashEntry, error) {
	var restored *TrashEntry
	err := withPkiLock(cfg.LockFile, func() error {
		entry, err := loadTrashEntry(cfg, id)
		if err != nil {
			return err
		}
		if err := moveTrashFiles(cfg, entry, false); err != nil {
			return err
		}
		if err := store.UpdateLocked(ctx, func(idx *ports.Index) error {
			for _, row := range entry.Rows {
				if idx.Find(row.Serial) < 0 {
					idx.Rows = append(idx.Rows, row)
				}
			}
			return nil
		}); err != nil {
			_ = moveTrashFiles(cfg, entry, true)
			return err
		}
		_, _, err = regenerateCRL(ctx, cfg, oss, engine, false)
		if err != nil {
			return err
		}
		restored = &entry
		return os.RemoveAll(filepath.Join(TrashDir(cfg), id))
	})
	return restored, err
}

// PurgeTrash permanently removes one entry. Secure erasure is not promised.
func PurgeTrash(cfg *Config, id string) error {
	path := filepath.Join(TrashDir(cfg), filepath.Base(id))
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return preconditionf("清理回收站", "回收站条目不存在: %s", id)
	}
	if err := os.RemoveAll(path); err != nil {
		return runtimef("清理回收站", "%s: %v", path, err)
	}
	return nil
}

func collectTrashFiles(cfg *Config, cn string) ([]TrashFile, error) {
	candidates := []string{
		filepath.Join(cfg.PKIDir, cn+".crt"),
		filepath.Join(cfg.PKIDir, cn+".key"),
		filepath.Join(cfg.PKIDir, cn+".csr"),
		filepath.Join(cfg.PKIDir, cn+".ext"),
		filepath.Join(cfg.ClientsDir, cn),
		MetaPath(cfg.MetaDir, cn),
	}
	archives, err := filepath.Glob(filepath.Join(cfg.ArchiveDir, cn+"-*"))
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, archives...)
	files := make([]TrashFile, 0, len(candidates))
	for _, source := range candidates {
		if _, err := os.Lstat(source); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		files = append(files, TrashFile{Source: source})
	}
	return files, nil
}

func createTrashEntry(cfg *Config, cn string, files []TrashFile, rows []ports.Row) (TrashEntry, error) {
	root := TrashDir(cfg)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return TrashEntry{}, runtimef("创建回收站", "%s: %v", root, err)
	}
	base := time.Now().UTC().Format("20060102T150405Z") + "-" + cn
	id := base
	for suffix := 1; ; suffix++ {
		path := filepath.Join(root, id)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			break
		}
		id = fmt.Sprintf("%s-%d", base, suffix)
	}
	entry := TrashEntry{ID: id, CN: cn, CreatedAt: time.Now().UTC(), Rows: rows}
	for index := range files {
		files[index].Relative = filepath.Join("files", fmt.Sprintf("%04d", index))
	}
	entry.Files = files
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		return TrashEntry{}, err
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return TrashEntry{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "entry.json"), append(data, '\n'), 0o600); err != nil {
		return TrashEntry{}, err
	}
	return entry, nil
}

func loadTrashEntry(cfg *Config, id string) (TrashEntry, error) {
	path := filepath.Join(TrashDir(cfg), filepath.Base(id), "entry.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return TrashEntry{}, preconditionf("回收站", "条目不存在: %s", id)
	}
	if err != nil {
		return TrashEntry{}, err
	}
	var entry TrashEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return TrashEntry{}, runtimef("读取回收站", "%s: %v", path, err)
	}
	if entry.ID == "" || entry.CN == "" {
		return TrashEntry{}, runtimef("读取回收站", "%s: 清单不完整", path)
	}
	return entry, nil
}

// moveTrashFiles moves files between original paths and the entry directory.
// toTrash=true: original -> entry; false: entry -> original.
func moveTrashFiles(cfg *Config, entry TrashEntry, toTrash bool) error {
	entryDir := filepath.Join(TrashDir(cfg), filepath.Base(entry.ID))
	moved := make([]TrashFile, 0, len(entry.Files))
	rollback := func() {
		for index := len(moved) - 1; index >= 0; index-- {
			file := moved[index]
			trashPath := filepath.Join(entryDir, file.Relative)
			source, target := file.Source, trashPath
			if !toTrash {
				source, target = trashPath, file.Source
			}
			_ = os.Rename(target, source)
		}
	}
	for _, file := range entry.Files {
		trashPath := filepath.Join(entryDir, file.Relative)
		source, target := file.Source, trashPath
		if !toTrash {
			source, target = trashPath, file.Source
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Rename(source, target); err != nil {
			rollback()
			return runtimef("移动回收站", "%s -> %s: %v", source, target, err)
		}
		moved = append(moved, file)
	}
	return nil
}
