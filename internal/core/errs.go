// 本文件定义错误分类和用户提示。
package core

import (
	"errors"
	"fmt"
)

// ErrorKind 表示失败类别，CLI 依此映射退出码（开发文档 §6.3）。
//
// core 不直接决定进程退出码，只把类别带回展示层；这样 CLI、未来 Web
// 与测试可以按各自方式处理同一类错误。
type ErrorKind int

const (
	// KindRuntime 是运行期失败：openssl、docker、文件系统等。
	KindRuntime ErrorKind = iota + 1
	// KindUsage 是用法错误：缺参数、CN 非法、未知子命令。
	KindUsage
	// KindPrecondition 是前置条件不满足：缺 CA、证书已存在、锁定冲突。
	KindPrecondition
)

// Error 是 core 返回的带类别错误。
type Error struct {
	Kind ErrorKind // 失败类别
	Op   string    // 操作名，例如 "读取索引"
	Err  error     // 底层错误
}

// Error 实现 error；Op 为空时只返回底层错误文本。
func (e *Error) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return e.Op + ": " + e.Err.Error()
}

// Unwrap 保留错误链，便于 errors.Is / errors.As 判断哨兵错误。
func (e *Error) Unwrap() error { return e.Err }

// runtimef 构造运行期错误。
func runtimef(op, format string, args ...any) error {
	return &Error{Kind: KindRuntime, Op: op, Err: fmt.Errorf(format, args...)}
}

// usagef 构造用法错误。
func usagef(op, format string, args ...any) error {
	return &Error{Kind: KindUsage, Op: op, Err: fmt.Errorf(format, args...)}
}

// preconditionf 构造前置条件错误。
func preconditionf(op, format string, args ...any) error {
	return &Error{Kind: KindPrecondition, Op: op, Err: fmt.Errorf(format, args...)}
}

// KindOf 返回错误的类别；未知错误按运行期失败处理，
// 与旧脚本 die() 的退出码保持一致。
func KindOf(err error) ErrorKind {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind
	}
	return KindRuntime
}
