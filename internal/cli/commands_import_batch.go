// 本文件实现 cert import-batch 目录批量导入命令。
package cli

import (
	"context"
	"fmt"

	"github.com/hyhdsl/neca/internal/core"
)

// newImportBatchCommand 构造 cert import-batch。
func newImportBatchCommand() Command {
	return Command{
		Name:    "import-batch",
		Summary: "按目录批量导入 p12 或 PEM cert+key",
		Write:   true,
		Group:   "cert",
		Run: func(env Env) int {
			pos, values, bools, err := parseFlags(env.Args,
				[]string{"password-dir", "platforms", "installers", "p12-profile"},
				[]string{"allow-expired"})
			if err != nil || len(pos) != 1 {
				if err == nil {
					err = fmt.Errorf("用法: import-batch <DIR> [--password-dir DIR] [--platforms ...] [--installers ...] [--p12-profile ...] [--allow-expired] [--dry-run] [--json --yes]")
				}
				return usageFail(env, "%v", err)
			}
			if env.JSON && !env.Yes && !env.DryRun {
				return usageFail(env, "cert import-batch --json 需要 --yes")
			}
			if !env.DryRun && !env.Yes {
				return usageFail(env, "cert import-batch 实际导入需要 --yes（批量非交互确认）")
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			if !env.DryRun {
				if err := core.RecoverImportTransactions(context.Background(), deps.cfg, deps.store); err != nil {
					return fail(env, err)
				}
			}
			platforms := buildPlatforms(values["platforms"])
			installers, err := buildInstallerSet(deps.cfg, values["installers"])
			if err != nil {
				return usageFail(env, "%v", err)
			}
			profile, err := core.ParseP12Profile(values["p12-profile"])
			if err != nil {
				return usageFail(env, "%v", err)
			}
			opts := core.BatchOptions{
				Dir:         pos[0],
				PasswordDir: values["password-dir"],
				Defaults: core.BatchDefaults{
					Platforms: platforms, Installers: installers, P12Profile: profile,
					AllowExpired: bools["allow-expired"],
				},
				DryRun: env.DryRun,
			}
			result, err := core.ImportBatch(context.Background(), deps.cfg, deps.store, deps.oss, opts)
			if err != nil {
				return fail(env, err)
			}
			for _, entry := range result.Entries {
				if entry.Status == "imported" {
					outcome := "ok"
					if entry.Expired {
						outcome = "ok-expired"
					}
					auditWrite(env, deps, "cert-import", entry.CN, entry.Serial, outcome)
				}
			}
			if env.JSON {
				envl := jsonEnvelope{OK: result.Failed == 0, Action: "cert import-batch", Data: result, DurationMS: elapsedMS(env)}
				if result.Failed > 0 {
					envl.Error = &jsonError{Code: "precondition", Message: "至少一条导入失败"}
				}
				writeEnvelope(env, envl)
			} else {
				renderBatchResult(env, result)
			}
			if result.Failed > 0 {
				return ExitPrecondition
			}
			return ExitOK
		},
	}
}

func renderBatchResult(env Env, result *core.BatchImportResult) {
	for _, entry := range result.Entries {
		switch entry.Status {
		case "imported":
			fmt.Fprintf(env.Stdout, "[OK]   %s：已导入（来源 %s，序列 %s）\n", entry.CN, entry.Source, entry.Serial)
		case "validated":
			fmt.Fprintf(env.Stdout, "[INFO] %s：dry-run 可导入（来源 %s，序列 %s）\n", entry.CN, entry.Source, entry.Serial)
		default:
			fmt.Fprintf(env.Stdout, "[ERR]  %s：%s\n", entry.CN, entry.Error)
		}
	}
	if result.DryRun {
		fmt.Fprintf(env.Stdout, "[INFO] dry-run：成功 %d，失败 %d；未修改任何文件\n", result.Succeeded, result.Failed)
		return
	}
	fmt.Fprintf(env.Stdout, "[INFO] 批量导入：成功 %d，失败 %d\n", result.Succeeded, result.Failed)
}
