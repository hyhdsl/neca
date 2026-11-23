//go:build windows

package core

import "io/fs"

// Windows has no POSIX uid/gid to preserve; the Web container is Linux-only.
func preserveIndexOwnership(string, fs.FileInfo) error { return nil }
