// 本文件实现 cert import 命令。
package cli

import (
	"context"
	"fmt"
	"github.com/hyhdsl/neca/internal/core"
)

// newImportCommand 构造 cert import。
func newImportCommand() Command {
	return Command{
		Name:    "import",
		Summary: "导入由当前 CA 签发的 p12 或 PEM 证书",
		Run: func(env Env) int {
			pos, values, bools, err := parseFlags(env.Args,
				[]string{"p12", "password-file", "cert", "key", "key-password-file", "platforms", "installers", "p12-profile"},
				[]string{"allow-expired", "zip"})
			if err != nil || len(pos) != 1 {
				if err == nil {
					err = fmt.Errorf("用法: import <CN> --p12 FILE --password-file FILE | --cert FILE --key FILE [--key-password-file FILE] [--platforms ...] [--installers ...] [--p12-profile ...] [--allow-expired] [--zip]")
				}
				return usageFail(env, "%v", err)
			}
			if env.JSON && !env.Yes && !env.DryRun {
				return usageFail(env, "cert import --json 需要 --yes")
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
			req := core.ImportRequest{
				CN:              pos[0],
				P12Path:         values["p12"],
				PasswordFile:    values["password-file"],
				CertPath:        values["cert"],
				KeyPath:         values["key"],
				KeyPasswordFile: values["key-password-file"],
				Platforms:       platforms, Installers: installers, P12Profile: profile,
				AllowExpired: bools["allow-expired"], DryRun: env.DryRun,
			}
			validation, err := core.ValidateImport(context.Background(), deps.cfg, deps.store, deps.oss, req)
			if err != nil {
				return fail(env, err)
			}
			if validation.Expired && !req.AllowExpired {
				return fail(env, &core.Error{Kind: core.KindPrecondition, Op: "导入证书", Err: fmt.Errorf("证书已过期，需 --allow-expired 并二次确认")})
			}
			if env.DryRun {
				return renderImportDryRun(env, validation, req)
			}
			if validation.Expired && !env.JSON && !env.Yes {
				fmt.Fprintf(env.Stdout, "[WARN] 证书已过期：%s\n", validation.Expires)
				fmt.Fprintln(env.Stdout, "[WARN] 导入后 index 状态为 E；必须随后执行 reissue 或 revoke。")
				confirmer := TerminalConfirmer{In: env.Stdin, Out: env.Stdout, AssumeYes: false}
				ok, err := confirmer.Confirm(context.Background(), ConfirmRequest{Action: "导入已过期证书", Subject: validation.CN, Default: false})
				if err != nil {
					return fail(env, err)
				}
				if !ok {
					fmt.Fprintln(env.Stdout, "已取消")
					return ExitOK
				}
			}
			result, err := core.ImportCertificate(context.Background(), deps.cfg, deps.store, deps.oss, req)
			if err != nil {
				return fail(env, err)
			}
			outcome := "ok"
			if result.Expired {
				outcome = "ok-expired"
			}
			auditWrite(env, deps, "cert-import", result.CN, result.Serial, outcome)
			data := map[string]any{
				"serial": result.Serial, "subject": result.Subject, "source": string(result.Source),
				"created": result.Created, "notBefore": result.NotBefore, "expires": result.Expires,
				"expired": result.Expired, "platforms": result.Platforms,
				"nextActions": result.NextActions, "indexAdded": result.IndexAdded, "metaWritten": result.MetaWritten,
			}
			if bools["zip"] {
				entry, err := core.CreateDownloadZip(deps.cfg, result.CN, result.Platforms)
				if err != nil {
					return fail(env, err)
				}
				data["zip"] = map[string]any{"path": entry.Path, "expiresAt": entry.ExpiresAt, "sha256": entry.SHA256}
			}
			if env.JSON {
				return emitJSON(env, "cert import", result.CN, result.Warnings, data)
			}
			renderWarnings(env, result.Warnings)
			fmt.Fprintf(env.Stdout, "[OK]   已导入 %s（序列 %s，来源 %s，入库 %s）\n", result.CN, result.Serial, result.Source, result.Created)
			renderImportNextActions(env, result.NextActions)
			if bools["zip"] {
				if err := createZip(env, deps.cfg, result.CN, result.Platforms); err != nil {
					return fail(env, err)
				}
			}
			return ExitOK
		},
	}
}

func renderImportDryRun(env Env, validation *core.ImportValidation, req core.ImportRequest) int {
	data := map[string]any{
		"dryRun": true, "cn": validation.CN, "serial": validation.Serial,
		"subject": validation.Subject, "source": string(validation.Source),
		"notBefore": validation.NotBefore, "expires": validation.Expires,
		"expired": validation.Expired, "platforms": validation.Platforms,
		"allowExpired": req.AllowExpired,
	}
	if env.JSON {
		return emitJSON(env, "cert import", validation.CN, nil, data)
	}
	fmt.Fprintf(env.Stdout, "[INFO] dry-run: 可导入 %s（序列 %s）\n", validation.CN, validation.Serial)
	fmt.Fprintf(env.Stdout, "[INFO] 生效：%s；到期：%s\n", validation.NotBefore, validation.Expires)
	if validation.Expired {
		fmt.Fprintln(env.Stdout, "[WARN] 证书已过期；实际导入需要 --allow-expired 并确认。")
	}
	fmt.Fprintln(env.Stdout, "[INFO] dry-run 未修改任何文件")
	return ExitOK
}

func renderImportNextActions(env Env, actions []string) {
	if len(actions) == 0 {
		return
	}
	fmt.Fprintln(env.Stdout, "[WARN] 证书已过期导入，index 状态 E。")
	if len(actions) >= 2 {
		fmt.Fprintf(env.Stdout, "  续签: %s\n", actions[0])
		fmt.Fprintf(env.Stdout, "  或吊销: %s\n", actions[1])
		return
	}
	for _, action := range actions {
		fmt.Fprintf(env.Stdout, "  %s\n", action)
	}
}
