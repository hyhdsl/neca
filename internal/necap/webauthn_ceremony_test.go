// 本文件使用软件 WebAuthn 认证器验证 Passkey 注册与登录 ceremony。
package necap

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

func TestPasskeyWebAuthnCeremony(t *testing.T) {
	store := testStore(t)
	user := testUser(t, store, "passkey-user")
	cfg := &RuntimeConfig{
		Config: Config{
			SchemaVersion: CurrentConfigSchemaVersion,
			RPID:          "neca.example.com",
			RPName:        "neca test",
			Origins:       []string{"https://neca.example.com"},
		},
		ChallengeDuration: 5 * time.Minute,
		SessionDuration:   10 * time.Minute,
	}
	webauthnService, err := NewWebAuthnService(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := store.WebAuthnHandle(context.Background(), cfg.RPID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentialID := make([]byte, 16)
	if _, err := rand.Read(credentialID); err != nil {
		t.Fatal(err)
	}
	origin := cfg.Origins[0]

	creation, challengeID, err := webauthnService.BeginRegistration(context.Background(), user.ID, "Laptop")
	if err != nil {
		t.Fatal(err)
	}
	registrationBody := syntheticRegistration(t, creation, key, credentialID, origin)
	record, err := webauthnService.FinishRegistration(context.Background(), user.ID, challengeID, registrationBody, "Laptop")
	if err != nil {
		t.Fatal(err)
	}
	if string(record.ID) != string(credentialID) {
		t.Fatalf("credential id = %x, want %x", record.ID, credentialID)
	}
	list, err := store.ListPasskeys(context.Background(), user.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("passkeys = %d, err = %v", len(list), err)
	}

	assertion, challengeID, err := webauthnService.BeginLogin(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertionBody := syntheticAssertion(t, assertion, key, credentialID, handle, origin)
	if err := webauthnService.FinishLogin(context.Background(), user.ID, challengeID, assertionBody); err != nil {
		t.Fatal(err)
	}
	count, err := store.PasskeyCount(context.Background(), user.ID)
	if err != nil || count != 1 {
		t.Fatalf("passkey count = %d, err = %v", count, err)
	}
}

func syntheticRegistration(t *testing.T, creation any, key *ecdsa.PrivateKey, credentialID []byte, origin string) []byte {
	t.Helper()
	challenge := ceremonyChallenge(t, creation)
	clientJSON, err := json.Marshal(map[string]any{
		"type": "webauthn.create", "challenge": challenge, "origin": origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte("neca.example.com"))
	authData := make([]byte, 0, 128)
	authData = append(authData, rpHash[:]...)
	authData = append(authData, 0x45) // UP | UV | AT
	authData = append(authData, 0, 0, 0, 0)
	authData = append(authData, make([]byte, 16)...)
	authData = append(authData, byte(len(credentialID)>>8), byte(len(credentialID)))
	authData = append(authData, credentialID...)
	x := key.PublicKey.X.FillBytes(make([]byte, 32))
	y := key.PublicKey.Y.FillBytes(make([]byte, 32))
	cose, err := cbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1, -2: x, -3: y,
	})
	if err != nil {
		t.Fatal(err)
	}
	authData = append(authData, cose...)
	attestation, err := cbor.Marshal(map[string]any{
		"fmt": "none", "attStmt": map[string]any{}, "authData": authData,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"id": rawURL(credentialID), "rawId": rawURL(credentialID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": rawURL(clientJSON), "attestationObject": rawURL(attestation),
			"transports": []string{"internal"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func syntheticAssertion(t *testing.T, assertion any, key *ecdsa.PrivateKey, credentialID, handle []byte, origin string) []byte {
	t.Helper()
	challenge := ceremonyChallenge(t, assertion)
	clientJSON, err := json.Marshal(map[string]any{
		"type": "webauthn.get", "challenge": challenge, "origin": origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte("neca.example.com"))
	authData := make([]byte, 0, 64)
	authData = append(authData, rpHash[:]...)
	authData = append(authData, 0x05) // UP | UV
	authData = append(authData, 0, 0, 0, 1)
	clientHash := sha256.Sum256(clientJSON)
	signed := append(append([]byte(nil), authData...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"id": rawURL(credentialID), "rawId": rawURL(credentialID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": rawURL(clientJSON), "authenticatorData": rawURL(authData),
			"signature": rawURL(signature), "userHandle": rawURL(handle),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func ceremonyChallenge(t *testing.T, ceremony any) string {
	t.Helper()
	raw, err := json.Marshal(ceremony)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	publicKey, ok := envelope["publicKey"].(map[string]any)
	if !ok {
		t.Fatalf("ceremony missing publicKey: %s", raw)
	}
	challenge, ok := publicKey["challenge"].(string)
	if !ok || challenge == "" {
		t.Fatalf("ceremony missing challenge: %s", raw)
	}
	return challenge
}

func rawURL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}
