package cli

import (
	"context"
	"fmt"

	"github.com/hyhdsl/neca/internal/core"
)

// newCertCommand is the v0.5 certificate namespace. It reuses the existing
// command implementations so flags and output stay stable during M9.4.
func newCertCommand() Command {
	return Command{
		Name:    "cert",
		Summary: "证书管理（list/verify/issue/reissue/revoke/unrevoke/trash/export/platform/package）",
		Group:   "cert",
		Surface: SurfaceBackend,
		Write:   true,
		Run: func(env Env) int {
			if len(env.Args) == 0 {
				return usageFail(env, "用法: cert <list|verify|issue|reissue|revoke|unrevoke|trash|export|platform|package>")
			}
			sub := env.Args[0]
			env.Args = env.Args[1:]
			env.Action = "cert " + sub
			switch sub {
			case "list":
				return newListCommand().Run(env)
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
	pos, _, _, err := parseFlags(env.Args, nil, nil)
	if err != nil || len(pos) != 1 {
		return usageFail(env, "用法: trash purge <ID>")
	}
	deps, err := newCoreDeps()
	if err != nil {
		return fail(env, err)
	}
	if env.JSON && !env.Yes {
		return usageFail(env, "trash purge --json 需要 --yes")
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
