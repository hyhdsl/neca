// Package cli 实现 neca 的命令行界面：参数解析、子命令分发与人类可读输出。
//
// 架构约束（开发文档 §3）：
//   - 本包属于展示层，不包含证书业务逻辑；业务实现全部在 internal/core；
//   - core 以结构化值返回结果，本包只负责渲染，绝不反向依赖 cmd/*；
//   - 输出前缀 [OK]/[INFO]/[WARN]/[ERR] 是下游契约（文档 §6.3），不得改动。
//
// M0 只实现 version、help 与 --version；其余子命令按里程碑推进：
//
//	M1 list/verify/export/index sync，M2 写命令，M3 交互向导/--json/--dry-run。
package cli

import (
	"fmt"
	"io"
	"strings"

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

// Run 解析参数并执行对应子命令，返回进程退出码。
//
// 参数：
//   - args：不含程序名的参数列表（即 os.Args[1:]）；
//   - stdout / stderr：由调用方注入，测试时替换为 buffer。
//
// 之所以注入 writer 而不是直接用 fmt.Print 系列，是为了让测试无需重定向
// os.Stdout，也让未来 --json 模式可以整体切换输出目标。
//
// M0 阶段无参数时打印帮助并返回用法错误；M3 接入交互向导后改为默认进入菜单。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		fmt.Fprintf(stderr, "[INFO] 交互向导将在 M3 提供；请先使用子命令，例如：%s version\n", progName)
		return ExitUsage
	}

	cmd := args[0]
	switch cmd {
	case "version":
		// 完整版本摘要：版本、提交、构建时间与目标平台，便于线上核对。
		fmt.Fprintln(stdout, version.Get().String())
		return ExitOK
	case "--version", "-V":
		// 单行版本号，方便脚本直接取值；保持与 version 子命令同源。
		fmt.Fprintf(stdout, "%s %s\n", progName, version.Get().Version)
		return ExitOK
	case "help", "-h", "--help":
		printHelp(stdout)
		return ExitOK
	default:
		// 未知子命令属于用法错误，提示后返回退出码 1（文档 §6.3）。
		fmt.Fprintf(stderr, "[ERR]  未知子命令：%s\n", cmd)
		fmt.Fprintf(stderr, "[INFO] 运行 %s help 查看可用命令\n", progName)
		return ExitUsage
	}
}

// printHelp 输出帮助文本。
//
// M0 只列已实现的命令，避免承诺尚未落地的功能；后续里程碑在功能实现时
// 同步扩充帮助，而不是提前写好"将来会有"的说明。
func printHelp(w io.Writer) {
	lines := []string{
		progName + " —— strongSwan IKEv2 用户证书管理工具",
		"",
		"用法：",
		"  " + progName + " <子命令> [参数]",
		"",
		"可用子命令（M0）：",
		"  version        显示版本、提交号、构建时间与目标平台",
		"  help           显示本帮助",
		"",
		"通用开关：",
		"  --version, -V  只输出版本号（单行）",
		"",
		"后续里程碑将提供：list / verify / export / issue / revoke / reissue 等子命令。",
	}
	fmt.Fprintln(w, strings.Join(lines, "\n"))
}
