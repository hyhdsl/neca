package cli

import (
	"bytes"
	"strings"
	"testing"
)

// runCLI 是测试辅助函数：用内存 buffer 替换真实 stdout/stderr，
// 同时返回退出码，避免测试依赖进程级全局状态。
func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errBuf bytes.Buffer
	code = Run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

// TestVersionSubcommand 校验 `neca version` 输出完整版本摘要且退出码为 0。
func TestVersionSubcommand(t *testing.T) {
	code, out, errOut := runCLI("version")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if !strings.HasPrefix(out, "neca ") {
		t.Fatalf("版本输出必须以程序名开头，实际为：%q", out)
	}
	if !strings.Contains(out, "commit ") || !strings.Contains(out, "built ") {
		t.Fatalf("版本输出缺少 commit/built 字段：%q", out)
	}
	if errOut != "" {
		t.Fatalf("成功路径不应写 stderr，实际为：%q", errOut)
	}
}

// TestVersionFlag 校验 --version 与 -V 只输出一行版本号，
// 这是脚本取值时依赖的稳定契约。
func TestVersionFlag(t *testing.T) {
	for _, flag := range []string{"--version", "-V"} {
		code, out, errOut := runCLI(flag)
		if code != ExitOK {
			t.Fatalf("%s exit code = %d, want %d", flag, code, ExitOK)
		}
		if !strings.HasPrefix(out, "neca ") {
			t.Fatalf("%s 输出必须以程序名开头：%q", flag, out)
		}
		if strings.Count(out, "\n") != 1 {
			t.Fatalf("%s 必须只输出一行，实际为：%q", flag, out)
		}
		if errOut != "" {
			t.Fatalf("%s 不应写 stderr：%q", flag, errOut)
		}
	}
}

// TestHelp 校验三个帮助入口都输出用法说明并返回成功。
func TestHelp(t *testing.T) {
	for _, cmd := range []string{"help", "-h", "--help"} {
		code, out, _ := runCLI(cmd)
		if code != ExitOK {
			t.Fatalf("%s exit code = %d, want %d", cmd, code, ExitOK)
		}
		if !strings.Contains(out, "用法") || !strings.Contains(out, "version") {
			t.Fatalf("%s 帮助文本缺少关键内容：%q", cmd, out)
		}
	}
}

// TestUnknownCommand 校验未知子命令返回用法错误（退出码 1）并写入 stderr。
func TestUnknownCommand(t *testing.T) {
	code, _, errOut := runCLI("frobnicate")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "[ERR]") || !strings.Contains(errOut, "frobnicate") {
		t.Fatalf("stderr 应包含 [ERR] 前缀与命令名，实际为：%q", errOut)
	}
}

// TestNoArgs 校验无参数时打印帮助并返回用法错误。
// M0 尚未接入交互向导（M3），因此当前行为是明确报用法错误而非静默成功。
func TestNoArgs(t *testing.T) {
	code, out, errOut := runCLI()
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(out, "用法") {
		t.Fatalf("无参数时应打印帮助：%q", out)
	}
	if !strings.Contains(errOut, "M3") {
		t.Fatalf("stderr 应提示交互向导尚未实现：%q", errOut)
	}
}

// TestExitCodeContract 固化文档 §6.3 规定的退出码数值。
// 这些常量是对外契约，任何改动都必须同时修改开发文档与下游脚本。
func TestExitCodeContract(t *testing.T) {
	if ExitOK != 0 || ExitUsage != 1 || ExitRuntime != 2 || ExitPrecondition != 3 {
		t.Fatalf("退出码契约被破坏：ok=%d usage=%d runtime=%d precondition=%d",
			ExitOK, ExitUsage, ExitRuntime, ExitPrecondition)
	}
}
