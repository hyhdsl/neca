package core

import (
	"archive/zip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DownloadEntry 是 downloads 目录中一个 zip 的元数据。
type DownloadEntry struct {
	Path       string    `json:"path"`
	CN         string    `json:"cn"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"`
	Platforms  []string  `json:"platforms"`
	P12Profile string    `json:"p12Profile"`
}

// CreateDownloadZip 把 clients/<CN>/ 打包到下载目录，并写 TTL 元数据。
func CreateDownloadZip(cfg *Config, cn string, platforms []string) (*DownloadEntry, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(cfg.ClientsDir, cn)); err != nil {
		return nil, preconditionf("打包 zip", "客户端目录不存在: %s", filepath.Join(cfg.ClientsDir, cn))
	}
	if _, err := os.Stat(filepath.Join(cfg.ClientsDir, cn, cn+".p12")); err != nil {
		return nil, preconditionf("打包 zip", "缺少 p12: %s", cn)
	}
	if err := cleanupExpired(cfg); err != nil {
		return nil, err
	}
	entries, err := ListDownloads(cfg)
	if err != nil {
		return nil, err
	}
	if len(entries) >= cfg.DownloadMaxFiles {
		return nil, preconditionf("打包 zip", "下载目录已达上限 %d 个，请先 neca downloads clean --expired/--all", cfg.DownloadMaxFiles)
	}
	if err := os.MkdirAll(cfg.DownloadsDir, 0o700); err != nil {
		return nil, runtimef("打包 zip", "创建下载目录: %v", err)
	}

	now := time.Now()
	name := fmt.Sprintf("%s-%s-%s.zip", cn, now.Format("20060102-150405"), randomSuffix(4))
	finalPath := filepath.Join(cfg.DownloadsDir, name)
	if cfg.DownloadTTL <= 0 {
		cfg.DownloadTTL = 30 * time.Minute
	}
	expires := now.Add(cfg.DownloadTTL)

	tmp, err := os.CreateTemp(cfg.DownloadsDir, ".zip-*.tmp")
	if err != nil {
		return nil, runtimef("打包 zip", "创建临时文件: %v", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	writer := zip.NewWriter(tmp)
	if err := addManifest(writer, cfg, cn, platforms); err != nil {
		writer.Close()
		tmp.Close()
		return nil, err
	}
	if err := addTree(writer, cfg.ClientsDir, cn, cn); err != nil {
		writer.Close()
		tmp.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		tmp.Close()
		return nil, runtimef("打包 zip", "%v", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, runtimef("打包 zip", "fsync: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, runtimef("打包 zip", "close: %v", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return nil, runtimef("打包 zip", "chmod: %v", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, runtimef("打包 zip", "rename: %v", err)
	}

	entry, err := buildDownloadEntry(cfg, cn, finalPath, now, expires, platforms)
	if err != nil {
		_ = os.Remove(finalPath)
		return nil, err
	}
	if err := writeDownloadMeta(entry); err != nil {
		_ = os.Remove(finalPath)
		return nil, err
	}
	return entry, nil
}

// ListDownloads 读取下载目录中的 sidecar 元数据；缺失/损坏的 zip 视为过期并清理。
func ListDownloads(cfg *Config) ([]DownloadEntry, error) {
	metas, err := filepath.Glob(filepath.Join(cfg.DownloadsDir, "*.zip.json"))
	if err != nil {
		return nil, runtimef("读取下载目录", "%v", err)
	}
	entries := make([]DownloadEntry, 0, len(metas))
	for _, metaPath := range metas {
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var entry DownloadEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		if _, err := os.Stat(entry.Path); err != nil {
			_ = os.Remove(metaPath)
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.Before(entries[j].CreatedAt) })
	return entries, nil
}

// CleanDownloads 清理下载目录；all=false 时只删除过期项。
func CleanDownloads(cfg *Config, all bool) (int, error) {
	entries, err := ListDownloads(cfg)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	removed := 0
	for _, entry := range entries {
		if all || !entry.ExpiresAt.After(now) {
			if err := os.Remove(entry.Path); err == nil || os.IsNotExist(err) {
				removed++
			}
			_ = os.Remove(entry.Path + ".json")
		}
	}
	return removed, nil
}

// cleanupExpired 在创建新 zip 前清掉过期项。
func cleanupExpired(cfg *Config) error {
	_, err := CleanDownloads(cfg, false)
	return err
}

// cleanupDownloadsForCN 删除某 CN 的全部下载 zip（删除证书时避免残留敏感文件）。
func cleanupDownloadsForCN(cfg *Config, cn string) error {
	entries, err := ListDownloads(cfg)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.EqualFold(entry.CN, cn) {
			continue
		}
		_ = os.Remove(entry.Path)
		_ = os.Remove(entry.Path + ".json")
	}
	return nil
}

// addManifest 写 zip 根部的 MANIFEST.txt。
func addManifest(writer *zip.Writer, cfg *Config, cn string, platforms []string) error {
	meta, _ := LoadMeta(MetaPath(cfg.MetaDir, cn))
	serial := ""
	expires := ""
	if meta != nil {
		serial = meta.Get("SERIAL")
		expires = meta.Get("EXPIRES")
	}
	manifest := "CN=" + cn + "\n" +
		"SERIAL=" + serial + "\n" +
		"EXPIRES=" + expires + "\n" +
		"PLATFORMS=" + strings.Join(platforms, ",") + "\n" +
		"P12_PROFILE=" + string(cfg.P12Profile) + "\n" +
		"GENERATED=" + time.Now().UTC().Format(time.RFC3339) + "\n"
	header := &zip.FileHeader{Name: cn + "/MANIFEST.txt", Method: zip.Deflate}
	header.SetMode(0o644)
	w, err := writer.CreateHeader(header)
	if err != nil {
		return runtimef("打包 zip", "%v", err)
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		return runtimef("打包 zip", "%v", err)
	}
	return nil
}

// addTree 把 clients/<CN> 下的文件以 <CN>/... 相对路径写入 zip。
func addTree(writer *zip.Writer, root, cn, prefix string) error {
	source := filepath.Join(root, cn)
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return runtimef("打包 zip", "%v", err)
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".key") {
			return nil // 绝不把私钥放进压缩包
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || strings.Contains(rel, "..") {
			return runtimef("打包 zip", "非法路径: %s", path)
		}
		header := &zip.FileHeader{Name: prefix + "/" + filepath.ToSlash(rel), Method: zip.Deflate}
		header.SetMode(info.Mode().Perm())
		w, err := writer.CreateHeader(header)
		if err != nil {
			return runtimef("打包 zip", "%v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return runtimef("打包 zip", "%v", err)
		}
		_, err = w.Write(data)
		return err
	})
}

// buildDownloadEntry 计算 zip 的大小与 sha256。
func buildDownloadEntry(cfg *Config, cn, path string, created, expires time.Time, platforms []string) (*DownloadEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, runtimef("打包 zip", "%v", err)
	}
	sum := sha256.Sum256(data)
	return &DownloadEntry{
		Path: path, CN: cn, CreatedAt: created, ExpiresAt: expires,
		Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
		Platforms: platforms, P12Profile: string(cfg.P12Profile),
	}, nil
}

// writeDownloadMeta 写 sidecar JSON（0600）。
func writeDownloadMeta(entry *DownloadEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return runtimef("打包 zip", "%v", err)
	}
	path := entry.Path + ".json"
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return runtimef("打包 zip", "写元数据: %v", err)
	}
	return nil
}

// randomSuffix 生成随机十六进制后缀。
func randomSuffix(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
