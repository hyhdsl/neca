//go:build !unix

package core

import "github.com/hyhdsl/neca/internal/core/ports"

// indexLock 是 Windows 等非 Unix 平台的占位实现。
type indexLock struct{}

// acquireIndexLock 在非 Unix 平台直接报错：neca 生产目标是 Linux，
// 文件级 flock 只在 Unix 提供；Windows 仅用于交叉编译与单元测试。
func acquireIndexLock(path string) (*indexLock, error) {
	return nil, &Error{Kind: KindPrecondition, Op: "获取索引锁", Err: ports.ErrIndexLocked}
}

// release 无操作。
func (l *indexLock) release() {}
