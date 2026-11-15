package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

const sessionCookieName = "neca_session"

var errSessionExpired = errors.New("session expired")

// sessionClaims 是签名 Cookie 的载荷。
//
// iat 是登录时刻，last 是最近一次认证请求时间，exp 是绝对上限（固定 24h），
// jti 用于登出撤销。Cookie 不保存密码、角色或 PKI 数据。
type sessionClaims struct {
	IssuedAt  int64  `json:"iat"`
	LastSeen  int64  `json:"last"`
	ExpiresAt int64  `json:"exp"`
	ID        string `json:"jti"`
	ClientCN  string `json:"cn,omitempty"`
}

type sessionManager struct {
	secret   []byte
	idle     time.Duration
	absolute time.Duration
	now      func() time.Time

	mu      sync.Mutex
	revoked map[string]time.Time
}

func newSessionManager(secret []byte, idle time.Duration, now func() time.Time) *sessionManager {
	if now == nil {
		now = time.Now
	}
	return &sessionManager{
		secret: secret, idle: idle, absolute: core.AbsoluteWebSessionTimeout, now: now,
		revoked: map[string]time.Time{},
	}
}

func (m *sessionManager) issue(clientCN string) (string, sessionClaims, error) {
	now := m.now().UTC()
	claims := sessionClaims{
		IssuedAt: now.Unix(), LastSeen: now.Unix(),
		ExpiresAt: now.Add(m.absolute).Unix(), ID: randomToken(16), ClientCN: clientCN,
	}
	token, err := m.sign(claims)
	return token, claims, err
}

func (m *sessionManager) sign(claims sessionClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (m *sessionManager) parse(token string) (sessionClaims, error) {
	body, signature, ok := strings.Cut(token, ".")
	if !ok || body == "" || signature == "" {
		return sessionClaims{}, errSessionExpired
	}
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(body))
	expected := mac.Sum(nil)
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || subtle.ConstantTimeCompare(expected, provided) != 1 {
		return sessionClaims{}, errSessionExpired
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return sessionClaims{}, errSessionExpired
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return sessionClaims{}, errSessionExpired
	}
	return claims, nil
}

// validate 校验签名、绝对上限、空闲超时与登出撤销表。
func (m *sessionManager) validate(token string) (sessionClaims, error) {
	claims, err := m.parse(token)
	if err != nil {
		return sessionClaims{}, err
	}
	now := m.now().UTC().Unix()
	if claims.ExpiresAt <= now || claims.IssuedAt <= 0 || claims.LastSeen <= 0 {
		return sessionClaims{}, errSessionExpired
	}
	if claims.LastSeen+int64(m.idle.Seconds()) <= now {
		return sessionClaims{}, errSessionExpired
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if expiry, ok := m.revoked[claims.ID]; ok {
		if expiry.After(m.now()) {
			return sessionClaims{}, errSessionExpired
		}
		delete(m.revoked, claims.ID)
	}
	return claims, nil
}

// renew 更新 lastSeen 并重新签名；绝对上限 exp 保持不变。
func (m *sessionManager) renew(token string) (string, sessionClaims, error) {
	claims, err := m.validate(token)
	if err != nil {
		return "", sessionClaims{}, err
	}
	claims.LastSeen = m.now().UTC().Unix()
	signed, err := m.sign(claims)
	return signed, claims, err
}

func (m *sessionManager) revoke(claims sessionClaims) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revoked[claims.ID] = time.Unix(claims.ExpiresAt, 0)
	now := m.now()
	for id, expiry := range m.revoked {
		if !expiry.After(now) {
			delete(m.revoked, id)
		}
	}
}

func randomToken(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().String()))
	}
	return hex.EncodeToString(buf)
}

func sessionFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool, absolute time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(absolute.Seconds()), Expires: time.Now().UTC().Add(absolute),
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
		Expires: time.Unix(1, 0).UTC(),
	})
}
