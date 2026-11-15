// Package cli 实现 neca 的命令行界面：参数解析、子命令分发与人类可读输出。
//
// 架构约束（开发文档 §3）：
//   - 本包属于展示层，不包含证书业务逻辑；业务实现全部在 internal/core；
//   - core 以结构化值返回结果，本包只负责渲染，绝不反向依赖 cmd/*；
//   - 输出前缀 [OK]/[INFO]/[WARN]/[ERR] 是下游契约（文档 §6.3），不得改动。
//
// 命令通过 Registry（command.go）注册：新增子命令只需实现 Command.Run 并
// 注册，不需要修改 Run 的分发逻辑或帮助文本。这是为 M1/M2 批量增加
// list/issue/revoke 等命令、以及未来 Web 端复用命令元数据预留的扩展点。
//
// 当前已实现：M1 只读命令、M2 写命令、M3 通用开关/JSON/向导/语言。
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/i18n"
	"github.com/hyhdsl/neca/internal/version"
)

// 退出码是 CLI 契约的一部分（文档 §6.3），脚本与下游依赖这些数值，
// 修改前必须同步文档与 cli_test.go 中的 TestExitCodeContract。
const (
	ExitOK           = 0 // 成功（允许附带 [WARN]）
	ExitUsage        = 1 // 用法错误：缺参数、CN 非法、未知子命令
	ExitRuntime      = 2 // 运行期失败：openssl、docker、文件系统
	ExitPrecondition = 3 // 前置条件不满足：证书已存在、容器未找到、缺 CA、锁定冲突
)

// progName 是面向用户的程序名，统一从这里取，避免各处硬编码。
const progName = "neca"

// Run 是保持旧签名的入口；测试可继续注入 stdout/stderr。
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithIO(args, os.Stdin, stdout, stderr)
}

// RunWithIO 解析参数并执行对应子命令，支持可注入 stdin（向导/测试）。
func RunWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	args = compatArgs(args)
	rest, opts, err := parseGlobals(args)
	if err != nil {
		fmt.Fprintf(stderr, "[ERR]  %v\n", err)
		return ExitUsage
	}
	if opts.Config != "" {
		if err := os.Setenv("NECA_CONFIG", opts.Config); err != nil {
			fmt.Fprintf(stderr, "[ERR]  设置 NECA_CONFIG: %v\n", err)
			return ExitRuntime
		}
	}
	if opts.ReloadMode != "" {
		if opts.ReloadMode != "creds" && opts.ReloadMode != "restart" {
			fmt.Fprintf(stderr, "[ERR]  --reload-mode 必须是 creds 或 restart\n")
			return ExitUsage
		}
		if err := os.Setenv("NECA_RELOAD_MODE", opts.ReloadMode); err != nil {
			fmt.Fprintf(stderr, "[ERR]  设置 NECA_RELOAD_MODE: %v\n", err)
			return ExitRuntime
		}
	}
	configLang, err := core.ReadConfigLanguage(core.ConfigFilePath())
	if err != nil {
		fmt.Fprintf(stderr, "[ERR]  %v\n", err)
		return ExitUsage
	}
	lang, err := i18n.ResolveWithConfig(opts.Lang, configLang)
	if err != nil {
		fmt.Fprintf(stderr, "[ERR]  %s\n", i18n.T("error.language", opts.Lang))
		return ExitUsage
	}
	_ = i18n.SetLanguage(lang)

	registry := newBuiltinRegistry()
	env := Env{
		Stdin: stdin, Stdout: stdout, Stderr: stderr,
		JSON: opts.JSON, DryRun: opts.DryRun, Quiet: opts.Quiet,
		Lang: lang, Started: time.Now(),
	}

	if opts.Version {
		env.Action = "version"
		info := version.Get()
		if opts.JSON {
			return emitJSON(env, "version", "", nil, map[string]any{
				"version": info.Version, "commit": info.Commit,
				"date": info.Date, "goos": info.OS, "goarch": info.Arch,
			})
		}
		fmt.Fprintf(stdout, "%s %s\n", progName, info.Version)
		return ExitOK
	}

	if len(rest) == 0 {
		if opts.JSON {
			env.Action = "wizard"
			return emitUsageJSONError(env, fmt.Errorf("wizard is not available in JSON mode"))
		}
		env.Action = "wizard"
		return runWizard(env, registry)
	}

	cmd, ok := registry.Lookup(rest[0])
	if !ok {
		unknown := fmt.Errorf(i18n.T("error.unknown_command"), rest[0])
		if opts.JSON {
			env.Action = rest[0]
			return emitUsageJSONError(env, unknown)
		}
		fmt.Fprintf(stderr, "[ERR]  %v\n", unknown)
		fmt.Fprintf(stderr, "[INFO] %s\n", i18n.T("error.help_hint", progName))
		return ExitUsage
	}
	env.Action = cmd.Name
	env.CN = guessCN(cmd.Name, rest[1:])
	env.Args = rest[1:]
	if opts.Quiet && !opts.JSON {
		env.Stdout = io.Discard
	}

	if opts.DryRun && isWriteCommand(cmd.Name, rest[1:]) {
		if opts.JSON {
			return emitJSON(env, cmd.Name, env.CN, nil, map[string]any{
				"dryRun":  true,
				"command": cmd.Name,
				"args":    rest[1:],
			})
		}
		fmt.Fprintln(stdout, i18n.T("dryrun.line", cmd.Name, strings.Join(rest[1:], " ")))
		fmt.Fprintln(stdout, i18n.T("dryrun.done"))
		return ExitOK
	}

	if opts.JSON {
		return runJSONCommand(env, cmd, stdout, stderr)
	}
	if lang == "en-US" && cmd.Name != "wizard" && !opts.Quiet {
		return runLocalizedCommand(env, cmd, stdout, stderr)
	}
	return cmd.Run(env)
}

