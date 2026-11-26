//go:build windows

// 本文件在 Windows 上提供索引属主占位实现。
package core

import "io/fs"

// Windows has no POSIX uid/gid to preserve; the Web container is Linux-only.
func preserveIndexOwnership(string, fs.FileInfo) error { return nil }
