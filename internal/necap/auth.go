// 本文件实现 necap 的密码登录、第二因素流程、恢复码与管理再认证。
package necap

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrInvalidFactor      = errors.New("第二因素无效")
	ErrPendingExpired     = errors.New("待验证登录已过期")
	ErrNoFactor           = errors.New("用户没有可用的第二因素")
	ErrManagement         = errors.New("管理再认证无效或已过期")
)

type AuthMethod string

const (
	MethodTOTP     AuthMethod = "totp"
	MethodPasskey  AuthMethod = "passkey"
	MethodRecovery AuthMethod = "recovery"
)

type LoginChallenge struct {
	Complete          bool         `json:"complete"`
	UserID            string       `json:"userId"`
	Username          string       `json:"username"`
	PendingToken      string       `json:"pendingToken"`
	Methods           []AuthMethod `json:"methods"`
	RecoveryRemaining int          `json:"recoveryRemaining"`
}

type AuthResult struct {
	UserID   string
	Username string
	Method   AuthMethod
}

type ManagementGrant struct {
	Token   string
	UserID  string
	Expires time.Time
}

type pendingAuth struct {
	UserID   string
	Username string
	Purpose  string
	Methods  map[AuthMethod]bool
	Expires  time.Time
}

type managementGrant struct {
	UserID  string
	Expires time.Time
}

type pendingTOTP struct {
	UserID  string
	Secret  []byte
	Expires time.Time
}

type TOTPEnrollment struct {
	ChallengeID string `json:"challengeId"`
	URI         string `json:"uri"`
}

type AuthService struct {
	store    *Store
	webauthn *WebAuthnService
	cfg      *RuntimeConfig

	mu         sync.Mutex
	pending    map[string]pendingAuth
	management map[string]managementGrant
	totpEnroll map[string]pendingTOTP
	now        func() time.Time
}

func NewAuthService(cfg *RuntimeConfig, store *Store) (*AuthService, error) {
	wa, err := NewWebAuthnService(cfg, store)
	if err != nil {
		return nil, err
	}
	return &AuthService{
		store: store, webauthn: wa, cfg: cfg,
		pending: map[string]pendingAuth{}, management: map[string]managementGrant{},
		totpEnroll: map[string]pendingTOTP{}, now: time.Now,
	}, nil
}

// AuthenticatePassword 是 Web/CLI 第一阶段；policy=password 时直接完成。
func (s *AuthService) AuthenticatePassword(ctx context.Context, username, password string) (*LoginChallenge, error) {
	return s.beginPassword(ctx, username, password, "login")
}

// BeginReauth 要求密码 + 已注册第二因素/恢复码，成功后发管理 token。
func (s *AuthService) BeginReauth(ctx context.Context, username, password string) (*LoginChallenge, error) {
	return s.beginPassword(ctx, username, password, "reauth")
}

