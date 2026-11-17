package web

import (
	"context"
	"time"
)

// PasskeyCredential 是未来 WebAuthn 凭据的持久化结构。
//
// M6a 只定义接口与配置开关，不注册注册/登录路由，也不引入 WebAuthn 依赖。
// 字段只保存公钥与元数据，绝不保存私钥。
type PasskeyCredential struct {
	ID         []byte    `json:"id"`
	PublicKey  []byte    `json:"publicKey"`
	SignCount  uint32    `json:"signCount"`
	Label      string    `json:"label"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
}

// PasskeyStore 是凭据存储扩展点；未来可迁移到 SQLite/加密文件。
type PasskeyStore interface {
	Put(ctx context.Context, credential PasskeyCredential) error
	Get(ctx context.Context, id []byte) (PasskeyCredential, error)
	List(ctx context.Context) ([]PasskeyCredential, error)
	Delete(ctx context.Context, id []byte) error
}

// PasskeyAuthenticator 是 WebAuthn 流程扩展点。
//
// M6a 没有实现；启用前必须补齐 RP ID/Origin 校验、挑战一次性、
// 克隆检测与恢复流程（见 docs/evaluations/web-panel.md §6.3）。
type PasskeyAuthenticator interface {
	BeginRegistration(ctx context.Context, adminID string) (any, error)
	FinishRegistration(ctx context.Context, adminID string, response []byte) error
	BeginLogin(ctx context.Context) (any, error)
	FinishLogin(ctx context.Context, response []byte) (string, error)
}
