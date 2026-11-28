// 本文件提供交互式证书管理向导。向导按“证书管理 / 导入导出 / 回收站 / 维护诊断”分类，
// 每个动作执行前打印等价的 neca 命令，方便用户从向导过渡到脚本化操作。
package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/hyhdsl/neca/internal/i18n"
	"github.com/hyhdsl/neca/internal/version"
)

// runWizard 是无子命令时的默认交互入口。
func runWizard(env Env, registry *Registry) int {
	if env.Stdin == nil {
		return usageFail(env, "%s", i18n.T("wizard.stdin_required"))
	}
	reader := bufio.NewReader(env.Stdin)
	wizardHeader(env)
	for {
		fmt.Fprintln(env.Stdout)
		wizardMenu(env,
			"wizard.menu.main.1",
			"wizard.menu.main.2",
			"wizard.menu.main.3",
			"wizard.menu.main.4",
			"wizard.menu.main.5",
			"wizard.menu.exit",
		)
		choice, err := wizardChoice(env, reader)
		if err != nil {
			return ExitOK
		}
		switch choice {
		case "0":
			fmt.Fprintln(env.Stdout, i18n.T("wizard.bye"))
			return ExitOK
		case "1":
			runWizardCertMenu(env, reader, registry)
		case "2":
			runWizardImportExportMenu(env, reader, registry)
		case "3":
			runWizardTrashMenu(env, reader, registry)
		case "4":
			runWizardMaintenanceMenu(env, reader, registry)
		case "5":
			invokeWizard(env, registry, "help", nil)
		default:
			fmt.Fprintln(env.Stdout, i18n.T("wizard.invalid"))
		}
	}
}

func wizardHeader(env Env) {
	info := version.Get()
	fmt.Fprintln(env.Stdout, i18n.T("wizard.title"))
	fmt.Fprintln(env.Stdout, i18n.T("wizard.version", info.Version, info.Commit))
	fmt.Fprintln(env.Stdout, i18n.T("wizard.help_hint"))
}

func wizardMenu(env Env, keys ...string) {
	for _, key := range keys {
		fmt.Fprintln(env.Stdout, i18n.T(key))
	}
}

func wizardChoice(env Env, reader *bufio.Reader) (string, error) {
	fmt.Fprint(env.Stdout, i18n.T("wizard.prompt"))
	return readWizardLine(reader)
}

func runWizardCertMenu(env Env, reader *bufio.Reader, registry *Registry) {
	for {
		fmt.Fprintln(env.Stdout)
		wizardMenu(env,
			"wizard.menu.cert.1", "wizard.menu.cert.2", "wizard.menu.cert.3",
			"wizard.menu.cert.4", "wizard.menu.cert.5", "wizard.menu.cert.6",
			"wizard.menu.cert.7", "wizard.menu.cert.8", "wizard.menu.cert.9",
			"wizard.menu.cert.10", "wizard.menu.back",
		)
		choice, err := wizardChoice(env, reader)
		if err != nil {
			return
		}
		switch choice {
		case "0":
			return
		case "1":
			if cn, ok := wizardCN(env, reader); ok {
				invokeWizard(env, registry, "cert", []string{"issue", cn, "--platforms", "windows,android"})
			}
		case "2":
			invokeWizard(env, registry, "cert", []string{"list"})
		case "3":
			invokeWizard(env, registry, "cert", []string{"expiring"})
		case "4":
			invokeWizard(env, registry, "cert", []string{"verify"})
		case "5":
			if cn, ok := wizardCN(env, reader); ok {
				invokeWizard(env, registry, "cert", []string{"revoke", cn})
			}
		case "6":
			if cn, ok := wizardCN(env, reader); ok {
				invokeWizard(env, registry, "cert", []string{"unrevoke", cn})
			}
		case "7":
			if cn, ok := wizardCN(env, reader); ok {
				invokeWizard(env, registry, "cert", []string{"reissue", cn})
			}
		case "8":
			if cn, ok := wizardCN(env, reader); ok {
				if platforms, ok := wizardValue(env, reader, "wizard.platform_prompt"); ok {
					invokeWizard(env, registry, "cert", []string{"platform", "add", cn, "--platforms", platforms})
				}
			}
		case "9":
			if cn, ok := wizardCN(env, reader); ok {
				invokeWizard(env, registry, "cert", []string{"package", cn, "--platforms", "windows,android"})
			}
		case "10":
			if cn, ok := wizardCN(env, reader); ok {
				invokeWizard(env, registry, "cert", []string{"trash", cn})
			}
		default:
			fmt.Fprintln(env.Stdout, i18n.T("wizard.invalid"))
		}
	}
}

func runWizardImportExportMenu(env Env, reader *bufio.Reader, registry *Registry) {
	for {
		fmt.Fprintln(env.Stdout)
		wizardMenu(env,
			"wizard.menu.import.1",
			"wizard.menu.import.2",
			"wizard.menu.import.3",
			"wizard.menu.import.4",
			"wizard.menu.back",
		)
		choice, err := wizardChoice(env, reader)
		if err != nil {
			return
		}
		switch choice {
		case "0":
			return
		case "1":
			wizardImportP12(env, reader, registry)
		case "2":
			wizardImportCertKey(env, reader, registry)
		case "3":
			invokeWizard(env, registry, "cert", []string{"export", "issued"})
		case "4":
			invokeWizard(env, registry, "cert", []string{"export", "revoked"})
		default:
			fmt.Fprintln(env.Stdout, i18n.T("wizard.invalid"))
		}
	}
}

