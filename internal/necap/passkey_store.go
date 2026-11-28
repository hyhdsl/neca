// 本文件实现 necap Passkey 凭据元数据存储；公钥可公开，私钥永不进入数据库。
package necap

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type PasskeyRecord struct {
	ID             []byte
	RPID           string
	UserID         string
	CredentialJSON []byte
	Label          string
	CreatedAt      time.Time
	LastUsedAt     time.Time
}

// WebAuthnHandle 返回每个 (RP, 用户) 稳定的随机 User Handle。
func (s *Store) WebAuthnHandle(ctx context.Context, rpID, userID string) ([]byte, error) {
	var handle []byte
	err := s.db.QueryRowContext(ctx, "SELECT handle FROM webauthn_users WHERE rp_id = ? AND user_id = ?", rpID, userID).Scan(&handle)
	if err == nil {
		return handle, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	handle = make([]byte, 32)
	if _, err := rand.Read(handle); err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO webauthn_users(rp_id, user_id, handle, created_at) VALUES(?,?,?,?) ON CONFLICT(rp_id, user_id) DO NOTHING", rpID, userID, handle, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT handle FROM webauthn_users WHERE rp_id = ? AND user_id = ?", rpID, userID).Scan(&handle); err != nil {
		return nil, err
	}
	return handle, nil
}

func (s *Store) PutPasskey(ctx context.Context, record PasskeyRecord) error {
	if len(record.ID) == 0 || record.RPID == "" || record.UserID == "" || len(record.CredentialJSON) == 0 {
		return fmt.Errorf("Passkey 凭据参数非法")
	}
	if record.Label == "" {
		record.Label = "Passkey"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO passkey_credentials(id, rp_id, user_id, credential_json, label, created_at, last_used_at)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET credential_json=excluded.credential_json, label=excluded.label`,
		record.ID, record.RPID, record.UserID, record.CredentialJSON, record.Label, s.now().UTC().Format(time.RFC3339Nano), nullableTime(record.LastUsedAt))
	if err != nil {
		return fmt.Errorf("保存 Passkey: %w", err)
	}
	return nil
}

func (s *Store) ListPasskeys(ctx context.Context, userID string) ([]PasskeyRecord, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, rp_id, user_id, credential_json, label, created_at, last_used_at FROM passkey_credentials WHERE user_id = ? ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []PasskeyRecord
	for rows.Next() {
		var record PasskeyRecord
		var created string
		var lastUsed sql.NullString
		if err := rows.Scan(&record.ID, &record.RPID, &record.UserID, &record.CredentialJSON, &record.Label, &created, &lastUsed); err != nil {
			return nil, err
		}
		record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if lastUsed.Valid {
			record.LastUsedAt, _ = time.Parse(time.RFC3339Nano, lastUsed.String)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) DeletePasskey(ctx context.Context, userID string, id []byte) (bool, error) {
	if len(id) == 0 {
		return false, fmt.Errorf("Passkey ID 为空")
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM passkey_credentials WHERE user_id = ? AND id = ?", userID, id)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changed == 1, nil
}

func (s *Store) TouchPasskey(ctx context.Context, userID string, id []byte) error {
	_, err := s.db.ExecContext(ctx, "UPDATE passkey_credentials SET last_used_at = ? WHERE user_id = ? AND id = ?", s.now().UTC().Format(time.RFC3339Nano), userID, id)
	return err
}

func (s *Store) PasskeyCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM passkey_credentials WHERE user_id = ?", userID).Scan(&count)
	return count, err
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
