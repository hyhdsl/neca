//go:build !linux

// 本文件为非 Linux 平台提供 bind mount 检测占位实现。
package core

func isMountPoint(string) bool { return false }
