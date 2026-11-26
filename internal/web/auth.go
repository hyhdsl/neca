// 本文件处理登录、密码校验和会话认证。
package web

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const csrfCookieName = "neca_csrf"

// loginLimiter 是进程内失败限流；重启后清空。
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string]*loginWindow
}

type loginWindow struct {
	count        int
	firstSeen    time.Time
	blockedUntil time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: map[string]*loginWindow{}}
}

func (l *loginLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.failures[ip]
	if w == nil {
		return true
	}
	if now.Before(w.blockedUntil) {
		return false
	}
	if now.Sub(w.firstSeen) > 10*time.Minute {
		delete(l.failures, ip)
		return true
	}
	return w.count < 5
}

func (l *loginLimiter) fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.failures[ip]
	if w == nil || now.Sub(w.firstSeen) > 10*time.Minute {
		l.failures[ip] = &loginWindow{count: 1, firstSeen: now}
		return
	}
	w.count++
	if w.count >= 5 {
		w.blockedUntil = now.Add(10 * time.Minute)
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

func (s *Server) checkPassword(password string) bool {
	if len(s.passwordHash) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(s.passwordHash, []byte(password)) == nil
}

// checkProxyIdentity 校验反向代理共享密钥与管理员证书 CN。
//
// 直接通过 OpenSSH 转发访问时 adminCNs 为空，不要求这两个头；一旦配置
// adminCNs，就必须由可信代理提供共享密钥，客户端伪造头不会生效。
func (s *Server) checkProxyIdentity(r *http.Request) (string, bool) {
	if len(s.cfg.Web.AdminCNs) == 0 {
		return "", true
	}
	if !isLoopbackRequest(r) {
		return "", false
	}
	provided := r.Header.Get("X-Neca-Proxy-Secret")
	if subtle.ConstantTimeCompare([]byte(provided), s.proxySecret) != 1 {
		return "", false
	}
	cn := strings.TrimSpace(r.Header.Get("X-Neca-Client-CN"))
	if cn == "" {
		return "", false
	}
	for _, allowed := range s.cfg.Web.AdminCNs {
		if subtle.ConstantTimeCompare([]byte(cn), []byte(allowed)) == 1 {
			return cn, true
		}
	}
	return "", false
}

func (s *Server) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	token := randomToken(32)
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: token, Path: "/", HttpOnly: false,
		Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
		MaxAge: int((24 * time.Hour).Seconds()),
	})
	return token
}

func (s *Server) validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	provided := r.Header.Get("X-CSRF-Token")
	if provided == "" {
		provided = r.FormValue("csrf_token")
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(provided)) == 1
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