func (s *AuthService) beginPassword(ctx context.Context, username, password, purpose string) (*LoginChallenge, error) {
	user, err := s.store.VerifyPassword(ctx, username, password)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}
	policy, err := s.store.Policy(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if policy == PolicyPassword {
		if purpose == "reauth" {
			grant, err := s.issueManagement(user.ID)
			if err != nil {
				return nil, err
			}
			return &LoginChallenge{Complete: true, UserID: user.ID, Username: user.Username, PendingToken: grant.Token}, nil
		}
		return &LoginChallenge{Complete: true, UserID: user.ID, Username: user.Username}, nil
	}
	methods, recovery, err := s.availableMethods(ctx, user.ID, policy)
	if err != nil {
		return nil, err
	}
	if len(methods) == 0 {
		return nil, ErrNoFactor
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pending[token] = pendingAuth{
		UserID: user.ID, Username: user.Username, Purpose: purpose,
		Methods: methodSet(methods), Expires: s.now().Add(s.cfg.SessionDuration),
	}
	s.mu.Unlock()
	return &LoginChallenge{UserID: user.ID, Username: user.Username, PendingToken: token, Methods: methods, RecoveryRemaining: recovery}, nil
}

func (s *AuthService) availableMethods(ctx context.Context, userID string, policy PolicyMode) ([]AuthMethod, int, error) {
	methods := []AuthMethod{}
	if policy == PolicyTOTP || policy == PolicyAny {
		if factor, ok, err := s.store.TOTPStatus(ctx, userID); err != nil {
			return nil, 0, err
		} else if ok && factor.Confirmed {
			methods = append(methods, MethodTOTP)
		}
	}
	if policy == PolicyPasskey || policy == PolicyAny {
		if count, err := s.store.PasskeyCount(ctx, userID); err != nil {
			return nil, 0, err
		} else if count > 0 {
			methods = append(methods, MethodPasskey)
		}
	}
	recovery, err := s.store.RecoveryCodeCount(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	if recovery > 0 {
		methods = append(methods, MethodRecovery)
	}
	return methods, recovery, nil
}

func methodSet(methods []AuthMethod) map[AuthMethod]bool {
	result := make(map[AuthMethod]bool, len(methods))
	for _, method := range methods {
		result[method] = true
	}
	return result
}

func (s *AuthService) pendingFor(token string) (pendingAuth, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[token]
	if !ok || s.now().After(pending.Expires) {
		delete(s.pending, token)
		return pendingAuth{}, false
	}
	return pending, true
}

func (s *AuthService) finishPending(token string, method AuthMethod) (*AuthResult, *ManagementGrant, error) {
	pending, ok := s.pendingFor(token)
	if !ok {
		return nil, nil, ErrPendingExpired
	}
	if !pending.Methods[method] {
		return nil, nil, ErrInvalidFactor
	}
	s.mu.Lock()
	delete(s.pending, token)
	s.mu.Unlock()
	if pending.Purpose == "reauth" {
		grant, err := s.issueManagement(pending.UserID)
		if err != nil {
			return nil, nil, err
		}
		return nil, grant, nil
	}
	return &AuthResult{UserID: pending.UserID, Username: pending.Username, Method: method}, nil, nil
}

// CompleteTOTP 完成 TOTP 登录/再认证；失败时保留 pending 供重试。
func (s *AuthService) CompleteTOTP(ctx context.Context, pendingToken, code string) (*AuthResult, *ManagementGrant, error) {
	pending, ok := s.pendingFor(pendingToken)
	if !ok {
		return nil, nil, ErrPendingExpired
	}
	if !pending.Methods[MethodTOTP] {
		return nil, nil, ErrInvalidFactor
	}
	ok, err := s.store.VerifyTOTP(ctx, pending.UserID, code, s.now())
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrInvalidFactor
	}
	return s.finishPending(pendingToken, MethodTOTP)
}

// CompleteRecovery 消费一次性恢复码完成登录/再认证。
func (s *AuthService) CompleteRecovery(ctx context.Context, pendingToken, code string) (*AuthResult, *ManagementGrant, error) {
	pending, ok := s.pendingFor(pendingToken)
	if !ok {
		return nil, nil, ErrPendingExpired
	}
	if !pending.Methods[MethodRecovery] {
		return nil, nil, ErrInvalidFactor
	}
	ok, err := s.store.ConsumeRecoveryCode(ctx, pending.UserID, code)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrInvalidFactor
	}
	return s.finishPending(pendingToken, MethodRecovery)
}

// BeginPasskey 启动 WebAuthn 登录；返回断言和 challenge ID。
func (s *AuthService) BeginPasskey(ctx context.Context, pendingToken string) (any, string, error) {
	pending, ok := s.pendingFor(pendingToken)
	if !ok {
		return nil, "", ErrPendingExpired
	}
	if !pending.Methods[MethodPasskey] {
		return nil, "", ErrInvalidFactor
	}
	assertion, challengeID, err := s.webauthn.BeginLogin(ctx, pending.UserID)
	if err != nil {
		return nil, "", err
	}
	return assertion, challengeID, nil
}

