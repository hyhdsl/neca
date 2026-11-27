//go:build linux

// 本文件在 Linux 上识别单文件 bind mount。
package core

import (
	"os"
	"path/filepath"
	"strings"
)

// isMountPoint 判断路径是否出现在 /proc/self/mountinfo 中。
// 读取失败时返回 false，保持普通原子 rename 路径。
func isMountPoint(path string) bool {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	target := filepath.Clean(path)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if unescapeMountInfo(fields[4]) == target {
			return true
		}
	}
	return false
}

// unescapeMountInfo 还原 mountinfo 中的八进制转义。
func unescapeMountInfo(value string) string {
	return strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(value)
}
