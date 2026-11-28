// 本文件实现 necap 的 Passkey 注册/登录 ceremony 与凭据持久化衔接。
package necap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type WebAuthnService struct {
	cfg        *RuntimeConfig
	store      *Store
	rp         *webauthn.WebAuthn
	challenges *ChallengeStore
}

func NewWebAuthnService(cfg *RuntimeConfig, store *Store) (*WebAuthnService, error) {
	if cfg == nil || store == nil {
		return nil, fmt.Errorf("necap WebAuthn 依赖为空")
	}
	service := &WebAuthnService{cfg: cfg, store: store}
	rp, err := webauthn.New(&webauthn.Config{
		RPDisplayName: cfg.RPName,
		RPID:          cfg.RPID,
		RPOrigins:     append([]string(nil), cfg.Origins...),
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: service.userVerification(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 WebAuthn: %w", err)
	}
	service.rp = rp
	service.challenges = NewChallengeStore(cfg.ChallengeDuration)
	return service, nil
}

type passkeyUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.id }
func (u *passkeyUser) WebAuthnName() string                       { return u.name }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *WebAuthnService) loadUser(ctx context.Context, userID string) (*passkeyUser, error) {
	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Disabled {
		return nil, fmt.Errorf("用户不存在或已禁用")
	}
	handle, err := s.store.WebAuthnHandle(ctx, s.cfg.RPID, user.ID)
	if err != nil {
		return nil, err
	}
	records, err := s.store.ListPasskeys(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		var credential webauthn.Credential
		if err := json.Unmarshal(record.CredentialJSON, &credential); err != nil {
			return nil, fmt.Errorf("解析 Passkey 凭据: %w", err)
		}
		credentials = append(credentials, credential)
	}
	return &passkeyUser{id: handle, name: user.Username, displayName: user.Username, credentials: credentials}, nil
}

func (s *WebAuthnService) userVerification() protocol.UserVerificationRequirement {
	switch s.cfg.UserVerification {
	case "required":
		return protocol.VerificationRequired
	case "discouraged":
		return protocol.VerificationDiscouraged
	default:
		return protocol.VerificationPreferred
	}
}

func (s *WebAuthnService) BeginRegistration(ctx context.Context, userID, label string) (*protocol.CredentialCreation, string, error) {
	user, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	creation, session, err := s.rp.BeginRegistration(user)
	if err != nil {
		return nil, "", fmt.Errorf("开始 Passkey 注册: %w", err)
	}
	challengeID, err := s.challenges.Put(userID, "register", *session)
	if err != nil {
		return nil, "", err
	}
	return creation, challengeID, nil
}

func (s *WebAuthnService) FinishRegistration(ctx context.Context, userID, challengeID string, body []byte, label string) (*PasskeyRecord, error) {
	session, ok := s.challenges.Take(challengeID, userID, "register")
	if !ok {
		return nil, fmt.Errorf("Passkey 注册 challenge 无效或已过期")
	}
	user, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	request, err := ceremonyRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	credential, err := s.rp.FinishRegistration(user, session, request)
	if err != nil {
		return nil, fmt.Errorf("完成 Passkey 注册: %w", err)
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(label) == "" {
		label = "Passkey"
	}
	record := PasskeyRecord{
		ID: credential.ID, RPID: s.cfg.RPID, UserID: userID,
		CredentialJSON: encoded, Label: label,
	}
	if err := s.store.PutPasskey(ctx, record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *WebAuthnService) BeginLogin(ctx context.Context, userID string) (*protocol.CredentialAssertion, string, error) {
	user, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if len(user.credentials) == 0 {
		return nil, "", fmt.Errorf("用户没有已注册的 Passkey")
	}
	assertion, session, err := s.rp.BeginLogin(user, webauthn.WithUserVerification(s.userVerification()))
	if err != nil {
		return nil, "", fmt.Errorf("开始 Passkey 登录: %w", err)
	}
	challengeID, err := s.challenges.Put(userID, "login", *session)
	if err != nil {
		return nil, "", err
	}
	return assertion, challengeID, nil
}

func (s *WebAuthnService) FinishLogin(ctx context.Context, userID, challengeID string, body []byte) error {
	session, ok := s.challenges.Take(challengeID, userID, "login")
	if !ok {
		return fmt.Errorf("Passkey 登录 challenge 无效或已过期")
	}
	user, err := s.loadUser(ctx, userID)
	if err != nil {
		return err
	}
	request, err := ceremonyRequest(ctx, body)
	if err != nil {
		return err
	}
	credential, err := s.rp.FinishLogin(user, session, request)
	if err != nil {
		return fmt.Errorf("完成 Passkey 登录: %w", err)
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	record := PasskeyRecord{
		ID: credential.ID, RPID: s.cfg.RPID, UserID: userID,
		CredentialJSON: encoded, Label: "Passkey",
	}
	if err := s.store.PutPasskey(ctx, record); err != nil {
		return err
	}
	return s.store.TouchPasskey(ctx, userID, credential.ID)
}

func ceremonyRequest(ctx context.Context, body []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://necap.local/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}
