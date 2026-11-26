// 本文件注册 cert、config、web、platform 等命令命名空间。
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

// newCertCommand is the v0.5 certificate namespace. It reuses the existing
// command implementations so flags and output stay stable during M9.4.
func newCertCommand() Command {
	return Command{
		Name:    "cert",
		Summary: "证书管理（list/expiring/verify/issue/reissue/revoke/unrevoke/trash/export/platform/package）",
		Group:   "cert",
		Surface: SurfaceBackend,
		Write:   true,
		Run: func(env Env) int {
			if len(env.Args) == 0 {
				return usageFail(env, "用法: cert <list|expiring|verify|issue|reissue|revoke|unrevoke|trash|export|platform|package>")
			}
			if env.Args[0] == "--help" || env.Args[0] == "-h" {
				return printUsage(env, "cert.help")
			}
			sub := env.Args[0]
			env.Args = env.Args[1:]
			env.Action = "cert " + sub
			switch sub {
			case "list":
				return newListCommand().Run(env)
			case "expiring":
				return newExpiringCommand().Run(env)
			case "verify":
				return newVerifyCommand().Run(env)
			case "issue":
				return newIssueCommand().Run(env)
			case "reissue":
				return newReissueCommand().Run(env)
			case "revoke":
				return newRevokeCommand().Run(env)
			case "unrevoke":
				return newUnrevokeCommand().Run(env)
			case "trash":
				return newCertTrashCommand().Run(env)
			case "export":
				return newExportCommand().Run(env)
			case "platform":
				return newPlatformCommand().Run(env)
			case "package":
				return newPackageCommand().Run(env)
			default:
				return usageFail(env, "未知 cert 子命令: %s", sub)
			}
		},
	}
}

func newPkiCommand() Command {
	return Command{
		Name:    "pki",
		Summary: "PKI 运维（index/downloads）",
		Group:   "pki",
		Surface: SurfaceBackend,
		Write:   true,
		Run: func(env Env) int {
			if len(env.Args) == 0 {
				return usageFail(env, "用法: pki <index|downloads> ...")
			}
			if env.Args[0] == "--help" || env.Args[0] == "-h" {
				return printUsage(env, "pki.help")
			}
			sub := env.Args[0]
			env.Args = env.Args[1:]
			env.Action = "pki " + sub
			switch sub {
			case "index":
				return newIndexCommand().Run(env)
			case "downloads":
				return newDownloadsCommand().Run(env)
			default:
				return usageFail(env, "未知 pki 子命令: %s", sub)
			}
		},
	}
}

func newTrashCommand() Command {
	return Command{
		Name:    "trash",
		Summary: "回收站（list/restore/purge）",
		Group:   "trash",
		Surface: SurfaceBackend,
		Write:   true,
		Run: func(env Env) int {
			if len(env.Args) == 0 {
				return usageFail(env, "用法: trash <list|restore|purge> ...")
			}
			if env.Args[0] == "--help" || env.Args[0] == "-h" {
				return printUsage(env, "trash.help")
			}
			sub := env.Args[0]
			env.Args = env.Args[1:]
			env.Action = "trash " + sub
			switch sub {
			case "list":
				return runTrashList(env)
			case "restore":
				return runTrashRestore(env)
			case "purge":
				return runTrashPurge(env)
			default:
				return usageFail(env, "未知 trash 子命令: %s", sub)
			}
		},
	}
}

