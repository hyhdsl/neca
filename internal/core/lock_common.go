package core

// withPkiLock 在 PKI 全局锁内执行 fn；锁冲突返回包装了 ports.ErrIndexLocked 的错误。
//
// 写命令（issue/reissue/revoke/unrevoke/delete/platform）必须在外层使用它，
// 内层索引读写使用 FileIndexStore.UpdateLocked，避免同一进程重复加锁。
func withPkiLock(lockPath string, fn func() error) error {
	lock, err := acquireIndexLock(lockPath)
	if err != nil {
		return err
	}
	defer lock.release()
	return fn()
}
