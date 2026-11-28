// 本文件实现 necap 的 TOTP 因子存储、重放防护和状态查询。
package necap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type TOTPFactor struct {
	UserID      string
	Digits      int
	Period      int
	Algorithm   string
	LastCounter uint64
	Confirmed   bool
	CreatedAt   time.Time
}

// PutTOTP 保存加密 secret；confirmed=false 用于待确认的注册流程。
func (s *Store) PutTOTP(ctx context.Context, userID string, secret []byte, digits, period int, algorithm string, confirmed bool) error {
	if len(secret) == 0 || digits < 6 || digits > 8 || period <= 0 {
		return fmt.Errorf("TOTP 因子参数非法")
	}
	if _, err := totpHash(algorithm); err != nil {
		return err
	}
	sealed, err := SealTOTP(s.key, userID, secret)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO totp_factors(user_id, secret_ciphertext, digits, period, algorithm, last_counter, confirmed, created_at)
VALUES(?,?,?,?,?,0,?,?)
ON CONFLICT(user_id) DO UPDATE SET secret_ciphertext=excluded.secret_ciphertext, digits=excluded.digits, period=excluded.period, algorithm=excluded.algorithm, last_counter=0, confirmed=excluded.confirmed, created_at=excluded.created_at`,
		userID, sealed, digits, period, algorithm, boolInt(confirmed), s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("保存 TOTP 因子: %w", err)
	}
	return nil
}

// TOTPStatus 只返回非秘密元数据。
func (s *Store) TOTPStatus(ctx context.Context, userID string) (TOTPFactor, bool, error) {
	var factor TOTPFactor
	var confirmed int
	var created string
	err := s.db.QueryRowContext(ctx, "SELECT user_id, digits, period, algorithm, last_counter, confirmed, created_at FROM totp_factors WHERE user_id = ?", userID).
		Scan(&factor.UserID, &factor.Digits, &factor.Period, &factor.Algorithm, &factor.LastCounter, &confirmed, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return TOTPFactor{}, false, nil
	}
	if err != nil {
		return TOTPFactor{}, false, err
	}
	factor.Confirmed = confirmed != 0
	factor.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return factor, true, nil
}

// VerifyTOTP 校验并消费 counter；成功后 last_counter 前移，旧 counter 立即失效。
func (s *Store) VerifyTOTP(ctx context.Context, userID, code string, at time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var sealed []byte
	var digits, period int
	var algorithm string
	var lastCounter uint64
	var confirmed int
	if err := tx.QueryRowContext(ctx, "SELECT secret_ciphertext, digits, period, algorithm, last_counter, confirmed FROM totp_factors WHERE user_id = ?", userID).
		Scan(&sealed, &digits, &period, &algorithm, &lastCounter, &confirmed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if confirmed == 0 {
		return false, nil
	}
	secret, err := OpenTOTP(s.key, userID, sealed)
	if err != nil {
		return false, fmt.Errorf("解密 TOTP 因子: %w", err)
	}
	counter, ok := ValidateTOTP(secret, code, at, digits, period, DefaultTOTPSkew, algorithm)
	if !ok || counter <= lastCounter {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, "UPDATE totp_factors SET last_counter = ? WHERE user_id = ? AND last_counter < ?", counter, userID, counter)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed == 0 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) DeleteTOTP(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM totp_factors WHERE user_id = ?", userID)
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
