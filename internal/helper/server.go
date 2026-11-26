// 本文件实现 helper Unix socket 服务端和白名单。
package helper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/version"
)

type cacheEntry struct {
	fingerprint string
	response    Response
	expires     time.Time
}

// persistedCacheEntry 是缓存落盘格式；字段必须导出，encoding/json 才会写入。
type persistedCacheEntry struct {
	Fingerprint string    `json:"fingerprint"`
	Response    Response  `json:"response"`
	Expires     time.Time `json:"expires"`
}

// Server 是 root helper：唯一写边界，只接受白名单结构化请求。
type Server struct {
	cfg       *core.Config
	cfgPath   string
	cacheFile string
	store     *core.FileIndexStore
	oss       *core.OpenSSL
	engine    *core.Engine

	mu    sync.Mutex
	cache map[string]cacheEntry
	now   func() time.Time
}

// NewServer 构造 root helper；调用方必须保证以 root 运行。
func NewServer(cfg *core.Config, cfgPath string) *Server {
	runner := core.NewExecRunner()
	server := &Server{
		cfg: cfg, cfgPath: cfgPath, cacheFile: strings.TrimSpace(os.Getenv("NECA_WEB_HELPER_CACHE_FILE")),
		store: core.NewFileIndexStore(cfg.IndexFile, cfg.LockFile),
		oss:   core.NewOpenSSL(runner), engine: core.NewEngine(cfg, runner),
		cache: map[string]cacheEntry{}, now: time.Now,
	}
	if server.cacheFile != "" {
		server.loadCache()
	}
	return server
}

