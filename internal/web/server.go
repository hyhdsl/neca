// 本文件管理 HTTP 服务生命周期和热重载。
package web

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/helper"
	"github.com/hyhdsl/neca/internal/version"
	"golang.org/x/crypto/bcrypt"
)

//go:embed templates/*.html static/*
var assets embed.FS

// Options 是构造 Web 服务所需的依赖。测试可注入时钟与审计 writer。
type Options struct {
	Config           *core.Config
	Store            *core.FileIndexStore
	OpenSSL          *core.OpenSSL
	PasswordHash     []byte
	SessionKey       []byte
	ProxySecret      string
	SecureCookies    bool
	AllowNonLoopback bool
	Audit            io.Writer
	Now              func() time.Time
	HelperSocket     string
}

// Server 是 Web 面板的 HTTP 服务；写能力由 helper 与 readOnly 双重控制。
type Server struct {
	cfg               *core.Config
	store             *core.FileIndexStore
	oss               *core.OpenSSL
	passwordHash      []byte
	proxySecret       []byte
	secureCookies     bool
	allowNonLoopback  bool
	sessions          *sessionManager
	limiter           *loginLimiter
	audit             *auditLogger
	helperClient      *helper.Client
	translator        *webTranslator
	translators       map[string]*webTranslator
	now               func() time.Time
	templates         map[string]*template.Template
	static            http.Handler
	readOnlyFlag      atomic.Bool
	exportEnabledFlag atomic.Bool
	idleTimeoutNanos  atomic.Int64
}

type sessionContextKey struct{}

// New 校验配置与密钥并加载模板。
func New(opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, coreUsage("Web 启动", "缺少配置")
	}
	if err := opts.Config.Web.Validate(); err != nil {
		return nil, err
	}
	if len(opts.PasswordHash) == 0 {
		return nil, corePrecondition("Web 启动", "未设置 NECA_WEB_PASSWORD_HASH")
	}
	if _, err := bcrypt.Cost(opts.PasswordHash); err != nil {
		return nil, corePrecondition("Web 启动", "NECA_WEB_PASSWORD_HASH 不是合法 bcrypt 哈希: %v", err)
	}
	if len(opts.SessionKey) < 32 {
		return nil, corePrecondition("Web 启动", "会话密钥至少 32 字节（NECA_WEB_SESSION_KEY 或 NECA_WEB_SESSION_KEY_FILE）")
	}
	if len(opts.Config.Web.AdminCNs) > 0 && strings.TrimSpace(opts.ProxySecret) == "" {
		return nil, corePrecondition("Web 启动", "配置 web.adminCNs 时必须设置 NECA_WEB_PROXY_SECRET")
	}
	idle, err := opts.Config.Web.IdleTimeout()
	if err != nil {
		return nil, coreUsage("Web 启动", "%v", err)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	templates, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	translators := map[string]*webTranslator{}
	for _, lang := range []string{"zh-CN", "en-US"} {
		translator, err := newWebTranslator(lang)
		if err != nil {
			return nil, err
		}
		translators[lang] = translator
	}
	defaultTranslator := translators[normalizeWebLanguage(opts.Config.Language)]
	if defaultTranslator == nil {
		defaultTranslator = translators[webDefaultLanguage]
	}
	var helperClient *helper.Client
	if strings.TrimSpace(opts.HelperSocket) != "" {
		helperClient = helper.NewClient(opts.HelperSocket)
	}
	server := &Server{
		cfg: opts.Config, store: opts.Store, oss: opts.OpenSSL,
		passwordHash: opts.PasswordHash, proxySecret: []byte(opts.ProxySecret),
		secureCookies: opts.SecureCookies, allowNonLoopback: opts.AllowNonLoopback,
		helperClient: helperClient,
		translator:   defaultTranslator, translators: translators,
		sessions: newSessionManager(opts.SessionKey, idle, now),
		limiter:  newLoginLimiter(), audit: newAuditLogger(opts.Audit), now: now,
		templates: templates, static: http.FileServer(http.FS(staticFS)),
	}
	server.readOnlyFlag.Store(opts.Config.Web.ReadOnly)
	server.exportEnabledFlag.Store(opts.Config.Web.ExportEnabled)
	server.idleTimeoutNanos.Store(int64(idle))
	server.sessions.SetEpoch(int64(opts.Config.Web.SessionEpoch))
	return server, nil
}

// readOnly 返回当前生效的只读状态；SIGHUP 重载会原子更新该值。
func (s *Server) readOnly() bool { return s.readOnlyFlag.Load() }

