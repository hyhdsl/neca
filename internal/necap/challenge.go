// 本文件实现 necap 的 WebAuthn ceremony challenge 存储；只在内存、单次、短 TTL。
package necap

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

type challengeRecord struct {
	Session webauthn.SessionData
	UserID  string
	Purpose string
	Expires time.Time
}

type ChallengeStore struct {
	mu      sync.Mutex
	records map[string]challengeRecord
	ttl     time.Duration
	now     func() time.Time
}

func NewChallengeStore(ttl time.Duration) *ChallengeStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &ChallengeStore{records: map[string]challengeRecord{}, ttl: ttl, now: time.Now}
}

func (c *ChallengeStore) Put(userID, purpose string, session webauthn.SessionData) (string, error) {
	if userID == "" || purpose == "" {
		return "", fmt.Errorf("challenge 绑定信息为空")
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for key, record := range c.records {
		if now.After(record.Expires) {
			delete(c.records, key)
		}
	}
	c.records[id] = challengeRecord{Session: session, UserID: userID, Purpose: purpose, Expires: now.Add(c.ttl)}
	return id, nil
}

func (c *ChallengeStore) Take(id, userID, purpose string) (webauthn.SessionData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	record, ok := c.records[id]
	if !ok {
		return webauthn.SessionData{}, false
	}
	delete(c.records, id)
	if c.now().After(record.Expires) || record.UserID != userID || record.Purpose != purpose {
		return webauthn.SessionData{}, false
	}
	return record.Session, true
}
