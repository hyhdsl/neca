package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/helper"
)

// newWebCommand 构造 neca web 管理命令。
//
// lite 镜像只带管理命令，不包含 necad；Web 服务由 neca-web 镜像启动。
// readonly off 需要 root helper 可用，避免出现“配置已可写、实际没有
// 写边界”的半状态。
func newWebCommand() Command {
	return Command{
		Name:    "web",
		Summary: "管理 Web 面板（status/readonly/idle-timeout/hash-password）",
		Run: func(env Env) int {
			if len(env.Args) == 0 || env.Args[0] == "status" {
				if len(env.Args) > 1 {
					return usageFail(env, "web status 不接受额外参数")
				}
				return runWebStatus(env)
			}
			switch env.Args[0] {
			case "readonly":
				return runWebReadOnly(env, env.Args[1:])
			case "idle-timeout":
				return runWebIdleTimeout(env, env.Args[1:])
			case "hash-password":
				if len(env.Args) > 1 {
					return usageFail(env, "web hash-password 不接受参数，密码从 stdin 读取")
				}
				return runWebHashPassword(env)
			default:
				return usageFail(env, "未知 web 子命令: %s（可用 status/readonly/idle-timeout/hash-password）", env.Args[0])
			}
		},
	}
}

func runWebStatus(env Env) int {
	cfg, err := core.LoadConfig()
	if err != nil {
		return fail(env, err)
	}
	idle, idleErr := cfg.Web.IdleTimeout()
	validationErr := cfg.Web.Validate()
	helperStatus, helperErr := probeHelper(context.Background())
	helperText := helperSummary(helperStatus, helperErr)
	if env.JSON {
		data := map[string]any{
			"enabled":       cfg.Web.Enabled,
			"listen":        cfg.Web.Listen,
			"readOnly":      cfg.Web.ReadOnly,
			"writable":      !cfg.Web.ReadOnly && helperErr == nil && helperStatus != nil && !helperStatus.ReadOnly,
			"idleTimeout":   cfg.Web.IdleTimeoutRaw,
			"absoluteLimit": core.AbsoluteWebSessionTimeout.String(),
			"adminCNs":      cfg.Web.AdminCNs,
			"passkey": map[string]any{
				"enabled": cfg.Web.Passkey.Enabled,
				"rpId":    cfg.Web.Passkey.RPID,
				"rpName":  cfg.Web.Passkey.RPName,
			},
			"helper": map[string]any{
				"socket": helperSocketPath(), "available": helperErr == nil,
				"readOnly": helperStatus != nil && helperStatus.ReadOnly,
				"version": func() string {
					if helperStatus != nil {
						return helperStatus.Version
					}
					return ""
				}(),
				"error": func() string {
					if helperErr != nil {
						return helperErr.Error()
					}
					return ""
				}(),
			},
		}
		if idleErr == nil {
			data["idleTimeoutEffective"] = idle.String()
		}
		if validationErr != nil {
			data["validationError"] = validationErr.Error()
		}
		return emitJSON(env, "web-status", "", nil, data)
	}

	fmt.Fprintf(env.Stdout, "[INFO] 配置文件: %s\n", cfg.ConfigFile)
	if cfg.Web.Enabled {
		fmt.Fprintln(env.Stdout, "[INFO] Web: 已启用")
	} else {
		fmt.Fprintln(env.Stdout, "[INFO] Web: 未启用（lite 默认；使用 neca-web 镜像并启用 web.enabled）")
	}
	fmt.Fprintf(env.Stdout, "[INFO] 监听: %s\n", cfg.Web.Listen)
	mode := "只读"
	if !cfg.Web.ReadOnly {
		if helperErr == nil && helperStatus != nil && !helperStatus.ReadOnly {
			mode = "可写"
		} else {
			mode = "配置可写（helper 不可用）"
		}
	}
	fmt.Fprintf(env.Stdout, "[INFO] 模式: %s\n", mode)
	if idleErr == nil {
		fmt.Fprintf(env.Stdout, "[INFO] 空闲超时: %s（绝对上限 %s）\n", idle, core.AbsoluteWebSessionTimeout)
	} else {
		fmt.Fprintf(env.Stdout, "[WARN] 空闲超时配置无效: %v\n", idleErr)
	}
	if validationErr != nil {
		fmt.Fprintf(env.Stdout, "[WARN] Web 配置无效: %v\n", validationErr)
	}
	if cfg.Web.Passkey.Enabled {
		fmt.Fprintf(env.Stdout, "[INFO] Passkey: 已启用（当前仅保留接口，不注册路由）rpId=%s\n", cfg.Web.Passkey.RPID)
	} else {
		fmt.Fprintln(env.Stdout, "[INFO] Passkey: 关闭（当前仅保留接口）")
	}
	fmt.Fprintf(env.Stdout, "[INFO] helper: %s\n", helperText)
	return ExitOK
}

