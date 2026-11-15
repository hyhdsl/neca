//go:build !unix

package core

import (
	"io/fs"
	"os"
)

// preserveOwnership 在非 Unix 平台是空操作；测试与开发可在 Windows 构建。
func preserveOwnership(file *os.File, info fs.FileInfo) error { return nil }

func inheritGroupFromDir(path, dir string) error { return nil }

func groupReadMode(dir string, base fs.FileMode) fs.FileMode { return base }
