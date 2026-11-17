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
		return usageFail(env, "%s", i18n.T("wizard.stdin_required"))
	}
	reader := bufio.NewReader(env.Stdin)
	fmt.Fprintln(env.Stdout, i18n.T("wizard.title"))
	for {
		fmt.Fprintln(env.Stdout, "")
		fmt.Fprintln(env.Stdout, i18n.T("wizard.menu.1"))
		fmt.Fprintln(env.Stdout, i18n.T("wizard.menu.2"))
		fmt.Fprintln(env.Stdout, i18n.T("wizard.menu.3"))
		fmt.Fprintln(env.Stdout, i18n.T("wizard.menu.4"))
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
			args := []string{"issue", cn, "--platforms", "windows,android"}
			invokeWizard(env, registry, "cert", args)
		case "2":
			cn, ok := wizardCN(env, reader)
			if ok {
				invokeWizard(env, registry, "cert", []string{"revoke", cn})
			}
		case "3":
			cn, ok := wizardCN(env, reader)
			if ok {
				invokeWizard(env, registry, "cert", []string{"unrevoke", cn})
			}
		case "4":
			cn, ok := wizardCN(env, reader)
			if ok {
				invokeWizard(env, registry, "cert", []string{"reissue", cn})
			}
		case "5":
			cn, ok := wizardCN(env, reader)
			if !ok {
				continue
			}
			fmt.Fprint(env.Stdout, i18n.T("wizard.platform_prompt"))
			platforms, err := readWizardLine(reader)
			if err != nil || platforms == "" {
				continue
			}
			invokeWizard(env, registry, "cert", []string{"platform", "add", cn, "--platforms", platforms})
		case "6":
			invokeWizard(env, registry, "cert", []string{"export", "issued"})
		case "7":
			invokeWizard(env, registry, "cert", []string{"export", "revoked"})
		case "8":
			invokeWizard(env, registry, "cert", []string{"list"})
		case "9":
			invokeWizard(env, registry, "pki", []string{"index", "sync"})
		case "10":
			cn, ok := wizardCN(env, reader)
			if !ok {
				continue
			}
			fmt.Fprint(env.Stdout, i18n.T("wizard.confirm_delete", cn))
			confirm, err := readWizardLine(reader)
			if err != nil || strings.ToLower(confirm) != "yes" {
				fmt.Fprintln(env.Stdout, i18n.T("wizard.canceled"))
				continue
			}
			invokeWizard(env, registry, "cert", []string{"trash", cn})
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
		fmt.Fprintf(env.Stderr, "[ERR]  %s\n", i18n.T("wizard.unknown_command", name))
		return
	}
	commandLine := append([]string{progName, name}, args...)
	fmt.Fprintln(env.Stdout, i18n.T("wizard.equivalent", strings.Join(commandLine, " ")))
	env.Args = args
	if env.Lang == "en-US" {
		runLocalizedCommand(env, cmd, env.Stdout, env.Stderr)
		return
	}
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
