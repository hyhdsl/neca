// 本文件测试 completion 命令的四种脚本输出与错误路径。
package cli

import (
	"strings"
	"testing"
)

// TestCompletionShells 校验四种 shell 都能生成非空脚本并包含命令、开关与别名。
func TestCompletionShells(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{"bash", []string{"complete -F _neca_completion neca", "cert", "completion", "--platforms", "--json", "--help"}},
		{"zsh", []string{"#compdef neca", "compdef _neca neca", "cert", "--platforms", "--json"}},
		{"fish", []string{"complete -c neca", "__fish_use_subcommand", "cert", "--platforms", "--json"}},
		{"powershell", []string{"Register-ArgumentCompleter", "'cert'", "--platforms", "--json", "pwsh"}},
	}
	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			code, out, errOut := runCLI("completion", tc.shell)
			if code != ExitOK {
				t.Fatalf("exit code = %d, want %d; stderr=%q", code, ExitOK, errOut)
			}
			if errOut != "" {
				t.Fatalf("成功路径不应写 stderr: %q", errOut)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("%s 补全缺少 %q", tc.shell, want)
				}
			}
		})
	}
}

// TestCompletionPwshAlias 校验 pwsh 可作为 powershell 的简写。
func TestCompletionPwshAlias(t *testing.T) {
	code, out, errOut := runCLI("completion", "pwsh")
	if code != ExitOK || errOut != "" || !strings.Contains(out, "Register-ArgumentCompleter") {
		t.Fatalf("pwsh completion failed: code=%d stderr=%q", code, errOut)
	}
}

// TestCompletionHelp 校验 completion --help 走帮助路由而不是当成 shell 参数。
func TestCompletionHelp(t *testing.T) {
	code, out, errOut := runCLI("completion", "--help")
	if code != ExitOK || errOut != "" {
		t.Fatalf("exit code=%d stderr=%q", code, errOut)
	}
	if !strings.Contains(out, "completion") || !strings.Contains(out, "bash") {
		t.Fatalf("completion 帮助缺少用法: %q", out)
	}
}

// TestCompletionRejectsUnknownShell 校验未知 shell 返回用法错误并写入 stderr。
func TestCompletionRejectsUnknownShell(t *testing.T) {
	code, _, errOut := runCLI("completion", "tcsh")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "[ERR]") || !strings.Contains(errOut, "tcsh") {
		t.Fatalf("stderr 应包含错误与 shell 名: %q", errOut)
	}
}

// TestCompletionNoArgs 校验缺少 shell 参数时不会静默成功。
func TestCompletionNoArgs(t *testing.T) {
	code, _, errOut := runCLI("completion")
	if code != ExitUsage || !strings.Contains(errOut, "用法") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}
