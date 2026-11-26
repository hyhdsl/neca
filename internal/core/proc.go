// 本文件运行外部命令并捕获输出。
package core

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// Result 是一次外部命令执行的结果。
type Result struct {
	Stdout   string        // 标准输出（完整捕获）
	Stderr   string        // 标准错误（完整捕获）
	Code     int           // 退出码；成功为 0
	Duration time.Duration // 执行耗时
}

// Runner 抽象外部命令执行。
//
// 架构约束（开发文档 §3）：所有外部命令必须经此接口执行，便于测试注入、
// 统一超时与错误文本；未来 --dry-run 与远程执行也在这里扩展。
type Runner interface {
	Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error)
}

// ExecRunner 是本机可执行文件的 Runner 实现。
type ExecRunner struct{}

// NewExecRunner 创建默认 Runner。
func NewExecRunner() *ExecRunner { return &ExecRunner{} }

// Run 执行命令并捕获输出；非零退出码返回带 stderr 尾部的运行期错误。
func (r *ExecRunner) Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	res := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.Code = exitErr.ExitCode()
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return res, runtimef(name, "命令超时（%s）", timeout)
		}
		return res, runtimef(name, "%s 失败（退出码 %d）：%s", name, res.Code, stderrTail(res.Stderr))
	}

	return res, nil
}

// stderrTail 取 stderr 末尾最多 500 字节，避免错误信息过长刷屏。
// 错误信息里只出现路径、序列号等非敏感内容（开发文档 §12.3）。
func stderrTail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 500 {
		return s
	}
	return "..." + s[len(s)-500:]
}