// exportEnabled 返回当前生效的 CSV 导出开关。
func (s *Server) exportEnabled() bool { return s.exportEnabledFlag.Load() }

// idleTimeout 返回当前生效的空闲超时；主要用于重载测试与诊断。
func (s *Server) idleTimeout() time.Duration {
	return time.Duration(s.idleTimeoutNanos.Load())
}

// Reload 热更新可安全替换的运行时配置。
//
// 只处理 readOnly 与 idleTimeout；listen/adminCNs/passkey 仍需要重启 necad。
// 失败时不修改当前运行时状态。
func (s *Server) Reload(cfg *core.Config) error {
	if cfg == nil {
		return fmt.Errorf("web reload: 配置为空")
	}
	if !cfg.Web.Enabled {
		return fmt.Errorf("web reload: web.enabled=false，进程需停止")
	}
	if err := cfg.Web.Validate(); err != nil {
		return err
	}
	idle, err := cfg.Web.IdleTimeout()
	if err != nil {
		return err
	}
	s.readOnlyFlag.Store(cfg.Web.ReadOnly)
	s.exportEnabledFlag.Store(cfg.Web.ExportEnabled)
	s.idleTimeoutNanos.Store(int64(idle))
	if s.sessions != nil {
		s.sessions.SetIdle(idle)
		s.sessions.SetEpoch(int64(cfg.Web.SessionEpoch))
	}
	return nil
}

// Handler 返回带安全头、认证与只读保护的 HTTP 处理器。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerPublicRoutes(mux)
	s.registerAuthRoutes(mux)
	s.registerCertRoutes(mux)
	s.registerDownloadRoutes(mux)
	s.registerExportRoutes(mux)
	s.registerAPIRoutes(mux)

	return s.securityHeaders(s.readOnlyGuard(s.authenticate(mux)))
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		if s.secureCookies {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/static/") {
			h.Set("Cache-Control", "public, max-age=3600")
		} else {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// readOnlyGuard 保留 M6a 的安全兜底，并放行 M6b 的写路由。
//
// 写路由自身必须再次检查 readOnly、helper 可用性与 CSRF；这里的放行
// 不是授权，只是让请求到达 handler。未知 POST 仍然 403。
func (s *Server) readOnlyGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/login" || r.URL.Path == "/logout" || isWriteEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusForbidden, "read_only", "Web 面板当前为只读模式")
			return
		}
		http.Error(w, "Web 面板当前为只读模式", http.StatusForbidden)
	})
}

func isWriteEndpoint(path string) bool {
	if path == "/settings/high-risk" || path == "/certs/new" || path == "/api/v1/certs" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/certs/") {
		return strings.HasSuffix(path, "/revoke") || strings.HasSuffix(path, "/unrevoke") || strings.HasSuffix(path, "/reissue")
	}
	if strings.HasPrefix(path, "/certs/") {
		return strings.HasSuffix(path, "/revoke") || strings.HasSuffix(path, "/unrevoke") || strings.HasSuffix(path, "/reissue")
	}
	return false
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/language" || r.URL.Path == "/login" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		if !s.allowNonLoopback && !isLoopbackRequest(r) {
			s.audit.log("access-denied", clientIP(r), "", "non-loopback")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if _, ok := s.checkProxyIdentity(r); !ok {
			s.audit.log("access-denied", clientIP(r), "", "proxy-identity")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		token, ok := sessionFromRequest(r)
		if !ok {
			s.unauthorized(w, r)
			return
		}
		claims, err := s.sessions.validate(token)
		if err != nil {
			clearSessionCookie(w, s.secureCookies)
			s.unauthorized(w, r)
			return
		}
		renewed, claims, err := s.sessions.renew(token)
		if err != nil {
			clearSessionCookie(w, s.secureCookies)
			s.unauthorized(w, r)
			return
		}
		setSessionCookie(w, renewed, s.secureCookies, core.AbsoluteWebSessionTimeout)
		ctx := context.WithValue(r.Context(), sessionContextKey{}, claims)
		r = r.WithContext(ctx)
		if r.Method == http.MethodGet {
			s.ensureCSRF(w, r)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) unauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}
	next := r.URL.RequestURI()
	http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
}

