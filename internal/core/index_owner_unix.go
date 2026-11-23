//go:build !windows

package core

import (
	"io/fs"
	"os"
	"syscall"
)

// preserveIndexOwnership copies the uid/gid of the existing index file to the
// temporary replacement before the atomic rename. Without this, every rewrite
// creates a new root-owned inode and the neca-web process (UID/GID 10001) loses
// read access to index.txt.
func preserveIndexOwnership(path string, info fs.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(path, int(st.Uid), int(st.Gid))
}