func runWebReadOnly(env Env, args []string) int {
	if len(args) == 0 || args[0] == "status" {
		if len(args) > 1 {
			return usageFail(env, "web readonly status 不接受额外参数")
		}
		cfg, err := core.LoadConfig()
		if err != nil {
			return fail(env, err)
		}
		mode := "readonly"
		if !cfg.Web.ReadOnly {
			mode = "readwrite"
		}
		if env.JSON {
			return emitJSON(env, "web-readonly-status", "", nil, map[string]any{
				"readOnly": cfg.Web.ReadOnly, "mode": mode,
			})
		}
		fmt.Fprintf(env.Stdout, "[INFO] 当前模式: %s\n", mode)
		return ExitOK
	}
	if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
		return usageFail(env, "用法: web readonly status|on|off")
	}
	if err := requireRoot(); err != nil {
		return fail(env, err)
	}
	cfg, err := core.LoadConfig()
	if err != nil {
		return fail(env, err)
	}
	if args[0] == "off" {
		return runWebReadWrite(env, cfg)
	}
	updated, err := core.UpdateWebReadOnly(cfg.ConfigFile, true)
	if err != nil {
		return fail(env, err)
	}
	appendWebAudit(cfg, "web-readonly", "readwrite", "readonly")
	writeWebReloadHint(env, cfg.ConfigFile)
	if env.JSON {
		return emitJSON(env, "web-readonly-on", "", nil, map[string]any{
			"readOnly": updated.ReadOnly, "listen": updated.Listen, "idleTimeout": updated.IdleTimeoutRaw,
		})
	}
	fmt.Fprintln(env.Stdout, "[OK]   Web 已切换为只读")
	return ExitOK
}

func runWebIdleTimeout(env Env, args []string) int {
	cfg, err := core.LoadConfig()
	if err != nil {
		return fail(env, err)
	}
	if len(args) == 0 || args[0] == "status" {
		if len(args) > 1 {
			return usageFail(env, "web idle-timeout status 不接受额外参数")
		}
		idle, parseErr := cfg.Web.IdleTimeout()
		if parseErr != nil {
			if env.JSON {
				return emitJSON(env, "web-idle-timeout-status", "", nil, map[string]any{
					"raw": cfg.Web.IdleTimeoutRaw, "error": parseErr.Error(),
				})
			}
			return fail(env, usageError(parseErr.Error()))
		}
		if env.JSON {
			return emitJSON(env, "web-idle-timeout-status", "", nil, map[string]any{
				"raw": cfg.Web.IdleTimeoutRaw, "effective": idle.String(),
				"min": core.MinWebIdleTimeout.String(), "max": core.MaxWebIdleTimeout.String(),
			})
		}
		fmt.Fprintf(env.Stdout, "[INFO] 空闲超时: %s（允许 %s–%s）\n", idle, core.MinWebIdleTimeout, core.MaxWebIdleTimeout)
		return ExitOK
	}
	if len(args) != 1 {
		return usageFail(env, "用法: web idle-timeout status|<duration>")
	}
	if err := requireRoot(); err != nil {
		return fail(env, err)
	}
	old := cfg.Web.IdleTimeoutRaw
	updated, err := core.UpdateWebIdleTimeout(cfg.ConfigFile, args[0])
	if err != nil {
		return fail(env, err)
	}
	appendWebAudit(cfg, "web-idle-timeout", old, updated.IdleTimeoutRaw)
	writeWebReloadHint(env, cfg.ConfigFile)
	if env.JSON {
		return emitJSON(env, "web-idle-timeout", "", nil, map[string]any{
			"idleTimeout": updated.IdleTimeoutRaw,
		})
	}
	fmt.Fprintf(env.Stdout, "[OK]   空闲超时已更新: %s\n", updated.IdleTimeoutRaw)
	return ExitOK
}

