//go:build unix

// 本文件在 Unix 上调整文件属主和权限。
package core

import (
	"io/fs"
	"os"
	"syscall"
)

// inheritGroupFromDir 把 path 的组设置为 dir 的组，保留文件属主。
// 目录组为 root（gid 0）时不改变任何东西，保持原有 0600 行为。
func inheritGroupFromDir(path, dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Gid == 0 {
		return nil
	}
	return os.Chown(path, -1, int(stat.Gid))
}

// groupReadMode 在目录使用非 root 组时增加组读权限（0600 -> 0640）。
func groupReadMode(dir string, base fs.FileMode) fs.FileMode {
	info, err := os.Stat(dir)
	if err != nil {
		return base
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Gid == 0 {
		return base
	}
	return base | 0o040
}

// preserveOwnership 把临时文件的属主/组恢复为原配置文件的属主/组。
//
// Web 启用时 neca.json 是 root:neca-web 0640；CLI 以 root 在容器内原子
// 重写配置后，必须保留组，否则非 root 的 necad 会立即失去读权限。
func preserveOwnership(file *os.File, info fs.FileInfo) error {
	if info == nil {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return file.Chown(int(stat.Uid), int(stat.Gid))
}
