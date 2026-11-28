// 本文件实现 necap 一次性恢复码的生成、哈希存储和消费。
package necap

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const DefaultRecoveryCodeCount = 10

// GenerateRecoveryCodes 生成便于抄写的恢复码；明文只应展示一次。
func GenerateRecoveryCodes(count int) ([]string, error) {
	if count <= 0 {
		count = DefaultRecoveryCodeCount
	}
	codes := make([]string, count)
	for i := range codes {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		text := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
		parts := make([]string, 0, 4)
		for len(text) > 0 {
			n := 4
			if len(text) < n {
				n = len(text)
			}
			parts = append(parts, text[:n])
			text = text[n:]
		}
		codes[i] = strings.Join(parts, "-")
	}
	return codes, nil
}

// ReplaceRecoveryCodes 替换某用户的全部恢复码；数据库中只保留 bcrypt 哈希。
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID string, codes []string) error {
	if len(codes) == 0 {
		return fmt.Errorf("恢复码不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id = ?", userID); err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			return fmt.Errorf("恢复码不能为空")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO recovery_codes(user_id, hash, used, created_at) VALUES(?,?,0,?)", userID, hash, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ConsumeRecoveryCode 校验并立即消费一个未使用恢复码。
func (s *Store) ConsumeRecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return false, nil
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, hash FROM recovery_codes WHERE user_id = ? AND used = 0", userID)
	if err != nil {
		return false, err
	}
	type candidate struct {
		id   int64
		hash []byte
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.hash); err != nil {
			rows.Close()
			return false, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	for _, item := range candidates {
		if bcrypt.CompareHashAndPassword(item.hash, []byte(code)) == nil {
			result, err := s.db.ExecContext(ctx, "UPDATE recovery_codes SET used = 1, used_at = ? WHERE id = ? AND used = 0", s.now().UTC().Format(time.RFC3339Nano), item.id)
			if err != nil {
				return false, err
			}
			changed, err := result.RowsAffected()
			if err != nil {
				return false, err
			}
			return changed == 1, nil
		}
	}
	return false, nil
}

func (s *Store) RecoveryCodeCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used = 0", userID).Scan(&count)
	return count, err
}