// ListenAndServe 在配置的地址上启动 HTTP 服务，支持 context 取消。
func (s *Server) ListenAndServe(ctx context.Context) error {
	server := &http.Server{
		Addr: s.cfg.Web.Listen, Handler: s.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ln, err := net.Listen("tcp", s.cfg.Web.Listen)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(ln) }()
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

// pageData 是所有模板共用的数据。
type pageData struct {
	Title              string
	Nav                string
	ReadOnly           bool
	Writable           bool
	ExportEnabled      bool
	HelperAvailable    bool
	UserCN             string
	ClientCN           string
	CSRF               string
	Version            string
	Now                time.Time
	Flash              string
	Error              string
	Login              bool
	Next               string
	Lang               string
	Tr                 *webTranslator
	Path               string
	TitleKey           string
	TitleArg           string
	NoticeKey          string
	Stats              dashboardStats
	Certs              []core.WebCert
	Query              string
	Status             string
	Expiry             core.ExpirySummary
	ExpiryWindow       string
	Cert               *core.WebCert
	Verify             *core.VerifyReport
	Downloads          []downloadView
	CRLRevoked         int
	Notice             string
	Days               int
	PlatformChoices    []platformChoice
	P12Profile         string
	Web                *core.WebConfig
	TrashEnabled       bool
	SettingsEnabled    bool
	TrashRetention     string
	IdleTimeoutOptions []string
}

type dashboardStats struct {
	Total      int
	Valid      int
	Revoked    int
	Expired    int
	Unindexed  int
	CRLRevoked int
	Downloads  int
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data pageData) {
	s.renderStatus(w, r, http.StatusOK, name, data)
}

func (s *Server) renderStatus(w http.ResponseWriter, r *http.Request, status int, name string, data pageData) {
	data.ReadOnly = s.readOnly()
	data.TrashEnabled = s.cfg.Web.FeatureEnabled("trash")
	data.SettingsEnabled = s.cfg.Web.FeatureEnabled("settings")
	data.ExportEnabled = s.exportEnabled()
	data.Version = version.Get().Version
	data.Path = r.URL.RequestURI()
	tr := s.translatorFor(r)
	data.Lang = tr.Language()
	data.Tr = tr
	if data.TitleKey != "" {
		if data.TitleArg != "" {
			data.Title = tr.T(data.TitleKey, data.TitleArg)
		} else {
			data.Title = tr.T(data.TitleKey)
		}
	}
	if data.NoticeKey != "" {
		data.Notice = tr.T(data.NoticeKey)
	}
	data.Now = s.now()
	data.CSRF = s.ensureCSRF(w, r)
	if claims, ok := sessionClaimsFrom(r.Context()); ok {
		data.UserCN = claims.ClientCN
	}
	if len(s.cfg.Web.AdminCNs) > 0 {
		data.ClientCN = r.Header.Get("X-Neca-Client-CN")
	}
	tmpl := s.templates[name]
	if tmpl == nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		log.Printf("web: render %s: %v", name, err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func sessionClaimsFrom(ctx context.Context) (sessionClaims, bool) {
	claims, ok := ctx.Value(sessionContextKey{}).(sessionClaims)
	return claims, ok
}

func loadTemplates() (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"statusClass": func(status string) string {
			switch status {
			case "valid":
				return "ok"
			case "revoked":
				return "danger"
			case "expired":
				return "warn"
			default:
				return "muted"
			}
		},
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Local().Format("2006-01-02 15:04")
		},
		"formatBytes": func(n int64) string {
			const unit = 1024
			if n < unit {
				return fmt.Sprintf("%d B", n)
			}
			div, exp := int64(unit), 0
			for n/div >= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
		},
		"join":      strings.Join,
		"hasPrefix": strings.HasPrefix,
	}
	pages := []string{"login", "dashboard", "certs", "cert_new", "cert_detail", "downloads", "settings", "settings_security", "settings_about", "error"}
	out := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		tmpl, err := template.New("layout").Funcs(funcs).ParseFS(assets, "templates/layout.html", "templates/"+page+".html")
		if err != nil {
			return nil, err
		}
		out[page] = tmpl
	}
	return out, nil
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if !s.allowNonLoopback && !isLoopbackRequest(r) {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version.Get().Version, "time": s.now().UTC()})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.checkProxyIdentity(r); !ok {
		s.audit.log("login-denied", clientIP(r), "", "proxy-identity")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if token, ok := sessionFromRequest(r); ok {
		if _, err := s.sessions.validate(token); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	next := safeNext(r.URL.Query().Get("next"))
	s.render(w, r, "login", pageData{TitleKey: "web.login.title", Nav: "login", Login: true, Next: next})
}

func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if strings.ContainsAny(next, "\r\n") {
		return "/"
	}
	return next
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, next string) {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}
