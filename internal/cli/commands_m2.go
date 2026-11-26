// 本文件实现 M2 阶段的导出、同步与平台维护命令。
package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

// parseFlags 解析 --key value / --key=value 与布尔开关，返回位置参数。
func parseFlags(args []string, valueFlags, boolFlags []string) ([]string, map[string]string, map[string]bool, error) {
	values := map[string]string{}
	bools := map[string]bool{}
	valueSet := map[string]bool{}
	boolSet := map[string]bool{}
	for _, name := range valueFlags {
		valueSet[name] = true
	}
	for _, name := range boolFlags {
		boolSet[name] = true
	}
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if eq := strings.Index(name, "="); eq >= 0 {
			values[name[:eq]] = name[eq+1:]
			continue
		}
		if valueSet[name] {
			if i+1 >= len(args) {
				return nil, nil, nil, fmt.Errorf("--%s 缺少值", name)
			}
			values[name] = args[i+1]
			i++
			continue
		}
		if boolSet[name] {
			bools[name] = true
			continue
		}
		return nil, nil, nil, fmt.Errorf("未知参数: --%s", name)
	}
	return positionals, values, bools, nil
}

func atoiDefault(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("必须是正整数: %q", raw)
	}
	return n, nil
}

func buildInstallerSet(cfg *core.Config, raw string) (core.InstallerSet, error) {
	if raw == "" {
		raw = cfg.Installers
	}
	return core.ParseInstallers(raw)
}

func buildPlatforms(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return out
}

func renderWarnings(env Env, warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(env.Stderr, "[WARN] %s\n", warning)
	}
}

func renderMessages(env Env, messages []string) {
	for _, message := range messages {
		fmt.Fprintf(env.Stdout, "[OK]   %s\n", message)
	}
}

func renderInfos(env Env, infos []string) {
	for _, info := range infos {
		fmt.Fprintf(env.Stdout, "[INFO] %s\n", info)
	}
}

// auditWrite 追加审计记录；失败只告警，不影响主流程。
func auditWrite(env Env, deps *coreDeps, action, cn, serial, result string) {
	if err := core.AppendAuditWithOrigin(deps.cfg, string(env.Origin), action, cn, serial, result); err != nil {
		fmt.Fprintf(env.Stderr, "[WARN] 审计日志: %v\n", err)
	}
}

func auditOutcome(already bool) string {
	if already {
		return "already"
	}
	return "ok"
}

