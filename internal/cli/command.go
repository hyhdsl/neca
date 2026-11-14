package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/hyhdsl/neca/internal/core"
)

// Env 是执行一次子命令所需的运行时环境。
//
// 命令实现不得直接读写 os.Stdout / os.Args：I/O 全部从 Env 取，
// 这样单元测试、未来的 --json 模式以及 Web 端复用都不必修改命令本身。
type Env struct {
	Stdout io.Writer // 正常输出
	Stderr io.Writer // 错误与告警
	Args   []string  // 子命令之后的参数（不含子命令名）
}

// Command 是一个可注册的 CLI 子命令。
//
// 新增子命令时只实现 Run 并注册，不需要修改 Run 函数里的分发 switch，
// 也不需要手写帮助文本——帮助由注册表自动生成。
type Command struct {
	Name    string        // 子命令名，例如 "list"
	Aliases []string      // 兼容别名，例如 {"ls"}；不得与任何命令名/别名冲突
	Summary string        // 帮助列表里的一句话说明
	Run     func(Env) int // 返回进程退出码（见 ExitOK 等常量）
}

// Registry 保存子命令表，是 CLI 的扩展点。
//
// 并发不安全：只在启动阶段构建，构建完成后只读。若未来需要插件式动态注册，
// 应在进程启动时一次性注册完毕，而不是运行期增删。
type Registry struct {
	byName map[string]*Command
	order  []*Command
}

// NewRegistry 创建一个空注册表。
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]*Command)}
}

// Register 注册一个子命令。
//
// 返回错误而不是 panic，便于未来集中注册多个命令时一次性报告冲突；
// 内置命令使用 MustRegister，让编程错误在测试阶段立即暴露。
func (r *Registry) Register(cmd Command) error {
	if cmd.Name == "" {
		return errors.New("cli: command name must not be empty")
	}
	if strings.ContainsAny(cmd.Name, " \t\r\n") {
		return fmt.Errorf("cli: command name %q must not contain whitespace", cmd.Name)
	}
	if cmd.Run == nil {
		return fmt.Errorf("cli: command %q has nil Run", cmd.Name)
	}
	if _, exists := r.byName[cmd.Name]; exists {
		return fmt.Errorf("cli: command %q already registered", cmd.Name)
	}
	for _, alias := range cmd.Aliases {
		if alias == "" {
			return fmt.Errorf("cli: command %q has empty alias", cmd.Name)
		}
		if _, exists := r.byName[alias]; exists {
			return fmt.Errorf("cli: alias %q of command %q already registered", alias, cmd.Name)
		}
	}

	copied := cmd
	r.byName[copied.Name] = &copied
	for _, alias := range copied.Aliases {
		r.byName[alias] = &copied
	}
	r.order = append(r.order, &copied)
	return nil
}

// MustRegister 注册命令，冲突时 panic；仅用于内置命令这类静态注册。
func (r *Registry) MustRegister(cmd Command) {
	if err := r.Register(cmd); err != nil {
		panic(err)
	}
}

// Lookup 按名称或别名查找命令。
func (r *Registry) Lookup(name string) (*Command, bool) {
	cmd, ok := r.byName[name]
	return cmd, ok
}

// Commands 返回按名称排序的命令列表（每个命令只出现一次，不含别名）。
// 返回值是副本，调用方修改不会影响注册表。
func (r *Registry) Commands() []*Command {
	cmds := append([]*Command(nil), r.order...)
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	return cmds
}

// WriteHelp 输出帮助文本。
//
// 命令列表由注册表生成：新增命令只需注册，帮助自动包含；
// 全局开关（--version / -V）不属于子命令，单独列在"通用开关"里。
func (r *Registry) WriteHelp(w io.Writer) {
	fmt.Fprintf(w, "%s —— strongSwan IKEv2 用户证书管理工具\n\n", progName)
	fmt.Fprintln(w, "用法：")
	fmt.Fprintf(w, "  %s <子命令> [参数]\n\n", progName)
	fmt.Fprintln(w, "可用子命令：")
	for _, cmd := range r.Commands() {
		name := cmd.Name
		if len(cmd.Aliases) > 0 {
			name += "（别名 " + strings.Join(cmd.Aliases, "、") + "）"
		}
		// 按显示宽度补齐（中文别名占 2 列），不能用 %-28s 的字节宽度。
		padding := 28 - core.DisplayWidth(name)
		if padding < 0 {
			padding = 0
		}
		fmt.Fprintf(w, "  %s%s %s\n", name, strings.Repeat(" ", padding), cmd.Summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "通用开关：")
	fmt.Fprintln(w, "  --version, -V  只输出版本号（单行）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "后续里程碑将提供：issue / reissue / revoke / unrevoke / delete 等写命令。")
}