// runWebHashPassword 从 stdin 读取密码并输出 bcrypt 哈希。
//
// 不读取配置文件、不要求 root；便于在服务器上生成 NECA_WEB_PASSWORD_HASH。
// 不把密码写入 argv、历史记录或日志。
func runWebHashPassword(env Env) int {
	data, err := io.ReadAll(io.LimitReader(env.Stdin, 64*1024))
	if err != nil {
		return fail(env, err)
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if len(password) < 8 {
		return usageFail(env, "密码至少需要 8 个字符")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprintln(env.Stdout, string(hash))
	return ExitOK
}

// requireRoot 返回带前置条件类别的错误，便于 JSON 输出与退出码 3。
func requireRoot() error {
	if os.Geteuid() == 0 {
		return nil
	}
	return &core.Error{Kind: core.KindPrecondition, Op: "权限检查", Err: fmt.Errorf("此操作只允许 root 执行")}
}

func helperSocketPath() string {
	if path := strings.TrimSpace(os.Getenv("NECA_WEB_HELPER_SOCKET")); path != "" {
		return path
	}
	return helper.DefaultSocketPath
}

func probeHelper(ctx context.Context) (*helper.Status, error) {
	status, err := helper.NewClient(helperSocketPath()).Status(ctx)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func helperSummary(status *helper.Status, err error) string {
	if err != nil {
		return "不可用: " + err.Error()
	}
	if status == nil {
		return "不可用"
	}
	mode := "只读"
	if !status.ReadOnly {
		mode = "可写"
	}
	return fmt.Sprintf("可用（%s，%s）", status.Version, mode)
}

func runWebReadWrite(env Env, cfg *core.Config) int {
	path := helperSocketPath()
	status, err := helper.NewClient(path).Status(context.Background())
	if err != nil {
		return fail(env, helperPrecondition("root helper 不可用: %v", err))
	}
	info, err := os.Stat(path)
	if err != nil {
		return fail(env, helperPrecondition("helper socket 不可读: %v", err))
	}
	if info.Mode().Perm()&0o060 != 0o060 {
		return fail(env, helperPrecondition("helper socket 权限必须允许 necad 组读写，实际为 %s", info.Mode().Perm()))
	}
	if _, err := os.Stat(cfg.CAKey); err != nil {
		return fail(env, helperPrecondition("找不到 CA 私钥，拒绝关闭只读: %s", cfg.CAKey))
	}
	if !status.OK || status.Protocol != helper.ProtocolVersion {
		return fail(env, helperPrecondition("helper 协议不匹配: %q", status.Protocol))
	}
	updated, err := core.UpdateWebReadOnly(cfg.ConfigFile, false)
	if err != nil {
		return fail(env, err)
	}
	appendWebAudit(cfg, "web-readonly", "readonly", "readwrite")
	writeWebReloadHint(env, cfg.ConfigFile)
	if env.JSON {
		return emitJSON(env, "web-readonly-off", "", nil, map[string]any{
			"readOnly": updated.ReadOnly, "helper": map[string]any{"available": true, "version": status.Version},
		})
	}
	fmt.Fprintln(env.Stdout, "[OK]   Web 已切换为可写（root helper 已就绪）")
	return ExitOK
}

func helperPrecondition(format string, args ...any) error {
	return &core.Error{Kind: core.KindPrecondition, Op: "Web 写模式", Err: fmt.Errorf(format, args...)}
}

func appendWebAudit(cfg *core.Config, action, oldValue, newValue string) {
	_ = core.AppendAuditWithOrigin(cfg, "web", action, "old="+oldValue+" new="+newValue, "", "ok")
}

func writeWebReloadHint(env Env, path string) {
	if !env.Quiet {
		fmt.Fprintf(env.Stderr, "[INFO] 配置已写入 %s；发送 SIGHUP 使运行中的 necad 重载（docker kill --signal=HUP neca-web）\n", path)
	}
}

// usageError 让配置层错误仍按用法错误返回退出码 1。
func usageError(message string) error {
	return &core.Error{Kind: core.KindUsage, Op: "Web 配置", Err: fmt.Errorf("%s", message)}
}