// Handler 返回 helper 的 HTTP 处理器。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleStatus)
	mux.HandleFunc("POST /v1/execute", s.handleExecute)
	mux.HandleFunc("POST /v1/settings", s.handleSettings)
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.reloadConfig()
	if err != nil {
		writeHelperJSON(w, http.StatusInternalServerError, Response{
			OK: false, Protocol: ProtocolVersion,
			Error: &Error{Code: "config", Message: err.Error()},
		})
		return
	}
	writeHelperJSON(w, http.StatusOK, Status{
		OK: true, Protocol: ProtocolVersion, Version: version.Get().Version,
		ReadOnly: cfg.Web.ReadOnly, Socket: os.Getenv("NECA_WEB_HELPER_SOCKET"),
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var req SettingsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeHelperError(w, "bad_request", "请求 JSON 非法: "+err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeHelperError(w, "bad_request", err.Error())
		return
	}
	if err := s.checkWritable(); err != nil {
		writeHelperError(w, "read_only", err.Error())
		return
	}
	cfg, err := s.reloadConfig()
	if err != nil {
		writeHelperError(w, "config", err.Error())
		return
	}
	if !cfg.Web.AllowHighRiskSettings {
		writeHelperError(w, "forbidden", "后台未允许 Web 修改高风险设置")
		return
	}
	path := s.cfgPath
	if strings.TrimSpace(path) == "" {
		path = cfg.ConfigFile
	}
	updated, err := core.UpdateWebSettings(path, core.WebSettingsUpdate{
		ReadOnly: req.ReadOnly, IdleTimeout: req.IdleTimeout, ExportEnabled: req.ExportEnabled,
	})
	if err != nil {
		writeHelperError(w, "settings", err.Error())
		return
	}
	_ = core.AppendAuditWithOrigin(cfg, "helper", "web-settings", "readOnly/idleTimeout/export", "", "ok")
	writeHelperJSON(w, http.StatusOK, SettingsResponse{
		OK: true, Protocol: ProtocolVersion, ReadOnly: updated.ReadOnly,
		IdleTimeout: updated.IdleTimeoutRaw, ExportEnabled: updated.ExportEnabled,
	})
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	var req Request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeHelperError(w, "bad_request", "请求 JSON 非法: "+err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeHelperError(w, "bad_request", err.Error())
		return
	}
	if err := s.checkWritable(); err != nil {
		writeHelperError(w, "read_only", err.Error())
		return
	}
	fingerprint, err := fingerprintRequest(req)
	if err != nil {
		writeHelperError(w, "bad_request", err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cleanupCache() {
		s.saveCacheLocked()
	}
	if cached, ok := s.cache[req.RequestID]; ok {
		if cached.fingerprint != fingerprint {
			writeHelperError(w, "request_id_conflict", "requestId 已用于不同请求")
			return
		}
		writeHelperJSON(w, http.StatusOK, cached.response)
		return
	}
	if err := s.checkWritable(); err != nil {
		writeHelperError(w, "read_only", err.Error())
		return
	}
	response, result := s.executeLocked(r.Context(), req)
	response.Protocol = ProtocolVersion
	s.cache[req.RequestID] = cacheEntry{
		fingerprint: fingerprint, response: response, expires: s.now().Add(CacheTTL),
	}
	s.saveCacheLocked()
	serial := ""
	if result != nil {
		serial = result.Serial
	}
	auditResult := "ok"
	if !response.OK {
		auditResult = "error"
	}
	_ = core.AppendAuditWithOrigin(s.cfg, "helper", "web-"+string(req.Op), req.CN, serial, auditResult)
	writeHelperJSON(w, http.StatusOK, response)
}

func (s *Server) executeLocked(ctx context.Context, req Request) (Response, *Result) {
	platforms, err := core.NormalizePlatforms(req.Platforms)
	if err != nil && len(req.Platforms) > 0 {
		return helperFailure("bad_request", err.Error()), nil
	}
	profile, err := core.ParseP12Profile(req.P12Profile)
	if err != nil {
		return helperFailure("bad_request", err.Error()), nil
	}
	installers, err := core.ParseInstallers(strings.Join(req.Installers, ","))
	if err != nil {
		return helperFailure("bad_request", err.Error()), nil
	}
	issueReq := core.IssueRequest{
		CN: req.CN, Days: req.Days, Platforms: platforms,
		Installers: installers, P12Profile: profile,
	}
	value, err := executeOperation(ctx, s, req, issueReq)
	if err != nil {
		code := "runtime"
		switch core.KindOf(err) {
		case core.KindUsage:
			code = "bad_request"
		case core.KindPrecondition:
			code = "precondition"
		}
		return helperFailure(code, err.Error()), nil
	}
	result := ToResult(value)
	return Response{OK: true, Protocol: ProtocolVersion, Result: result}, result
}

func (s *Server) checkWritable() error {
	cfg, err := s.reloadConfig()
	if err != nil {
		return err
	}
	if cfg.Web.ReadOnly {
		return errors.New("Web 当前为只读模式；请用 root CLI 执行 neca web readonly off")
	}
	return nil
}

func (s *Server) reloadConfig() (*core.Config, error) {
	cfg, err := core.LoadConfigReadOnly()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Server) cleanupCache() bool {
	now := s.now()
	removed := false
	for id, entry := range s.cache {
		if !entry.expires.After(now) {
			delete(s.cache, id)
			removed = true
		}
	}
	return removed
}

// loadCache 在 helper 启动时读取持久化幂等缓存；损坏时从空缓存继续。
func (s *Server) loadCache() {
	data, err := os.ReadFile(s.cacheFile)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		log.Printf("helper: 读取幂等缓存失败: %v", err)
		return
	}
	var persisted map[string]persistedCacheEntry
	if err := json.Unmarshal(data, &persisted); err != nil {
		log.Printf("helper: 幂等缓存 JSON 非法，忽略: %v", err)
		return
	}
	for id, entry := range persisted {
		s.cache[id] = cacheEntry{fingerprint: entry.Fingerprint, response: entry.Response, expires: entry.Expires}
	}
}

// saveCacheLocked 原子写入幂等缓存；必须由持有 s.mu 的调用方调用。
func (s *Server) saveCacheLocked() {
	if s.cacheFile == "" {
		return
	}
	persisted := make(map[string]persistedCacheEntry, len(s.cache))
	for id, entry := range s.cache {
		persisted[id] = persistedCacheEntry{Fingerprint: entry.fingerprint, Response: entry.response, Expires: entry.expires}
	}
	data, err := json.Marshal(persisted)
	if err != nil {
		log.Printf("helper: 序列化幂等缓存失败: %v", err)
		return
	}
	dir := filepath.Dir(s.cacheFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("helper: 创建幂等缓存目录失败: %v", err)
		return
	}
	tmp, err := os.CreateTemp(dir, ".web-helper-cache-*")
	if err != nil {
		log.Printf("helper: 创建幂等缓存临时文件失败: %v", err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	_ = tmp.Chmod(0o600)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		log.Printf("helper: 写入幂等缓存失败: %v", err)
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		log.Printf("helper: fsync 幂等缓存失败: %v", err)
		return
	}
	if err := tmp.Close(); err != nil {
		log.Printf("helper: 关闭幂等缓存临时文件失败: %v", err)
		return
	}
	if err := os.Rename(tmpName, s.cacheFile); err != nil {
		log.Printf("helper: 替换幂等缓存失败: %v", err)
		return
	}
	_ = os.Chmod(s.cacheFile, 0o600)
}

func fingerprintRequest(req Request) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func helperFailure(code, message string) Response {
	return Response{OK: false, Protocol: ProtocolVersion, Error: &Error{Code: code, Message: message}}
}

func writeHelperError(w http.ResponseWriter, code, message string) {
	writeHelperJSON(w, http.StatusOK, helperFailure(code, message))
}

func writeHelperJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// ListenAndServe 在 Unix socket 上提供服务；socket 为 root:gid 0660。
func (s *Server) ListenAndServe(ctx context.Context, socket string, gid int) error {
	if socket == "" {
		socket = DefaultSocketPath
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return fmt.Errorf("创建 helper socket 目录: %w", err)
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return fmt.Errorf("监听 helper socket: %w", err)
	}
	defer os.Remove(socket)
	_ = os.Chmod(socket, 0o660)
	if err := os.Chown(socket, 0, gid); err != nil {
		listener.Close()
		return fmt.Errorf("设置 helper socket 属组: %w", err)
	}
	server := &http.Server{
		Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 150 * time.Second, IdleTimeout: 60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	log.Printf("neca-web-helper %s listening on %s (root:%d 0660)", version.Get().Version, socket, gid)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
