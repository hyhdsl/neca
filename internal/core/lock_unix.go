//go:build unix

package core

import (
	"errors"
	"os"
	"syscall"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// indexLock 持有 index.txt 的全局 flock。
type indexLock struct {
	file *os.File
}

// acquireIndexLock 以非阻塞方式拿排他锁；拿不到立即返回锁定错误。
func acquireIndexLock(path string) (*indexLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, runtimef("打开索引锁", "%s: %v", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, &Error{Kind: KindPrecondition, Op: "获取索引锁", Err: ports.ErrIndexLocked}
		}
		return nil, runtimef("获取索引锁", "%s: %v", path, err)
	}
	return &indexLock{file: file}, nil
}

// release 释放锁并关闭文件。
func (l *indexLock) release() {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}