// newBuiltinRegistry 构建内置命令表。
func newBuiltinRegistry() *Registry {
	registry := NewRegistry()

	registry.MustRegister(Command{
		Name:    "version",
		Summary: "显示版本、提交号、构建时间与目标平台",
		Run: func(env Env) int {
			if env.JSON {
				info := version.Get()
				return emitJSON(env, "version", "", nil, map[string]any{
					"version": info.Version, "commit": info.Commit,
					"date": info.Date, "goos": info.OS, "goarch": info.Arch,
				})
			}
			fmt.Fprintln(env.Stdout, version.Get().String())
			return ExitOK
		},
	})

	registry.MustRegister(Command{
		Name:    "help",
		Aliases: []string{"-h", "--help"},
		Summary: "显示本帮助",
		Run: func(env Env) int {
			registry.WriteHelp(env.Stdout)
			return ExitOK
		},
	})

	// M1 只读命令：全部经 core 的结构化结果渲染，CLI 不直接调用 openssl。
	registry.MustRegister(newListCommand())
	registry.MustRegister(newVerifyCommand())
	registry.MustRegister(newExportCommand())
	registry.MustRegister(newIndexCommand())

	// M2 写命令。
	registry.MustRegister(newIssueCommand())
	registry.MustRegister(newReissueCommand())
	registry.MustRegister(newRevokeCommand())
	registry.MustRegister(newUnrevokeCommand())
	registry.MustRegister(newDeleteCommand())
	registry.MustRegister(newPlatformCommand())
	registry.MustRegister(newPackageCommand())
	registry.MustRegister(newDownloadsCommand())
	registry.MustRegister(newConfigCommand())

	// M3 交互向导。
	registry.MustRegister(newWizardCommand(registry))

	// 内置命令使用语言文件中的说明；第三方/测试注册的自定义命令保持 Summary。
	for _, cmd := range registry.Commands() {
		cmd.SummaryKey = "cmd." + cmd.Name + ".summary"
	}
	return registry
}