// FinishPasskey 完成 WebAuthn 登录；成功后才消费 pending。
func (s *AuthService) FinishPasskey(ctx context.Context, pendingToken, challengeID string, body []byte) (*AuthResult, *ManagementGrant, error) {
	pending, ok := s.pendingFor(pendingToken)
	if !ok {
		return nil, nil, ErrPendingExpired
	}
	if !pending.Methods[MethodPasskey] {
		return nil, nil, ErrInvalidFactor
	}
	if err := s.webauthn.FinishLogin(ctx, pending.UserID, challengeID, body); err != nil {
		return nil, nil, ErrInvalidFactor
	}
	return s.finishPending(pendingToken, MethodPasskey)
}

func (s *AuthService) issueManagement(userID string) (*ManagementGrant, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	grant := ManagementGrant{Token: token, UserID: userID, Expires: s.now().Add(s.cfg.SessionDuration)}
	s.mu.Lock()
	s.management[token] = managementGrant{UserID: userID, Expires: grant.Expires}
	s.mu.Unlock()
	return &grant, nil
}

// RequireManagement 校验管理 token；不消费，允许一次短会话内多次设置操作。
func (s *AuthService) RequireManagement(token, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.management[token]
	if !ok || grant.UserID != userID || s.now().After(grant.Expires) {
		delete(s.management, token)
		return ErrManagement
	}
	return nil
}

// SetPolicyWithManagement 修改第二因素策略；要求目标因素已注册。
func (s *AuthService) SetPolicyWithManagement(ctx context.Context, token, userID string, mode PolicyMode) error {
	if err := s.RequireManagement(token, userID); err != nil {
		return err
	}
	if !validPolicy(mode) {
		return fmt.Errorf("非法策略")
	}
	if mode == PolicyPassword {
		return s.store.SetPolicy(ctx, userID, mode)
	}
	methods, _, err := s.availableMethods(ctx, userID, mode)
	if err != nil {
		return err
	}
	if len(methods) == 0 {
		return ErrNoFactor
	}
	return s.store.SetPolicy(ctx, userID, mode)
}

// BeginTOTPEnrollment 生成待确认 secret；明文 secret 不写数据库。
func (s *AuthService) BeginTOTPEnrollment(ctx context.Context, token, userID string) (*TOTPEnrollment, error) {
	if err := s.RequireManagement(token, userID); err != nil {
		return nil, err
	}
	user, err := s.store.UserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("用户不存在")
	}
	encoded, err := NewTOTPSecret()
	if err != nil {
		return nil, err
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	challengeID, err := randomToken()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.totpEnroll[challengeID] = pendingTOTP{UserID: userID, Secret: secret, Expires: s.now().Add(s.cfg.ChallengeDuration)}
	s.mu.Unlock()
	return &TOTPEnrollment{ChallengeID: challengeID, URI: TOTPProvisioningURI("neca", user.Username, encoded, DefaultTOTPDigits, DefaultTOTPPeriod)}, nil
}

// FinishTOTPEnrollment 确认验证码并保存 TOTP；首次启用 2FA 时返回一次性恢复码。
func (s *AuthService) FinishTOTPEnrollment(ctx context.Context, token, userID, challengeID, code string) ([]string, error) {
	if err := s.RequireManagement(token, userID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	pending, ok := s.totpEnroll[challengeID]
	if !ok || pending.UserID != userID || s.now().After(pending.Expires) {
		delete(s.totpEnroll, challengeID)
		s.mu.Unlock()
		return nil, ErrPendingExpired
	}
	s.mu.Unlock()
	if _, ok := ValidateTOTP(pending.Secret, code, s.now(), DefaultTOTPDigits, DefaultTOTPPeriod, DefaultTOTPSkew, "SHA1"); !ok {
		return nil, ErrInvalidFactor
	}
	if err := s.store.PutTOTP(ctx, userID, pending.Secret, DefaultTOTPDigits, DefaultTOTPPeriod, "SHA1", true); err != nil {
		return nil, err
	}
	s.mu.Lock()
	delete(s.totpEnroll, challengeID)
	s.mu.Unlock()
	return s.ensureRecoveryCodes(ctx, userID)
}

func (s *AuthService) ensureRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	count, err := s.store.RecoveryCodeCount(ctx, userID)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil
	}
	codes, err := GenerateRecoveryCodes(DefaultRecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if err := s.store.ReplaceRecoveryCodes(ctx, userID, codes); err != nil {
		return nil, err
	}
	return codes, nil
}