func wizardImportP12(env Env, reader *bufio.Reader, registry *Registry) {
	cn, ok := wizardCN(env, reader)
	if !ok {
		return
	}
	p12, ok := wizardValue(env, reader, "wizard.import.p12")
	if !ok {
		return
	}
	passwordFile, ok := wizardValue(env, reader, "wizard.import.password")
	if !ok {
		return
	}
	platforms, ok := wizardValueDefault(env, reader, "wizard.import.platforms", "windows,android")
	if !ok {
		return
	}
	invokeWizard(env, registry, "cert", []string{
		"import", cn, "--p12", p12, "--password-file", passwordFile, "--platforms", platforms,
	})
}

func wizardImportCertKey(env Env, reader *bufio.Reader, registry *Registry) {
	cn, ok := wizardCN(env, reader)
	if !ok {
		return
	}
	cert, ok := wizardValue(env, reader, "wizard.import.cert")
	if !ok {
		return
	}
	key, ok := wizardValue(env, reader, "wizard.import.key")
	if !ok {
		return
	}
	keyPassword, ok := wizardValueOptional(env, reader, "wizard.import.key_password")
	if !ok {
		return
	}
	platforms, ok := wizardValueDefault(env, reader, "wizard.import.platforms", "windows,android")
	if !ok {
		return
	}
	args := []string{"import", cn, "--cert", cert, "--key", key}
	if keyPassword != "" {
		args = append(args, "--key-password-file", keyPassword)
	}
	args = append(args, "--platforms", platforms)
	invokeWizard(env, registry, "cert", args)
}

func runWizardTrashMenu(env Env, reader *bufio.Reader, registry *Registry) {
	for {
		fmt.Fprintln(env.Stdout)
		wizardMenu(env,
			"wizard.menu.trash.1", "wizard.menu.trash.2",
			"wizard.menu.trash.3", "wizard.menu.trash.4", "wizard.menu.back",
		)
		choice, err := wizardChoice(env, reader)
		if err != nil {
			return
		}
		switch choice {
		case "0":
			return
		case "1":
			invokeWizard(env, registry, "trash", []string{"list"})
		case "2":
			if id, ok := wizardValue(env, reader, "wizard.trash.id"); ok {
				invokeWizard(env, registry, "trash", []string{"restore", id})
			}
		case "3":
			if id, ok := wizardValue(env, reader, "wizard.trash.id"); ok {
				if wizardConfirm(env, reader, "wizard.confirm_purge", id) {
					invokeWizard(env, registry, "trash", []string{"purge", id, "--yes"})
				}
			}
		case "4":
			if wizardConfirm(env, reader, "wizard.confirm_purge_expired", "") {
				invokeWizard(env, registry, "trash", []string{"purge", "--expired", "--yes"})
			}
		default:
			fmt.Fprintln(env.Stdout, i18n.T("wizard.invalid"))
		}
	}
}

func runWizardMaintenanceMenu(env Env, reader *bufio.Reader, registry *Registry) {
	for {
		fmt.Fprintln(env.Stdout)
		wizardMenu(env,
			"wizard.menu.maintenance.1", "wizard.menu.maintenance.2",
			"wizard.menu.maintenance.3", "wizard.menu.maintenance.4",
			"wizard.menu.maintenance.5", "wizard.menu.back",
		)
		choice, err := wizardChoice(env, reader)
		if err != nil {
			return
		}
		switch choice {
		case "0":
			return
		case "1":
			invokeWizard(env, registry, "pki", []string{"index", "sync"})
		case "2":
			invokeWizard(env, registry, "pki", []string{"downloads", "list"})
		case "3":
			if wizardConfirm(env, reader, "wizard.confirm_downloads_clean", "") {
				invokeWizard(env, registry, "pki", []string{"downloads", "clean", "--all", "--yes"})
			}
		case "4":
			invokeWizard(env, registry, "doctor", nil)
		case "5":
			invokeWizard(env, registry, "web", []string{"status"})
		default:
			fmt.Fprintln(env.Stdout, i18n.T("wizard.invalid"))
		}
	}
}

func wizardCN(env Env, reader *bufio.Reader) (string, bool) {
	return wizardValue(env, reader, "wizard.cn")
}

func wizardValue(env Env, reader *bufio.Reader, key string) (string, bool) {
	fmt.Fprint(env.Stdout, i18n.T(key))
	value, err := readWizardLine(reader)
	if err != nil || value == "" {
		return "", false
	}
	return value, true
}

func wizardValueOptional(env Env, reader *bufio.Reader, key string) (string, bool) {
	fmt.Fprint(env.Stdout, i18n.T(key))
	value, err := readWizardLine(reader)
	if err != nil {
		return "", false
	}
	return value, true
}

func wizardValueDefault(env Env, reader *bufio.Reader, key, fallback string) (string, bool) {
	fmt.Fprint(env.Stdout, i18n.T(key, fallback))
	value, err := readWizardLine(reader)
	if err != nil {
		return "", false
	}
	if value == "" {
		return fallback, true
	}
	return value, true
}

func wizardConfirm(env Env, reader *bufio.Reader, key, subject string) bool {
	if subject == "" {
		fmt.Fprint(env.Stdout, i18n.T(key))
	} else {
		fmt.Fprint(env.Stdout, i18n.T(key, subject))
	}
	answer, err := readWizardLine(reader)
	if err != nil {
		return false
	}
	answer = strings.ToLower(answer)
	if answer != "yes" && answer != "y" && answer != "是" {
		fmt.Fprintln(env.Stdout, i18n.T("wizard.canceled"))
		return false
	}
	return true
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
		Summary: "进入交互向导（证书管理 / 导入导出 / 回收站 / 维护）",
		Run: func(env Env) int {
			return runWizard(env, registry)
		},
	}
}