// newIssueCommand 构造 neca issue。
func newIssueCommand() Command {
	return Command{
		Name:    "issue",
		Summary: "签发新证书（可选平台包、p12 算法与 zip）",
		Run: func(env Env) int {
			pos, values, bools, err := parseFlags(env.Args,
				[]string{"platforms", "days", "installers", "p12-profile"},
				[]string{"zip", "no-reload"})
			if err != nil || len(pos) != 1 {
				if err == nil {
					err = fmt.Errorf("用法: issue <CN> [--platforms ...] [--days N] [--installers ...] [--p12-profile ...] [--zip]")
				}
				return usageFail(env, "%v", err)
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			return runIssue(env, deps, pos[0], values, bools, false)
		},
	}
}

// newReissueCommand 构造 neca reissue。
func newReissueCommand() Command {
	return Command{
		Name:    "reissue",
		Summary: "归档旧证书并重签",
		Run: func(env Env) int {
			pos, values, bools, err := parseFlags(env.Args,
				[]string{"platforms", "days", "installers", "p12-profile"},
				[]string{"zip", "no-reload"})
			if err != nil || len(pos) != 1 {
				if err == nil {
					err = fmt.Errorf("用法: reissue <CN> [--platforms ...] [--days N]")
				}
				return usageFail(env, "%v", err)
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			return runIssue(env, deps, pos[0], values, bools, true)
		},
	}
}

// runIssue 执行 issue/reissue 并渲染结果。
func runIssue(env Env, deps *coreDeps, cn string, values map[string]string, bools map[string]bool, reissue bool) int {
	platforms := buildPlatforms(values["platforms"])
	days, err := atoiDefault(values["days"], deps.cfg.DefaultDays)
	if err != nil {
		return usageFail(env, "--days %v", err)
	}
	installers, err := buildInstallerSet(deps.cfg, values["installers"])
	if err != nil {
		return usageFail(env, "%v", err)
	}
	profile, err := core.ParseP12Profile(values["p12-profile"])
	if err != nil {
		return usageFail(env, "%v", err)
	}
	req := core.IssueRequest{CN: cn, Days: days, Platforms: platforms, Installers: installers, P12Profile: profile, NoReload: bools["no-reload"]}
	if !env.JSON {
		fmt.Fprintf(env.Stdout, "[INFO] 生成密钥与证书 %s（%d 天）...\n", cn, days)
	}
	var result *core.CertificateResult
	if reissue {
		result, err = core.ReissueCertificate(context.Background(), deps.cfg, deps.store, deps.oss, deps.engine, req)
	} else {
		result, err = core.IssueCertificate(context.Background(), deps.cfg, deps.store, deps.oss, req)
	}
	if err != nil {
		return fail(env, err)
	}
	action := env.Action
	if action == "" {
		action = "issue"
	}
	auditWrite(env, deps, action, cn, result.Serial, "ok")
	if env.JSON {
		data := map[string]any{
			"serial": result.Serial, "platforms": result.Platforms, "subject": result.Subject,
			"summary": result.Summary, "crlPath": result.CRLPath, "archivedSerial": result.ArchivedSerial,
		}
		if bools["zip"] {
			entry, err := core.CreateDownloadZip(deps.cfg, cn, result.Platforms)
			if err != nil {
				return fail(env, err)
			}
			data["zip"] = map[string]any{"path": entry.Path, "expiresAt": entry.ExpiresAt, "sha256": entry.SHA256}
		}
		return emitJSON(env, action, cn, result.Warnings, data)
	}
	renderInfos(env, result.Infos)
	renderWarnings(env, result.Warnings)
	renderMessages(env, result.Messages)
	if result.CRLPath != "" {
		fmt.Fprintf(env.Stdout, "[OK]   CRL 已更新: %s\n", result.CRLPath)
	}
	for _, line := range result.Summary {
		fmt.Fprintln(env.Stdout, line)
	}
	platformsText := strings.Join(result.Platforms, ",")
	fmt.Fprintf(env.Stdout, "[OK]   已签发 %s（序列 %s，平台 %s）\n", cn, result.Serial, platformsText)
	if bools["zip"] {
		if err := createZip(env, deps.cfg, cn, result.Platforms); err != nil {
			return fail(env, err)
		}
	}
	return ExitOK
}

// newRevokeCommand 构造 neca revoke。
func newRevokeCommand() Command {
	return Command{
		Name:    "revoke",
		Summary: "吊销证书并更新 CRL",
		Run: func(env Env) int {
			pos, _, bools, err := parseFlags(env.Args, nil, []string{"no-reload"})
			if err != nil || len(pos) != 1 {
				return usageFail(env, "用法: revoke <CN>")
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.RevokeCertificate(context.Background(), deps.cfg, deps.store, deps.oss, deps.engine, pos[0], bools["no-reload"])
			if err != nil {
				return fail(env, err)
			}
			auditWrite(env, deps, "revoke", pos[0], result.Serial, auditOutcome(result.Already))
			if env.JSON {
				return emitJSON(env, "revoke", pos[0], result.Warnings, map[string]any{"serial": result.Serial, "already": result.Already, "crlPath": result.CRLPath})
			}
			if result.Already {
				fmt.Fprintf(env.Stdout, "[INFO] %s 已处于吊销状态\n", pos[0])
				return ExitOK
			}
			fmt.Fprintf(env.Stdout, "[OK]   CRL 已更新: %s\n", result.CRLPath)
			renderWarnings(env, result.Warnings)
			fmt.Fprintf(env.Stdout, "[OK]   已吊销 %s（序列 %s）\n", pos[0], result.Serial)
			return ExitOK
		},
	}
}

// newUnrevokeCommand 构造 neca unrevoke。
func newUnrevokeCommand() Command {
	return Command{
		Name:    "unrevoke",
		Summary: "恢复已吊销证书",
		Run: func(env Env) int {
			pos, _, bools, err := parseFlags(env.Args, nil, []string{"no-reload"})
			if err != nil || len(pos) != 1 {
				return usageFail(env, "用法: unrevoke <CN>")
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.UnrevokeCertificate(context.Background(), deps.cfg, deps.store, deps.oss, deps.engine, pos[0], bools["no-reload"])
			if err != nil {
				return fail(env, err)
			}
			auditWrite(env, deps, "unrevoke", pos[0], result.Serial, auditOutcome(result.Already))
			if env.JSON {
				return emitJSON(env, "unrevoke", pos[0], result.Warnings, map[string]any{"serial": result.Serial, "already": result.Already, "crlPath": result.CRLPath})
			}
			if result.Already {
				fmt.Fprintf(env.Stdout, "[INFO] %s 未处于吊销状态\n", pos[0])
				return ExitOK
			}
			fmt.Fprintf(env.Stdout, "[OK]   CRL 已更新: %s\n", result.CRLPath)
			renderWarnings(env, result.Warnings)
			fmt.Fprintf(env.Stdout, "[OK]   已恢复 %s（序列 %s）\n", pos[0], result.Serial)
			return ExitOK
		},
	}
}

// newDeleteCommand 构造 neca delete。
func newDeleteCommand() Command {
	return Command{
		Name:    "delete",
		Summary: "删除证书、客户端包与元数据（需 --yes）",
		Run: func(env Env) int {
			pos, _, bools, err := parseFlags(env.Args, nil, []string{"purge-revoked", "no-reload"})
			if err != nil || len(pos) != 1 {
				return usageFail(env, "用法: delete <CN> --yes [--purge-revoked]")
			}
			if !env.Yes {
				return usageFail(env, "delete 需要 --yes 确认")
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.DeleteCertificate(context.Background(), deps.cfg, deps.store, deps.oss, deps.engine, pos[0], bools["purge-revoked"], bools["no-reload"])
			if err != nil {
				return fail(env, err)
			}
			auditWrite(env, deps, "delete", pos[0], "", "ok")
			if env.JSON {
				return emitJSON(env, "delete", pos[0], result.Warnings, map[string]any{"crlPath": result.CRLPath})
			}
			if result.CRLPath != "" {
				fmt.Fprintf(env.Stdout, "[OK]   CRL 已更新: %s\n", result.CRLPath)
			}
			renderWarnings(env, result.Warnings)
			fmt.Fprintf(env.Stdout, "[OK]   已删除 %s\n", pos[0])
			return ExitOK
		},
	}
}

// newPlatformCommand 构造 neca platform add|set。
func newPlatformCommand() Command {
	return Command{
		Name:    "platform",
		Summary: "平台包与平台标注（platform add|set）",
		Run: func(env Env) int {
			if len(env.Args) < 2 {
				return usageFail(env, "用法: platform add|set <CN> --platforms ...")
			}
			action := env.Args[0]
			pos, values, bools, err := parseFlags(env.Args[1:], []string{"platforms", "installers"}, []string{"zip", "no-reload"})
			if err != nil || len(pos) != 1 {
				return usageFail(env, "用法: platform add|set <CN> --platforms ...")
			}
			if values["platforms"] == "" {
				return usageFail(env, "--platforms 不能为空")
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			platforms := buildPlatforms(values["platforms"])
			var result *core.CertificateResult
			switch action {
			case "add":
				installers, err := buildInstallerSet(deps.cfg, values["installers"])
				if err != nil {
					return usageFail(env, "%v", err)
				}
				result, err = core.AddPlatforms(context.Background(), deps.cfg, deps.store, deps.oss, pos[0], platforms, installers)
				if err != nil {
					return fail(env, err)
				}
				auditWrite(env, deps, "platform-add", pos[0], "", "ok")
				if env.JSON {
					return emitJSON(env, "platform-add", pos[0], result.Warnings, map[string]any{"platforms": result.Platforms, "messages": result.Messages})
				}
				renderMessages(env, result.Messages)
				renderWarnings(env, result.Warnings)
				fmt.Fprintf(env.Stdout, "[OK]   已为 %s 增加平台: %s\n", pos[0], strings.Join(result.Platforms, ","))
				if bools["zip"] {
					if err := createZip(env, deps.cfg, pos[0], result.Platforms); err != nil {
						return fail(env, err)
					}
				}
			case "set":
				result, err = core.SetPlatforms(deps.cfg, pos[0], platforms)
				if err != nil {
					return fail(env, err)
				}
				auditWrite(env, deps, "platform-set", pos[0], "", "ok")
				if env.JSON {
					return emitJSON(env, "platform-set", pos[0], nil, map[string]any{"platforms": result.Platforms})
				}
				fmt.Fprintf(env.Stdout, "[OK]   平台标注已更新: %s -> %s\n", pos[0], strings.Join(result.Platforms, ","))
			default:
				return usageFail(env, "未知 platform 子命令: %s", action)
			}
			return ExitOK
		},
	}
}

// newPackageCommand 构造 neca package。
func newPackageCommand() Command {
	return Command{
		Name:    "package",
		Summary: "从现有证书重建平台包（可选 zip）",
		Run: func(env Env) int {
			pos, values, bools, err := parseFlags(env.Args,
				[]string{"platforms", "installers", "p12-profile"},
				[]string{"zip", "no-reload"})
			if err != nil || len(pos) != 1 {
				return usageFail(env, "用法: package <CN> [--platforms ...] [--installers ...] [--zip]")
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			installers, err := buildInstallerSet(deps.cfg, values["installers"])
			if err != nil {
				return usageFail(env, "%v", err)
			}
			profile, err := core.ParseP12Profile(values["p12-profile"])
			if err != nil {
				return usageFail(env, "%v", err)
			}
			result, err := core.RenderPackage(context.Background(), deps.cfg, deps.oss, pos[0], buildPlatforms(values["platforms"]), installers, profile)
			if err != nil {
				return fail(env, err)
			}
			auditWrite(env, deps, "package", pos[0], "", "ok")
			if env.JSON {
				return emitJSON(env, "package", pos[0], result.Warnings, map[string]any{"platforms": result.Platforms, "messages": result.Messages})
			}
			renderMessages(env, result.Messages)
			renderWarnings(env, result.Warnings)
			fmt.Fprintf(env.Stdout, "[OK]   平台包已生成: %s\n", strings.Join(result.Platforms, ","))
			if bools["zip"] {
				if err := createZip(env, deps.cfg, pos[0], result.Platforms); err != nil {
					return fail(env, err)
				}
			}
			return ExitOK
		},
	}
}

// newDownloadsCommand 构造 neca downloads list|clean。
func newDownloadsCommand() Command {
	return Command{
		Name:    "downloads",
		Summary: "管理下载目录中的 zip（list|clean）",
		Run: func(env Env) int {
			if len(env.Args) < 1 {
				return usageFail(env, "用法: downloads list|clean [--expired|--all]")
			}
			action := env.Args[0]
			_, _, bools, err := parseFlags(env.Args[1:], nil, []string{"expired", "all"})
			if err != nil {
				return usageFail(env, "%v", err)
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			switch action {
			case "list":
				entries, err := core.ListDownloads(deps.cfg)
				if err != nil {
					return fail(env, err)
				}
				if env.JSON {
					return emitJSON(env, "downloads-list", "", nil, map[string]any{"downloads": entries})
				}
				for _, entry := range entries {
					fmt.Fprintf(env.Stdout, "[INFO] %s  %s  %d 字节  过期: %s\n", entry.CN, entry.Path, entry.Size, entry.ExpiresAt.Format(time.RFC3339))
				}
				return ExitOK
			case "clean":
				removed, err := core.CleanDownloads(deps.cfg, bools["all"])
				if err != nil {
					return fail(env, err)
				}
				if env.JSON {
					return emitJSON(env, "downloads-clean", "", nil, map[string]any{"removed": removed})
				}
				fmt.Fprintf(env.Stdout, "[OK]   已清理 %d 个下载文件\n", removed)
				return ExitOK
			default:
				return usageFail(env, "未知 downloads 子命令: %s", action)
			}
		},
	}
}

// createZip 生成下载 zip 并输出路径/有效期。
func createZip(env Env, cfg *core.Config, cn string, platforms []string) error {
	entry, err := core.CreateDownloadZip(cfg, cn, platforms)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "[OK]   zip: %s（有效期至 %s）\n", entry.Path, entry.ExpiresAt.Format(time.RFC3339))
	return nil
}
