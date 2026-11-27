// 本文件实现 neca 的 shell completion 命令，覆盖顶层命令、别名、子命令与主要开关。
package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/hyhdsl/neca/internal/i18n"
)

// completionGlobalFlags 是所有命令共用的通用开关；-h/--help 也在这里完成。
var completionGlobalFlags = []string{
	"--json", "--dry-run", "--yes", "-y", "--quiet", "-q",
	"--version", "-V", "--config", "--reload-mode", "--lang", "-h", "--help",
}

// completionSubcommands 是各命名空间的直接子命令；只列出实际存在的命令。
var completionSubcommands = map[string][]string{
	"cert":       {"list", "expiring", "verify", "issue", "reissue", "revoke", "unrevoke", "trash", "export", "platform", "package", "import"},
	"pki":        {"index", "downloads"},
	"trash":      {"list", "restore", "purge"},
	"config":     {"init", "show", "validate", "export-env", "path"},
	"web":        {"status", "readonly", "idle-timeout", "export", "lock", "lockdown", "hash-password"},
	"completion": {"bash", "zsh", "fish", "powershell", "pwsh"},
}

// completionNested 是第三层子命令，例如 cert platform add。
var completionNested = map[string][]string{
	"cert platform": {"add", "set"},
	"pki index":     {"sync"},
	"pki downloads": {"list", "clean"},
	"web readonly":  {"status", "on", "off"},
	"web export":    {"on", "off"},
}

// completionFlags 只列主要开关；通用开关由 completionGlobalFlags 统一追加。
var completionFlags = map[string][]string{
	"cert":          {"--platforms", "--days", "--installers", "--p12-profile", "--zip", "--no-reload", "--out"},
	"cert expiring": {"--days"},
	"cert issue":    {"--platforms", "--days", "--installers", "--p12-profile", "--zip", "--no-reload"},
	"cert reissue":  {"--platforms", "--days", "--installers", "--p12-profile", "--zip", "--no-reload"},
	"cert revoke":   {"--no-reload"},
	"cert unrevoke": {"--no-reload"},
	"cert export":   {"--out"},
	"cert platform": {"--platforms", "--installers", "--zip", "--no-reload"},
	"cert package":  {"--platforms", "--installers", "--p12-profile", "--zip", "--no-reload"},
	"cert import":   {"--p12", "--password-file", "--platforms", "--installers", "--p12-profile", "--allow-expired", "--zip"},
	"pki downloads": {"--expired", "--all"},
	"trash purge":   {"--expired"},
	"config init":   {"--force", "--from-env"},
}

var completionFlagPaths = []string{
	"cert", "cert expiring", "cert issue", "cert reissue", "cert revoke", "cert unrevoke",
	"cert export", "cert platform", "cert package", "pki downloads", "trash purge", "config init",
}

var completionNestedPaths = []string{
	"cert platform", "pki index", "pki downloads", "web readonly", "web export",
}

type completionEntry struct {
	Name    string
	Summary string
}

// newCompletionCommand 构造 neca completion bash|zsh|fish|powershell。
func newCompletionCommand(registry *Registry) Command {
	return Command{
		Name:    "completion",
		Summary: "生成 shell 补全脚本（bash/zsh/fish/PowerShell）",
		Run: func(env Env) int {
			if len(env.Args) != 1 {
				return usageFail(env, "用法: completion <bash|zsh|fish|powershell>")
			}
			shell := strings.ToLower(strings.TrimSpace(env.Args[0]))
			if shell == "pwsh" {
				shell = "powershell"
			}
			var err error
			switch shell {
			case "bash":
				err = writeBashCompletion(env.Stdout, registry)
			case "zsh":
				err = writeZshCompletion(env.Stdout, registry)
			case "fish":
				err = writeFishCompletion(env.Stdout, registry)
			case "powershell":
				err = writePowerShellCompletion(env.Stdout, registry)
			default:
				return usageFail(env, "不支持的 shell: %s（可用 bash/zsh/fish/powershell）", env.Args[0])
			}
			if err != nil {
				return fail(env, err)
			}
			return ExitOK
		},
	}
}

// completionEntries 返回按注册表排序的顶层命令；带短横线的 help 别名由通用开关覆盖。
func completionEntries(registry *Registry) []completionEntry {
	var entries []completionEntry
	for _, cmd := range registry.Commands() {
		summary := cmd.Summary
		if cmd.SummaryKey != "" && i18n.Has(cmd.SummaryKey) {
			summary = i18n.T(cmd.SummaryKey)
		}
		entries = append(entries, completionEntry{Name: cmd.Name, Summary: summary})
		for _, alias := range cmd.Aliases {
			if !strings.HasPrefix(alias, "-") {
				entries = append(entries, completionEntry{Name: alias, Summary: summary})
			}
		}
	}
	return entries
}

