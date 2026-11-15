package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/hyhdsl/neca/internal/i18n"
)

// runWizard 实现旧版菜单 0–10；每个动作执行前打印等价 CLI 命令。
func runWizard(env Env, registry *Registry) int {
	if env.Stdin == nil {
		return usageFail(env, "向导需要 stdin")
	}
	reader := bufio.NewReader(env.Stdin)
	fmt.Fprintln(env.Stdout, i18n.T("wizard.title"))
	for {
		fmt.Fprintln(env.Stdout, "")
		fmt.Fprintln(env.Stdout, " 1) 签发证书        2) 吊销证书       3) 恢复吊销")
		fmt.Fprintln(env.Stdout, " 4) 重签证书        5) 增加平台       6) 导出已签发")
		fmt.Fprintln(env.Stdout, " 7) 导出已吊销      8) 列表           9) 索引同步")
		fmt.Fprintln(env.Stdout, "10) 删除证书        0) 退出")
		fmt.Fprint(env.Stdout, i18n.T("wizard.prompt"))

		choice, err := readWizardLine(reader)
		if err != nil {
			return ExitOK
		}
		switch choice {
		case "0":
			fmt.Fprintln(env.Stdout, i18n.T("wizard.bye"))
			return ExitOK
		case "1":
			cn, ok := wizardCN(env, reader)
			if !ok {
				continue
			}
			args := []string{cn, "--platforms", "windows,android"}
			invokeWizard(env, registry, "issue", args)
		case "2":
			cn, ok := wizardCN(env, reader)
			if ok {
				invokeWizard(env, registry, "revoke", []string{cn})
			}
		case "3":
			cn, ok := wizardCN(env, reader)
			if ok {
				invokeWizard(env, registry, "unrevoke", []string{cn})
			}
		case "4":
			cn, ok := wizardCN(env, reader)
			if ok {
				invokeWizard(env, registry, "reissue", []string{cn})
			}
		case "5":
			cn, ok := wizardCN(env, reader)
			if !ok {
				continue
			}
			fmt.Fprint(env.Stdout, "平台（逗号分隔，如 ios）：")
			platforms, err := readWizardLine(reader)
			if err != nil || platforms == "" {
				continue
			}
			invokeWizard(env, registry, "platform", []string{"add", cn, "--platforms", platforms})
		case "6":
			invokeWizard(env, registry, "export", []string{"issued"})
		case "7":
			invokeWizard(env, registry, "export", []string{"revoked"})
		case "8":
			invokeWizard(env, registry, "list", nil)
		case "9":
			invokeWizard(env, registry, "index", []string{"sync"})
		case "10":
			cn, ok := wizardCN(env, reader)
			if !ok {
				continue
			}
			fmt.Fprint(env.Stdout, "确认删除 "+cn+"？输入 yes：")
			confirm, err := readWizardLine(reader)
			if err != nil || strings.ToLower(confirm) != "yes" {
				fmt.Fprintln(env.Stdout, "已取消")
				continue
			}
			invokeWizard(env, registry, "delete", []string{cn, "--yes"})
		default:
			fmt.Fprintln(env.Stdout, i18n.T("wizard.invalid"))
		}
	}
}

func wizardCN(env Env, reader *bufio.Reader) (string, bool) {
	fmt.Fprint(env.Stdout, i18n.T("wizard.cn"))
	cn, err := readWizardLine(reader)
	if err != nil || cn == "" {
		return "", false
	}
	return cn, true
}

func readWizardLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), err
}

func invokeWizard(env Env, registry *Registry, name string, args []string) {
	cmd, ok := registry.Lookup(name)
	if !ok {
		fmt.Fprintf(env.Stderr, "[ERR]  未知命令: %s\n", name)
		return
	}
	commandLine := append([]string{progName, name}, args...)
	fmt.Fprintln(env.Stdout, i18n.T("wizard.equivalent", strings.Join(commandLine, " ")))
	env.Args = args
	cmd.Run(env)
}

// newWizardCommand 注册显式 wizard 子命令。
func newWizardCommand(registry *Registry) Command {
	return Command{
		Name:    "wizard",
		Summary: "进入交互向导（菜单 0–10）",
		Run: func(env Env) int {
			return runWizard(env, registry)
		},
	}
}
