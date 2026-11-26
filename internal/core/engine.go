// 本文件组装核心业务 Engine 与依赖。
package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Engine 负责让新 CRL 在 strongSwan 生效。
type Engine struct {
	cfg    *Config
	runner Runner
}

// NewEngine 创建引擎重载器。
func NewEngine(cfg *Config, runner Runner) *Engine {
	if runner == nil {
		runner = NewExecRunner()
	}
	return &Engine{cfg: cfg, runner: runner}
}

// ReloadResult 是重载结果；警告由 CLI 输出到 stderr。
type ReloadResult struct {
	Warnings []string
}

// Reload 按 NECA_RELOAD_MODE 重载；noReload 时直接返回。
func (e *Engine) Reload(ctx context.Context, noReload bool) ReloadResult {
	result := ReloadResult{}
	if noReload {
		return result
	}
	ps, err := e.runner.Run(ctx, 10*time.Second, e.cfg.DockerBin, "ps", "--format", "{{.Names}}")
	if err != nil || !containsLine(ps.Stdout, e.cfg.Container) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("未找到容器 %s，请手动重启 strongSwan", e.cfg.Container))
		return result
	}

	if e.cfg.ReloadMode == "restart" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("重启 %s 以加载新 CRL（当前 VPN 会话会断开）...", e.cfg.Container))
		if _, err := e.runner.Run(ctx, 120*time.Second, e.cfg.DockerBin, "restart", e.cfg.Container); err != nil {
			result.Warnings = append(result.Warnings, "重启容器失败，请手动检查")
			return result
		}
		for i := 0; i < 40; i++ {
			if _, err := e.runner.Run(ctx, 10*time.Second, e.cfg.DockerBin, "exec", e.cfg.Container, "swanctl", "--stats"); err == nil {
				return result
			}
			time.Sleep(time.Second)
		}
		result.Warnings = append(result.Warnings, "等待 strongSwan 启动超时，请手动检查")
		return result
	}

	if _, err := e.runner.Run(ctx, 60*time.Second, e.cfg.DockerBin, "exec", e.cfg.Container, "swanctl", "--load-creds"); err != nil {
		result.Warnings = append(result.Warnings, "swanctl --load-creds 失败，请手动重启 strongSwan（下次重启自动生效）")
	}
	return result
}

// containsLine 判断命令输出中是否有一行等于目标值。
func containsLine(output, target string) bool {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}