func completionNames(entries []completionEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

func completionAllWords(entries []completionEntry) []string {
	words := append([]string(nil), completionNames(entries)...)
	for _, cmd := range []string{"cert", "pki", "trash", "config", "web", "completion"} {
		words = append(words, completionSubcommands[cmd]...)
	}
	for _, path := range completionNestedPaths {
		words = append(words, completionNested[path]...)
	}
	words = append(words, completionAllFlags()...)
	return uniqueStrings(words)
}

func completionAllFlags() []string {
	flags := append([]string(nil), completionGlobalFlags...)
	for _, path := range completionFlagPaths {
		flags = append(flags, completionFlags[path]...)
	}
	return uniqueSortedStrings(flags)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	out := uniqueStrings(values)
	sort.Strings(out)
	return out
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shellWords(values []string) string {
	return strings.Join(values, " ")
}

// writeBashCompletion 输出 bash 补全；按当前位置分别补顶层命令、子命令和主要开关。
func writeBashCompletion(w io.Writer, registry *Registry) error {
	entries := completionEntries(registry)
	var b strings.Builder
	b.WriteString("# bash completion for neca\n")
	b.WriteString("_neca_completion() {\n")
	b.WriteString("  local cur cmd sub candidates\n")
	b.WriteString("  COMPREPLY=()\n")
	b.WriteString("  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("  cmd=\"${COMP_WORDS[1]}\"\n")
	b.WriteString("  sub=\"${COMP_WORDS[2]}\"\n")
	fmt.Fprintf(&b, "  candidates=%s\n", shellSingleQuote(shellWords(completionAllWords(entries))))
	b.WriteString("  if [[ $COMP_CWORD -eq 1 ]]; then\n")
	fmt.Fprintf(&b, "    candidates=%s\n", shellSingleQuote(shellWords(completionNames(entries))))
	b.WriteString("  elif [[ $COMP_CWORD -eq 2 ]]; then\n")
	b.WriteString("    case \"$cmd\" in\n")
	for _, cmd := range []string{"cert", "pki", "trash", "config", "web", "completion"} {
		fmt.Fprintf(&b, "      %s) candidates=%s;;\n", cmd, shellSingleQuote(shellWords(completionSubcommands[cmd])))
	}
	b.WriteString("    esac\n")
	b.WriteString("  elif [[ $COMP_CWORD -eq 3 ]]; then\n")
	b.WriteString("    case \"$cmd $sub\" in\n")
	for _, path := range completionNestedPaths {
		fmt.Fprintf(&b, "      %s) candidates=%s;;\n", shellSingleQuote(path), shellSingleQuote(shellWords(completionNested[path])))
	}
	b.WriteString("      *) candidates=\"\";;\n")
	b.WriteString("    esac\n")
	b.WriteString("  fi\n")
	b.WriteString("  if [[ \"$cur\" == -* ]]; then\n")
	fmt.Fprintf(&b, "    candidates=%s\n", shellSingleQuote(shellWords(completionAllFlags())))
	b.WriteString("  fi\n")
	b.WriteString("  COMPREPLY=( $(compgen -W \"$candidates\" -- \"$cur\") )\n")
	b.WriteString("}\n")
	b.WriteString("complete -F _neca_completion neca\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// writeZshCompletion 输出 zsh 补全；顶层使用 _describe，其余使用 compadd。
func writeZshCompletion(w io.Writer, registry *Registry) error {
	entries := completionEntries(registry)
	var b strings.Builder
	b.WriteString("#compdef neca\n\n_neca() {\n")
	b.WriteString("  local cmd sub\n")
	b.WriteString("  cmd=\"${words[2]}\"\n")
	b.WriteString("  sub=\"${words[3]}\"\n")
	b.WriteString("  local -a candidates\n")
	fmt.Fprintf(&b, "  candidates=( %s )\n", shellWords(completionAllWords(entries)))
	b.WriteString("  if [[ \"${words[CURRENT]}\" == -* ]]; then\n")
	fmt.Fprintf(&b, "    candidates=( %s )\n", shellWords(completionAllFlags()))
	b.WriteString("  elif (( CURRENT == 2 )); then\n")
	fmt.Fprintf(&b, "    candidates=( %s )\n", shellWords(completionNames(entries)))
	b.WriteString("  elif (( CURRENT == 3 )); then\n")
	b.WriteString("    case \"$cmd\" in\n")
	for _, cmd := range []string{"cert", "pki", "trash", "config", "web", "completion"} {
		fmt.Fprintf(&b, "      %s) candidates=( %s );;\n", cmd, shellWords(completionSubcommands[cmd]))
	}
	b.WriteString("    esac\n")
	b.WriteString("  elif (( CURRENT == 4 )); then\n")
	b.WriteString("    case \"$cmd $sub\" in\n")
	for _, path := range completionNestedPaths {
		fmt.Fprintf(&b, "      %s) candidates=( %s );;\n", shellSingleQuote(path), shellWords(completionNested[path]))
	}
	b.WriteString("    esac\n")
	b.WriteString("  fi\n")
	b.WriteString("  compadd -a candidates\n")
	b.WriteString("}\n")
	b.WriteString("compdef _neca neca\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// writeFishCompletion 输出 fish 补全；用 __fish_seen_subcommand_from 判断层级。
func writeFishCompletion(w io.Writer, registry *Registry) error {
	entries := completionEntries(registry)
	var b strings.Builder
	b.WriteString("# fish completion for neca\n")
	b.WriteString("complete -c neca -f\n")
	for _, entry := range entries {
		fmt.Fprintf(&b, "complete -c neca -n '__fish_use_subcommand' -a %s -d %s\n",
			shellSingleQuote(entry.Name), shellSingleQuote(entry.Summary))
	}
	fmt.Fprintf(&b, "complete -c neca -a %s\n", shellSingleQuote(shellWords(completionAllFlags())))
	for _, cmd := range []string{"cert", "pki", "trash", "config", "web", "completion"} {
		subs := completionSubcommands[cmd]
		conditions := fmt.Sprintf("__fish_seen_subcommand_from %s; and not __fish_seen_subcommand_from %s", cmd, shellWords(subs))
		fmt.Fprintf(&b, "complete -c neca -n %s -a %s\n", shellSingleQuote(conditions), shellSingleQuote(shellWords(subs)))
	}
	for _, path := range completionNestedPaths {
		parts := strings.Fields(path)
		subs := completionNested[path]
		conditions := fmt.Sprintf("__fish_seen_subcommand_from %s; and __fish_seen_subcommand_from %s; and not __fish_seen_subcommand_from %s",
			parts[0], parts[1], shellWords(subs))
		fmt.Fprintf(&b, "complete -c neca -n %s -a %s\n", shellSingleQuote(conditions), shellSingleQuote(shellWords(subs)))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writePowerShellCompletion 输出 PowerShell 补全；按命令词数和当前词过滤候选。
func writePowerShellCompletion(w io.Writer, registry *Registry) error {
	entries := completionEntries(registry)
	var b strings.Builder
	b.WriteString("# PowerShell completion for neca\n")
	b.WriteString("Register-ArgumentCompleter -Native -CommandName 'neca' -ScriptBlock {\n")
	b.WriteString("    param($wordToComplete, $commandAst, $cursorPosition)\n")
	b.WriteString("    $elements = @($commandAst.CommandElements | ForEach-Object { $_.Extent.Text })\n")
	b.WriteString("    $text = $commandAst.ToString()\n")
	b.WriteString("    $cmd = if ($elements.Count -gt 1) { $elements[1] } else { '' }\n")
	b.WriteString("    $sub = if ($elements.Count -gt 2) { $elements[2] } else { '' }\n")
	b.WriteString("    $items = @()\n")
	b.WriteString("    if ($wordToComplete.StartsWith('-')) {\n")
	fmt.Fprintf(&b, "        $items = @(%s)\n", strings.Join(quotePowerShell(completionAllFlags()), ", "))
	b.WriteString("    } elseif ($elements.Count -le 1 -or ($elements.Count -eq 2 -and -not $text.EndsWith(' '))) {\n")
	fmt.Fprintf(&b, "        $items = @(%s)\n", strings.Join(quotePowerShell(completionNames(entries)), ", "))
	b.WriteString("    } else {\n")
	b.WriteString("        switch ($cmd) {\n")
	for _, cmd := range []string{"cert", "pki", "trash", "config", "web", "completion"} {
		fmt.Fprintf(&b, "            %s {\n", quotePowerShellOne(cmd))
		fmt.Fprintf(&b, "                if ($elements.Count -le 3) { $items = @(%s) }\n", strings.Join(quotePowerShell(completionSubcommands[cmd]), ", "))
		for _, path := range completionNestedPaths {
			parts := strings.Fields(path)
			if parts[0] != cmd {
				continue
			}
			fmt.Fprintf(&b, "                elseif ($sub -eq %s -and $elements.Count -le 4) { $items = @(%s) }\n",
				quotePowerShellOne(parts[1]), strings.Join(quotePowerShell(completionNested[path]), ", "))
		}
		b.WriteString("            }\n")
	}
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	b.WriteString("    $items | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n")
	b.WriteString("        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func quotePowerShell(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, quotePowerShellOne(value))
	}
	return out
}

func quotePowerShellOne(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
