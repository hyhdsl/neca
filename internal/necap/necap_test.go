// 本文件测试 necap 配置、SQLite、密码、TOTP、恢复码和 Passkey 元数据。
package necap

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(context.Background(), filepath.Join(t.TempDir(), "necap.db"), testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func testUser(t *testing.T, store *Store, username string) *User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(context.Background(), username, hash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func TestParseConfigValidAndInvalid(t *testing.T) {
	root := t.TempDir()
	base := `{
		"schemaVersion": 1,
		"rpId": "neca.example.com",
		"rpName": "neca Admin",
		"origins": ["https://neca.example.com"],
		"database": "` + root + `/necap.db",
		"masterKeyFile": "` + root + `/master.key",
		"socket": "` + root + `/necap.sock"
	}`
	cfg, err := ParseConfig([]byte(base), "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserVerification != "preferred" || cfg.ChallengeDuration != 5*time.Minute || cfg.SessionDuration != 10*time.Minute {
		t.Fatalf("defaults = %+v", cfg)
	}
	bad := strings.Replace(base, "https://neca.example.com", "http://neca.example.com", 1)
	if _, err := ParseConfig([]byte(bad), "test"); err == nil {
		t.Fatal("non-local http origin must fail")
	}
	unknown := strings.Replace(base, `"rpName": "neca Admin",`, `"rpName": "neca Admin", "unknown": true,`, 1)
	if _, err := ParseConfig([]byte(unknown), "test"); err == nil {
		t.Fatal("unknown field must fail")
	}
}

func TestTOTPRFCVectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	tests := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, tt := range tests {
		got, _, err := GenerateTOTP(secret, time.Unix(tt.unix, 0), 8, 30, "SHA1")
		if err != nil || got != tt.want {
			t.Fatalf("at %d: got %q, want %q, err=%v", tt.unix, got, tt.want, err)
		}
	}
}

func TestTOTPStoreVerifiesAndPreventsReplay(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	user := testUser(t, store, "admin")
	secret := []byte("12345678901234567890")
	if err := store.PutTOTP(ctx, user.ID, secret, 6, 30, "SHA1", true); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1700000000, 0)
	code, _, err := GenerateTOTP(secret, at, 6, 30, "SHA1")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := store.VerifyTOTP(ctx, user.ID, code, at)
	if err != nil || !ok {
		t.Fatalf("first verify = %v, %v", ok, err)
	}
	ok, err = store.VerifyTOTP(ctx, user.ID, code, at)
	if err != nil || ok {
		t.Fatalf("replay verify = %v, %v", ok, err)
	}
	factor, exists, err := store.TOTPStatus(ctx, user.ID)
	if err != nil || !exists || !factor.Confirmed {
		t.Fatalf("status = %+v, exists=%v, err=%v", factor, exists, err)
	}
}

func TestMasterKeyAndTOTPEncryption(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "master.key")
	key, err := LoadOrCreateMasterKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %o", info.Mode().Perm())
	}
	sealed, err := SealTOTP(key, "user-1", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := OpenTOTP(key, "user-1", sealed)
	if err != nil || string(plain) != "secret" {
		t.Fatalf("open = %q, %v", plain, err)
	}
	if _, err := OpenTOTP(key, "user-2", sealed); err == nil {
		t.Fatal("AAD must bind user")
	}
}

func TestUserPasswordPolicyAndRecoveryCodes(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	user := testUser(t, store, "admin")
	okUser, err := store.VerifyPassword(ctx, "admin", "correct horse battery staple")
	if err != nil || okUser == nil || okUser.ID != user.ID {
		t.Fatalf("password verify = %+v, %v", okUser, err)
	}
	if got, _ := store.VerifyPassword(ctx, "admin", "wrong"); got != nil {
		t.Fatal("wrong password accepted")
	}
	if err := store.SetPolicy(ctx, user.ID, PolicyAny); err != nil {
		t.Fatal(err)
	}
	if mode, err := store.Policy(ctx, user.ID); err != nil || mode != PolicyAny {
		t.Fatalf("policy = %q, %v", mode, err)
	}
	codes, err := GenerateRecoveryCodes(3)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceRecoveryCodes(ctx, user.ID, codes); err != nil {
		t.Fatal(err)
	}
	ok, err := store.ConsumeRecoveryCode(ctx, user.ID, codes[0])
	if err != nil || !ok {
		t.Fatalf("consume = %v, %v", ok, err)
	}
	ok, err = store.ConsumeRecoveryCode(ctx, user.ID, codes[0])
	if err != nil || ok {
		t.Fatalf("reuse = %v, %v", ok, err)
	}
	if count, err := store.RecoveryCodeCount(ctx, user.ID); err != nil || count != 2 {
		t.Fatalf("remaining = %d, %v", count, err)
	}
}

func TestWebAuthnHandleStable(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	user := testUser(t, store, "admin")
	first, err := store.WebAuthnHandle(ctx, "neca.example.com", user.ID)
	if err != nil || len(first) != 32 {
		t.Fatalf("handle = %v, err = %v", first, err)
	}
	second, err := store.WebAuthnHandle(ctx, "neca.example.com", user.ID)
	if err != nil || string(first) != string(second) {
		t.Fatalf("handle changed: %v vs %v, err=%v", first, second, err)
	}
}

