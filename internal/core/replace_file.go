// 本文件提供单文件 bind mount 安全的替换逻辑。
package core

import (
	"io"
	"os"
)

// replaceFile 默认使用原子 rename；当目标本身是 Linux 单文件 bind mount 时，
// rename 会替换宿主机 inode，但容器内已挂载的旧 inode 不会更新，导致 Web 读到旧索引。
// 这种情况下改为写回原 inode，牺牲 rename 的原子性以保持读者可见性。
func replaceFile(tmpPath, dstPath string) error {
	if inPlaceWrites() || isMountPoint(dstPath) {
		if err := copyFileIntoExisting(tmpPath, dstPath); err != nil {
			return err
		}
		return os.Remove(tmpPath)
	}
	return os.Rename(tmpPath, dstPath)
}

// inPlaceWrites 允许 helper 在单文件 bind mount 场景下显式改用保留 inode 的写回。
func inPlaceWrites() bool { return os.Getenv("NECA_INDEX_INPLACE") == "1" }

// copyFileIntoExisting 将临时文件内容写回已存在的目标文件，保留目标 inode。
func copyFileIntoExisting(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}
