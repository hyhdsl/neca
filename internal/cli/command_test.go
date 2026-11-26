// 本文件测试 command 相关逻辑。
package cli

import (
	"bytes"
	"strings"
	"testing"
)

// noopCommand 返回一个合法的最小命令，减少用例样板。
func noopCommand(name string) Command {
	return Command{Name: name, Summary: "测试命令", Run: func(Env) int { return ExitOK }}
}

// TestRegistryRejectsDuplicates 校验名称/别名冲突与非法定义都会报错，
// 防止启动时出现"某个命令悄悄覆盖另一个命令"。
func TestRegistryRejectsDuplicates(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(noopCommand("list")); err != nil {
		t.Fatalf("首次注册应成功：%v", err)
	}

	tests := []struct {
		name string
		cmd  Command
	}{
		{"重复名称", noopCommand("list")},
		{"别名与已有名称冲突", Command{Name: "ls", Aliases: []string{"list"}, Run: func(Env) int { return ExitOK }}},
		{"名称为空", Command{Run: func(Env) int { return ExitOK }}},
		{"名称含空格", Command{Name: "bad name", Run: func(Env) int { return ExitOK }}},
		{"缺少 Run", Command{Name: "nilrun"}},
		{"空别名", Command{Name: "empty-alias", Aliases: []string{""}, Run: func(Env) int { return ExitOK }}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := registry.Register(tc.cmd); err == nil {
				t.Fatalf("Register(%+v) 应当报错", tc.cmd)
			}
		})
	}

	// 与已有别名冲突的用例需要先注册一个带别名的命令。
	registry.MustRegister(Command{Name: "show", Aliases: []string{"ls"}, Run: func(Env) int { return ExitOK }})
	if err := registry.Register(Command{Name: "other", Aliases: []string{"ls"}, Run: func(Env) int { return ExitOK }}); err == nil {
		t.Fatal("别名与已有别名冲突应报错")
	}
}

// TestRegistryLookupAlias 校验按名称与别名都能查到同一个命令。
func TestRegistryLookupAlias(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(Command{Name: "list", Aliases: []string{"ls"}, Summary: "列出证书", Run: func(Env) int { return ExitOK }})

	byName, ok := registry.Lookup("list")
	if !ok {
		t.Fatal("Lookup(list) 未命中")
	}
	byAlias, ok := registry.Lookup("ls")
	if !ok {
		t.Fatal("Lookup(ls) 未命中")
	}
	if byName != byAlias {
		t.Fatal("名称与别名应指向同一个 Command")
	}
	if _, ok := registry.Lookup("nope"); ok {
		t.Fatal("Lookup(nope) 不应命中")
	}
}

// TestRegistryCommandsSorted 校验 Commands 按名称排序且返回副本。
func TestRegistryCommandsSorted(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(noopCommand("verify"))
	registry.MustRegister(noopCommand("list"))

	cmds := registry.Commands()
	if len(cmds) != 2 || cmds[0].Name != "list" || cmds[1].Name != "verify" {
		t.Fatalf("Commands() = %+v, want [list verify]", cmds)
	}

	// 修改返回的切片不应影响注册表。
	cmds[0] = nil
	if again := registry.Commands(); again[0] == nil {
		t.Fatal("Commands() 必须返回副本")
	}
}

// TestMustRegisterPanics 校验内置命令注册冲突时立即 panic，
// 让错误在开发/测试阶段暴露，而不是运行时静默失效。
func TestMustRegisterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister 冲突时应 panic")
		}
	}()
	registry := NewRegistry()
	registry.MustRegister(noopCommand("dup"))
	registry.MustRegister(noopCommand("dup"))
}

// TestWriteHelpIncludesRegisteredCommands 校验帮助文本由注册表生成，
// 新增命令无需修改帮助代码。
func TestWriteHelpIncludesRegisteredCommands(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(Command{Name: "list", Aliases: []string{"ls"}, Summary: "列出证书", Run: func(Env) int { return ExitOK }})
	registry.MustRegister(Command{Name: "issue", Summary: "签发证书", Run: func(Env) int { return ExitOK }})

	var buf bytes.Buffer
	registry.WriteHelp(&buf)
	help := buf.String()
	for _, want := range []string{"用法", "list", "ls", "列出证书", "issue", "签发证书", "--version"} {
		if !strings.Contains(help, want) {
			t.Fatalf("帮助文本缺少 %q：\n%s", want, help)
		}
	}
}

// TestCommandReceivesEnv 校验命令从 Env 取 I/O 与参数，而不是读全局状态。
func TestCommandReceivesEnv(t *testing.T) {
	var got Env
	registry := NewRegistry()
	registry.MustRegister(Command{Name: "echo", Run: func(env Env) int {
		got = env
		return ExitUsage
	}})

	cmd, ok := registry.Lookup("echo")
	if !ok {
		t.Fatal("Lookup(echo) 未命中")
	}
	var out bytes.Buffer
	code := cmd.Run(Env{Stdout: &out, Args: []string{"a", "b"}})
	if code != ExitUsage {
		t.Fatalf("Run() 退出码 = %d, want %d", code, ExitUsage)
	}
	if len(got.Args) != 2 || got.Args[0] != "a" || got.Args[1] != "b" {
		t.Fatalf("命令收到的 Args = %v", got.Args)
	}
	if got.Stdout != &out {
		t.Fatal("命令应使用注入的 Stdout")
	}
}