func newCertTrashCommand() Command {
	return Command{
		Name:    "trash",
		Summary: "将证书移入回收站（不物理删除）",
		Group:   "cert",
		Surface: SurfaceBackend,
		Write:   true,
		Run: func(env Env) int {
			pos, _, _, err := parseFlags(env.Args, nil, nil)
			if err != nil || len(pos) != 1 {
				return usageFail(env, "用法: cert trash <CN>")
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.TrashCertificate(context.Background(), deps.cfg, deps.store, deps.oss, deps.engine, pos[0])
			if err != nil {
				return fail(env, err)
			}
			_ = core.AppendAuditWithOrigin(deps.cfg, string(env.Origin), "trash", pos[0], "", "ok")
			if env.JSON {
				return emitJSON(env, "cert trash", pos[0], result.Warnings, map[string]any{"cn": pos[0], "crlPath": result.CRLPath})
			}
			fmt.Fprintf(env.Stdout, "[OK]   %s 已移入回收站\n", pos[0])
			return ExitOK
		},
	}
}

func runTrashList(env Env) int {
	if len(env.Args) != 0 {
		return usageFail(env, "trash list 不接受参数")
	}
	deps, err := newCoreDeps()
	if err != nil {
		return fail(env, err)
	}
	entries, err := core.ListTrash(deps.cfg)
	if err != nil {
		return fail(env, err)
	}
	if env.JSON {
		return emitJSON(env, "trash list", "", nil, map[string]any{"entries": entries})
	}
	if len(entries) == 0 {
		fmt.Fprintln(env.Stdout, "回收站为空")
		return ExitOK
	}
	rows := [][]string{{"ID", "CN", "移入时间"}}
	for _, entry := range entries {
		rows = append(rows, []string{entry.ID, entry.CN, entry.CreatedAt.Format("2006-01-02 15:04:05Z")})
	}
	fmt.Fprint(env.Stdout, core.FormatTable(rows))
	return ExitOK
}

func runTrashRestore(env Env) int {
	pos, _, _, err := parseFlags(env.Args, nil, nil)
	if err != nil || len(pos) != 1 {
		return usageFail(env, "用法: trash restore <ID>")
	}
	deps, err := newCoreDeps()
	if err != nil {
		return fail(env, err)
	}
	entry, err := core.RestoreTrash(context.Background(), deps.cfg, deps.store, deps.oss, deps.engine, pos[0])
	if err != nil {
		return fail(env, err)
	}
	_ = core.AppendAuditWithOrigin(deps.cfg, string(env.Origin), "trash-restore", entry.CN, "", "ok")
	if env.JSON {
		return emitJSON(env, "trash restore", entry.CN, nil, map[string]any{"entry": entry})
	}
	fmt.Fprintf(env.Stdout, "[OK]   %s 已恢复（条目 %s）\n", entry.CN, entry.ID)
	return ExitOK
}

func runTrashPurge(env Env) int {
	pos, _, bools, err := parseFlags(env.Args, nil, []string{"expired"})
	if err != nil {
		return usageFail(env, "用法: trash purge <ID> | --expired")
	}
	deps, err := newCoreDeps()
	if err != nil {
		return fail(env, err)
	}
	if env.JSON && !env.Yes {
		return usageFail(env, "trash purge --json 需要 --yes")
	}
	if bools["expired"] {
		if len(pos) != 0 {
			return usageFail(env, "用法: trash purge --expired")
		}
		ids, err := core.ExpiredTrashIDs(deps.cfg, time.Now())
		if err != nil {
			return fail(env, err)
		}
		if len(ids) == 0 {
			if env.JSON {
				return emitJSON(env, "trash purge", "", nil, map[string]any{"purged": []string{}})
			}
			fmt.Fprintf(env.Stdout, "没有超过保留期（%s）的条目\n", deps.cfg.TrashRetention)
			return ExitOK
		}
		confirmer := TerminalConfirmer{In: env.Stdin, Out: env.Stdout, AssumeYes: env.Yes}
		ok, err := confirmer.Confirm(context.Background(), ConfirmRequest{Action: "trash purge --expired", Subject: fmt.Sprintf("%d 个条目", len(ids)), Default: false})
		if err != nil {
			return fail(env, err)
		}
		if !ok {
			fmt.Fprintln(env.Stdout, "已取消")
			return ExitOK
		}
		for _, id := range ids {
			if err := core.PurgeTrash(deps.cfg, id); err != nil {
				return fail(env, err)
			}
			_ = core.AppendAuditWithOrigin(deps.cfg, string(env.Origin), "trash-purge", id, "", "expired")
		}
		if env.JSON {
			return emitJSON(env, "trash purge", "", nil, map[string]any{"purged": ids})
		}
		fmt.Fprintf(env.Stdout, "[OK]   已清理 %d 个过期条目\n", len(ids))
		return ExitOK
	}
	if len(pos) != 1 {
		return usageFail(env, "用法: trash purge <ID> | --expired")
	}
	confirmer := TerminalConfirmer{In: env.Stdin, Out: env.Stdout, AssumeYes: env.Yes}
	ok, err := confirmer.Confirm(context.Background(), ConfirmRequest{Action: "trash purge", Subject: pos[0], Default: false})
	if err != nil {
		return fail(env, err)
	}
	if !ok {
		fmt.Fprintln(env.Stdout, "已取消")
		return ExitOK
	}
	if err := core.PurgeTrash(deps.cfg, pos[0]); err != nil {
		return fail(env, err)
	}
	_ = core.AppendAuditWithOrigin(deps.cfg, string(env.Origin), "trash-purge", pos[0], "", "ok")
	if env.JSON {
		return emitJSON(env, "trash purge", pos[0], nil, map[string]any{"id": pos[0], "purged": true})
	}
	fmt.Fprintf(env.Stdout, "[OK]   %s 已清理\n", pos[0])
	return ExitOK
}