type FactorStatus struct {
	Policy        PolicyMode `json:"policy"`
	TOTP          bool       `json:"totp"`
	Passkeys      int        `json:"passkeys"`
	RecoveryCodes int        `json:"recoveryCodes"`
}

func (s *AuthService) FactorStatus(ctx context.Context, userID string) (FactorStatus, error) {
	policy, err := s.store.Policy(ctx, userID)
	if err != nil {
		return FactorStatus{}, err
	}
	totp, _, err := s.store.TOTPStatus(ctx, userID)
	if err != nil {
		return FactorStatus{}, err
	}
	passkeys, err := s.store.PasskeyCount(ctx, userID)
	if err != nil {
		return FactorStatus{}, err
	}
	recovery, err := s.store.RecoveryCodeCount(ctx, userID)
	if err != nil {
		return FactorStatus{}, err
	}
	return FactorStatus{Policy: policy, TOTP: totp.Confirmed, Passkeys: passkeys, RecoveryCodes: recovery}, nil
}

type PasskeyInfo struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
}

func (s *AuthService) ListPasskeysWithManagement(ctx context.Context, token, userID string) ([]PasskeyInfo, error) {
	if err := s.RequireManagement(token, userID); err != nil {
		return nil, err
	}
	records, err := s.store.ListPasskeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]PasskeyInfo, 0, len(records))
	for _, record := range records {
		result = append(result, PasskeyInfo{
			ID: base64.RawURLEncoding.EncodeToString(record.ID), Label: record.Label,
			CreatedAt: record.CreatedAt, LastUsedAt: record.LastUsedAt,
		})
	}
	return result, nil
}

func (s *AuthService) DeleteTOTPWithManagement(ctx context.Context, token, userID string) error {
	if err := s.RequireManagement(token, userID); err != nil {
		return err
	}
	status, err := s.FactorStatus(ctx, userID)
	if err != nil {
		return err
	}
	if status.Policy == PolicyTOTP || (status.Policy == PolicyAny && status.Passkeys == 0) {
		return fmt.Errorf("不能删除当前策略依赖的唯一第二因素；请先改为仅密码或注册 Passkey")
	}
	return s.store.DeleteTOTP(ctx, userID)
}

func (s *AuthService) DeletePasskeyWithManagement(ctx context.Context, token, userID string, id []byte) error {
	if err := s.RequireManagement(token, userID); err != nil {
		return err
	}
	status, err := s.FactorStatus(ctx, userID)
	if err != nil {
		return err
	}
	if status.Policy == PolicyPasskey || (status.Policy == PolicyAny && !status.TOTP) {
		return fmt.Errorf("不能删除当前策略依赖的唯一第二因素；请先改为仅密码或注册 TOTP")
	}
	removed, err := s.store.DeletePasskey(ctx, userID, id)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("Passkey 不存在")
	}
	return nil
}

func (s *AuthService) RegenerateRecoveryCodes(ctx context.Context, token, userID string) ([]string, error) {
	if err := s.RequireManagement(token, userID); err != nil {
		return nil, err
	}
	codes, err := GenerateRecoveryCodes(DefaultRecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if err := s.store.ReplaceRecoveryCodes(ctx, userID, codes); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *AuthService) BeginPasskeyEnrollment(ctx context.Context, token, userID, label string) (any, string, error) {
	if err := s.RequireManagement(token, userID); err != nil {
		return nil, "", err
	}
	return s.webauthn.BeginRegistration(ctx, userID, label)
}

func (s *AuthService) FinishPasskeyEnrollment(ctx context.Context, token, userID, challengeID string, body []byte, label string) ([]string, error) {
	if err := s.RequireManagement(token, userID); err != nil {
		return nil, err
	}
	if _, err := s.webauthn.FinishRegistration(ctx, userID, challengeID, body, label); err != nil {
		return nil, err
	}
	return s.ensureRecoveryCodes(ctx, userID)
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