func TestPasskeyMetadataLifecycle(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	user := testUser(t, store, "admin")
	record := PasskeyRecord{ID: []byte("credential-1"), RPID: "neca.example.com", UserID: user.ID, CredentialJSON: []byte(`{"id":"credential-1"}`), Label: "Laptop"}
	if err := store.PutPasskey(ctx, record); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListPasskeys(ctx, user.ID)
	if err != nil || len(list) != 1 || list[0].Label != "Laptop" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	removed, err := store.DeletePasskey(ctx, user.ID, record.ID)
	if err != nil || !removed {
		t.Fatalf("delete = %v, %v", removed, err)
	}
	if count, err := store.PasskeyCount(ctx, user.ID); err != nil || count != 0 {
		t.Fatalf("count = %d, %v", count, err)
	}
}

func testAuth(t *testing.T, store *Store) *AuthService {
	t.Helper()
	cfg := &RuntimeConfig{
		Config: Config{
			SchemaVersion: 1, RPID: "neca.example.com", RPName: "neca Admin",
			Origins: []string{"https://neca.example.com"}, UserVerification: "preferred",
		},
		ChallengeDuration: 5 * time.Minute, SessionDuration: 10 * time.Minute,
	}
	auth, err := NewAuthService(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestAuthPasswordOnlyAndTOTP(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	auth := testAuth(t, store)
	user := testUser(t, store, "admin")

	challenge, err := auth.AuthenticatePassword(ctx, "admin", "correct horse battery staple")
	if err != nil || !challenge.Complete || challenge.UserID != user.ID {
		t.Fatalf("password-only = %+v, %v", challenge, err)
	}
	if _, err := auth.AuthenticatePassword(ctx, "admin", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("bad password error = %v", err)
	}

	secret := []byte("12345678901234567890")
	if err := store.PutTOTP(ctx, user.ID, secret, 6, 30, "SHA1", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPolicy(ctx, user.ID, PolicyTOTP); err != nil {
		t.Fatal(err)
	}
	challenge, err = auth.AuthenticatePassword(ctx, "admin", "correct horse battery staple")
	if err != nil || challenge.Complete || challenge.PendingToken == "" || len(challenge.Methods) != 1 || challenge.Methods[0] != MethodTOTP {
		t.Fatalf("2fa challenge = %+v, %v", challenge, err)
	}
	if _, _, err := auth.CompleteTOTP(ctx, challenge.PendingToken, "000000"); err != ErrInvalidFactor {
		t.Fatalf("bad TOTP error = %v", err)
	}
	code, _, err := GenerateTOTP(secret, time.Now(), 6, 30, "SHA1")
	if err != nil {
		t.Fatal(err)
	}
	result, grant, err := auth.CompleteTOTP(ctx, challenge.PendingToken, code)
	if err != nil || result == nil || grant != nil {
		t.Fatalf("complete TOTP = %+v, %+v, %v", result, grant, err)
	}
}

func TestAuthRecoveryAndReauth(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	auth := testAuth(t, store)
	user := testUser(t, store, "admin")
	codes, err := GenerateRecoveryCodes(2)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceRecoveryCodes(ctx, user.ID, codes); err != nil {
		t.Fatal(err)
	}
	secret := []byte("12345678901234567890")
	if err := store.PutTOTP(ctx, user.ID, secret, 6, 30, "SHA1", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPolicy(ctx, user.ID, PolicyAny); err != nil {
		t.Fatal(err)
	}
	challenge, err := auth.AuthenticatePassword(ctx, "admin", "correct horse battery staple")
	if err != nil || challenge.Complete || len(challenge.Methods) != 2 {
		t.Fatalf("challenge = %+v, %v", challenge, err)
	}
	result, _, err := auth.CompleteRecovery(ctx, challenge.PendingToken, codes[0])
	if err != nil || result == nil {
		t.Fatalf("recovery = %+v, %v", result, err)
	}
	if result, _, err := auth.CompleteRecovery(ctx, challenge.PendingToken, codes[0]); err == nil || result != nil {
		t.Fatal("recovery token/code must not be reusable")
	}

	reauth, err := auth.BeginReauth(ctx, "admin", "correct horse battery staple")
	if err != nil || reauth.Complete || reauth.PendingToken == "" {
		t.Fatalf("reauth challenge = %+v, %v", reauth, err)
	}
	code, _, err := GenerateTOTP(secret, time.Now(), 6, 30, "SHA1")
	if err != nil {
		t.Fatal(err)
	}
	result, grant, err := auth.CompleteTOTP(ctx, reauth.PendingToken, code)
	if err != nil || result != nil || grant == nil {
		t.Fatalf("reauth = %+v, %+v, %v", result, grant, err)
	}
	if err := auth.RequireManagement(grant.Token, user.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPEnrollment(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	auth := testAuth(t, store)
	user := testUser(t, store, "admin")
	challenge, err := auth.AuthenticatePassword(ctx, "admin", "correct horse battery staple")
	if err != nil || !challenge.Complete {
		t.Fatalf("password-only = %+v, %v", challenge, err)
	}
	reauth, err := auth.BeginReauth(ctx, "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := auth.issueManagement(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = reauth
	enrollment, err := auth.BeginTOTPEnrollment(ctx, grant.Token, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(enrollment.URI)
	if err != nil {
		t.Fatal(err)
	}
	encoded := parsed.Query().Get("secret")
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := GenerateTOTP(secret, time.Now(), 6, 30, "SHA1")
	if err != nil {
		t.Fatal(err)
	}
	codes, err := auth.FinishTOTPEnrollment(ctx, grant.Token, user.ID, enrollment.ChallengeID, code)
	if err != nil || len(codes) != DefaultRecoveryCodeCount {
		t.Fatalf("finish enrollment = %d codes, %v", len(codes), err)
	}
	if factor, ok, err := store.TOTPStatus(ctx, user.ID); err != nil || !ok || !factor.Confirmed {
		t.Fatalf("factor = %+v, %v, %v", factor, ok, err)
	}
}

func decodeTOTPSecret(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
