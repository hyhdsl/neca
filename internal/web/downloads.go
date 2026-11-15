package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

var (
	errDownloadNotFound = errors.New("download not found")
	errDownloadGone     = errors.New("download expired")
)

type downloadView struct {
	ID         string    `json:"id"`
	CN         string    `json:"cn"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	SHA256     string    `json:"sha256"`
	Platforms  []string  `json:"platforms"`
	P12Profile string    `json:"p12Profile"`
}

func (s *Server) downloadViews() ([]downloadView, error) {
	entries, err := core.ListDownloads(s.cfg)
	if err != nil {
		return nil, err
	}
	views := make([]downloadView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, downloadView{
			ID: downloadID(entry.Path), CN: entry.CN, Size: entry.Size,
			CreatedAt: entry.CreatedAt, ExpiresAt: entry.ExpiresAt,
			SHA256: entry.SHA256, Platforms: entry.Platforms, P12Profile: entry.P12Profile,
		})
	}
	return views, nil
}

// downloadID 是不透明 ID：只暴露路径哈希，不把绝对路径或任意文件名交给 URL。
func downloadID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8])
}

func (s *Server) resolveDownload(id string) (core.DownloadEntry, error) {
	entries, err := core.ListDownloads(s.cfg)
	if err != nil {
		return core.DownloadEntry{}, err
	}
	for _, entry := range entries {
		if downloadID(entry.Path) != id {
			continue
		}
		if !entry.ExpiresAt.After(s.now()) {
			return core.DownloadEntry{}, errDownloadGone
		}
		// sidecar 中的路径必须落在 downloads 目录内且是 .zip，防止被篡改的元数据指向任意文件。
		clean := filepath.Clean(entry.Path)
		rel, relErr := filepath.Rel(s.cfg.DownloadsDir, clean)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !strings.HasSuffix(clean, ".zip") {
			return core.DownloadEntry{}, errDownloadNotFound
		}
		if _, err := os.Stat(entry.Path); err != nil {
			return core.DownloadEntry{}, errDownloadNotFound
		}
		return entry, nil
	}
	return core.DownloadEntry{}, errDownloadNotFound
}

func (s *Server) serveDownload(w http.ResponseWriter, r *http.Request, entry core.DownloadEntry) {
	file, err := os.Open(entry.Path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "下载文件不存在")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		writeJSONError(w, http.StatusNotFound, "not_found", "下载文件不存在")
		return
	}
	name := filepath.Base(entry.Path)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	s.audit.log("download", clientIP(r), "", "id="+downloadID(entry.Path)+" cn="+entry.CN)
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (s *Server) handleAPIDownload(w http.ResponseWriter, r *http.Request) {
	entry, err := s.resolveDownload(r.PathValue("id"))
	switch {
	case errors.Is(err, errDownloadGone):
		writeJSONError(w, http.StatusGone, "gone", "下载已过期")
		return
	case errors.Is(err, errDownloadNotFound):
		writeJSONError(w, http.StatusNotFound, "not_found", "下载不存在")
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.serveDownload(w, r, entry)
}
