// 本文件实现 necap 的 SQLite 存储、schema 迁移和用户/策略基础操作。
package necap

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

const CurrentStoreVersion = 1

// PolicyMode 是密码之外的第二因素策略；密码始终是第一阶段。
type PolicyMode string

const (
	PolicyPassword PolicyMode = "password"
	PolicyTOTP     PolicyMode = "totp"
	PolicyPasskey  PolicyMode = "passkey"
	PolicyAny      PolicyMode = "any"
)

// User 是 necap 的用户记录；PasswordHash 为 bcrypt。
type User struct {
	ID           string
	Username     string
	PasswordHash []byte
	Role         string
	Disabled     bool
	CreatedAt    time.Time
}

// Store 封装 necap SQLite 数据库和 TOTP 主密钥。
type Store struct {
	db  *sql.DB
	key []byte
	now func() time.Time
}

// OpenStore 打开/创建 SQLite 数据库、执行迁移并收紧文件权限。
func OpenStore(ctx context.Context, dbPath string, key []byte) (*Store, error) {
	if len(key) != masterKeySize {
		return nil, fmt.Errorf("主密钥长度必须为 %d 字节", masterKeySize)
	}
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" || !filepath.IsAbs(dbPath) {
		return nil, fmt.Errorf("necap 数据库路径必须是绝对路径")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("创建 necap 数据库目录: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开 necap 数据库: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接 necap 数据库: %w", err)
	}
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("设置 necap 数据库 %s: %w", pragma, err)
		}
	}
	store := &Store{db: db, key: append([]byte(nil), key...), now: time.Now}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if _, err := os.Stat(path); err == nil {
			if err := os.Chmod(path, 0o600); err != nil {
				db.Close()
				return nil, fmt.Errorf("收紧 necap 数据库权限 %s: %w", path, err)
			}
		}
	}
	return store, nil
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("读取 necap schema 版本: %w", err)
	}
	if version > CurrentStoreVersion {
		return fmt.Errorf("necap 数据库 schemaVersion=%d 高于当前支持的 %d", version, CurrentStoreVersion)
	}
	if version == CurrentStoreVersion {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 necap 迁移: %w", err)
	}
	defer tx.Rollback()
	if version < 1 {
		if _, err := tx.ExecContext(ctx, necapSchemaV1); err != nil {
			return fmt.Errorf("创建 necap schema v1: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", CurrentStoreVersion)); err != nil {
		return fmt.Errorf("写入 necap schema 版本: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 necap 迁移: %w", err)
	}
	return nil
}

const necapSchemaV1 = `
CREATE TABLE IF NOT EXISTS users (
	id            TEXT PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE,
	password_hash BLOB NOT NULL,
	role          TEXT NOT NULL DEFAULT 'admin',
	disabled      INTEGER NOT NULL DEFAULT 0,
	created_at    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS auth_policy (
	user_id    TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	mode       TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS totp_factors (
	user_id            TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	secret_ciphertext  BLOB NOT NULL,
	digits             INTEGER NOT NULL,
	period             INTEGER NOT NULL,
	algorithm          TEXT NOT NULL,
	last_counter       INTEGER NOT NULL DEFAULT 0,
	confirmed          INTEGER NOT NULL DEFAULT 0,
	created_at         TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS webauthn_users (
	rp_id     TEXT NOT NULL,
	user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	handle    BLOB NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (rp_id, user_id),
	UNIQUE (rp_id, handle)
);
CREATE TABLE IF NOT EXISTS passkey_credentials (
	id              BLOB PRIMARY KEY,
	rp_id           TEXT NOT NULL,
	user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	credential_json BLOB NOT NULL,
	label           TEXT NOT NULL,
	created_at      TEXT NOT NULL,
	last_used_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_passkey_user ON passkey_credentials(user_id, rp_id);
CREATE TABLE IF NOT EXISTS recovery_codes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	hash       BLOB NOT NULL,
	used       INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	used_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_recovery_user_unused ON recovery_codes(user_id, used);
`

func (s *Store) Close() error { return s.db.Close() }

// CreateUser 创建用户并默认使用仅密码策略；passwordHash 必须是 bcrypt 哈希。
func (s *Store) CreateUser(ctx context.Context, username string, passwordHash []byte, role string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(passwordHash) == 0 {
		return nil, fmt.Errorf("用户名和密码哈希不能为空")
	}
	if role == "" {
		role = "admin"
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id, username, password_hash, role, disabled, created_at) VALUES(?,?,?,?,0,?)", id, username, passwordHash, role, now.Format(time.RFC3339Nano)); err != nil {
		return nil, fmt.Errorf("创建 necap 用户: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO auth_policy(user_id, mode, updated_at) VALUES(?,?,?)", id, string(PolicyPassword), now.Format(time.RFC3339Nano)); err != nil {
		return nil, fmt.Errorf("创建 necap 策略: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交 necap 用户: %w", err)
	}
	return &User{ID: id, Username: username, PasswordHash: append([]byte(nil), passwordHash...), Role: role, CreatedAt: now}, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	return s.queryUser(ctx, "SELECT id, username, password_hash, role, disabled, created_at FROM users WHERE username = ?", strings.TrimSpace(username))
}

func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return s.queryUser(ctx, "SELECT id, username, password_hash, role, disabled, created_at FROM users WHERE id = ?", id)
}

func (s *Store) queryUser(ctx context.Context, query, value string) (*User, error) {
	var user User
	var disabled int
	var created string
	if err := s.db.QueryRowContext(ctx, query, value).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &disabled, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	user.Disabled = disabled != 0
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return &user, nil
}

func (s *Store) SetPasswordHash(ctx context.Context, userID string, passwordHash []byte) error {
	if len(passwordHash) == 0 {
		return fmt.Errorf("密码哈希不能为空")
	}
	result, err := s.db.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, userID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// VerifyPassword 校验启用用户的 bcrypt 密码；未知用户返回 nil。
func (s *Store) VerifyPassword(ctx context.Context, username, password string) (*User, error) {
	user, err := s.UserByUsername(ctx, username)
	if err != nil || user == nil || user.Disabled {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)) != nil {
		return nil, nil
	}
	return user, nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) SetPolicy(ctx context.Context, userID string, mode PolicyMode) error {
	if !validPolicy(mode) {
		return fmt.Errorf("非法 necap 策略: %q", mode)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO auth_policy(user_id, mode, updated_at) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET mode=excluded.mode, updated_at=excluded.updated_at", userID, string(mode), s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return nil
}

func (s *Store) Policy(ctx context.Context, userID string) (PolicyMode, error) {
	var mode string
	err := s.db.QueryRowContext(ctx, "SELECT mode FROM auth_policy WHERE user_id = ?", userID).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return PolicyPassword, nil
	}
	if err != nil {
		return "", err
	}
	if !validPolicy(PolicyMode(mode)) {
		return "", fmt.Errorf("数据库中存在非法策略: %q", mode)
	}
	return PolicyMode(mode), nil
}

func validPolicy(mode PolicyMode) bool {
	switch mode {
	case PolicyPassword, PolicyTOTP, PolicyPasskey, PolicyAny:
		return true
	default:
		return false
	}
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
