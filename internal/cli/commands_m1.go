package cli

import (
	"context"
	"fmt"

	"github.com/hyhdsl/neca/internal/core"
)

// coreDeps 是 M1 只读命令需要的 core 组件。
//
// 每次命令执行都重新加载配置（创建目录骨架、校验 CA），
// 与旧脚本 init_env() 的时机一致。
type coreDeps struct {
	cfg    *core.Config
	store  *core.FileIndexStore
	oss    *core.OpenSSL
	engine *core.Engine
}

// newCoreDeps 构造 core 依赖。
func newCoreDeps() (*coreDeps, error) {
	cfg, err := core.LoadConfig()
	if err != nil {
		return nil, err
	}
	runner := core.NewExecRunner()
	return &coreDeps{
		cfg:    cfg,
		store:  core.NewFileIndexStore(cfg.IndexFile, cfg.LockFile),
		oss:    core.NewOpenSSL(runner),
		engine: core.NewEngine(cfg, runner),
	}, nil
}

// exitForError 把 core 错误类别映射成文档 §6.3 的退出码。
func exitForError(err error) int {
	switch core.KindOf(err) {
	case core.KindUsage:
		return ExitUsage
	case core.KindPrecondition:
		return ExitPrecondition
	default:
		return ExitRuntime
	}
}

// fail 输出 [ERR] 并把 core 错误映射成退出码。
func fail(env Env, err error) int {
	if env.JSON {
		return emitJSONError(env, err)
	}
	fmt.Fprintf(env.Stderr, "[ERR]  %v\n", err)
	return exitForError(err)
}

// usageFail 输出用法错误并返回退出码 1（不走 core 错误类型）。
func usageFail(env Env, format string, args ...any) int {
	err := fmt.Errorf(format, args...)
	if env.JSON {
		return emitUsageJSONError(env, err)
	}
	fmt.Fprintf(env.Stderr, "[ERR]  %v\n", err)
	return ExitUsage
}

// newListCommand 构造 neca list。
//
// 输出契约（开发文档 §6.3）：表体与 [INFO] CRL 行与旧脚本逐字节一致。
func newListCommand() Command {
	return Command{
		Name:    "list",
		Summary: "列出证书（状态/序列号/平台/到期）",
		Run: func(env Env) int {
			if len(env.Args) > 0 {
				return usageFail(env, "list 不接受参数，收到: %v", env.Args)
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.ListCerts(context.Background(), deps.cfg, deps.store, deps.oss)
			if err != nil {
				return fail(env, err)
			}
			if env.JSON {
				certs := make([]map[string]any, 0, len(result.Certs))
				for _, cert := range result.Certs {
					certs = append(certs, map[string]any{
						"cn": cert.CN, "status": cert.Status, "serial": cert.Serial,
						"platforms": cert.Platforms, "expires": cert.Expires, "note": cert.Note,
					})
				}
				return emitJSON(env, "list", "", nil, map[string]any{"certs": certs, "crlRevoked": result.CRLRevoked})
			}

			rows := [][]string{{"CN", "状态", "序列号", "平台", "到期", "备注"}}
			for _, cert := range result.Certs {
				rows = append(rows, []string{cert.CN, cert.Status, cert.Serial, cert.Platforms, cert.Expires, cert.Note})
			}
			fmt.Fprint(env.Stdout, core.FormatTable(rows))
			fmt.Fprintf(env.Stdout, "[INFO] CRL 吊销条目: %d\n", result.CRLRevoked)
			return ExitOK
		},
	}
}

// newVerifyCommand 构造 neca verify [CN...]。
//
// 告警写 stderr、成功行写 stdout，与旧脚本 verify_p12 一致；
// 存在告警时退出码仍为 0（旧脚本行为）。
func newVerifyCommand() Command {
	return Command{
		Name:    "verify",
		Summary: "检查客户端 p12（3DES/证书数/密码长度）",
		Run: func(env Env) int {
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.VerifyClients(context.Background(), deps.cfg, deps.oss, env.Args)
			if err != nil {
				return fail(env, err)
			}
			if env.JSON {
				reports := make([]map[string]any, 0, len(result.Reports))
				var warnings []string
				for _, report := range result.Reports {
					reports = append(reports, map[string]any{
						"cn": report.CN, "ok": report.OKMessage, "warnings": report.Warnings,
					})
					warnings = append(warnings, report.Warnings...)
				}
				return emitJSON(env, "verify", "", warnings, map[string]any{"reports": reports})
			}
			for _, report := range result.Reports {
				for _, warning := range report.Warnings {
					fmt.Fprintf(env.Stderr, "[WARN] %s\n", warning)
				}
				if report.OKMessage != "" {
					fmt.Fprintf(env.Stdout, "[OK]   %s\n", report.OKMessage)
				}
			}
			return ExitOK
		},
	}
}

// newExportCommand 构造 neca export issued|revoked [--out F]。
func newExportCommand() Command {
	return Command{
		Name:    "export",
		Summary: "导出证书清单 CSV（issued | revoked）",
		Run: func(env Env) int {
			kind, outPath, err := parseExportArgs(env.Args)
			if err != nil {
				return usageFail(env, "%v", err)
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.ExportCerts(context.Background(), deps.cfg, deps.store, deps.oss, kind, outPath)
			if err != nil {
				return fail(env, err)
			}
			if env.JSON {
				return emitJSON(env, "export", "", nil, map[string]any{"kind": string(kind), "path": result.Path})
			}
			label := "已签发"
			if kind == core.ExportRevoked {
				label = "已吊销"
			}
			fmt.Fprintf(env.Stdout, "[OK]   已导出%s证书: %s\n", label, result.Path)
			return ExitOK
		},
	}
}

// parseExportArgs 解析 export 的位置参数与 --out。
//
// 语法：export issued|revoked [--out 文件]；两种顺序都接受。
func parseExportArgs(args []string) (core.ExportKind, string, error) {
	var kind core.ExportKind
	outPath := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "issued":
			if kind != "" {
				return "", "", fmt.Errorf("导出类型只能指定一次")
			}
			kind = core.ExportIssued
		case "revoked":
			if kind != "" {
				return "", "", fmt.Errorf("导出类型只能指定一次")
			}
			kind = core.ExportRevoked
		case "--out":
			if index+1 >= len(args) {
				return "", "", fmt.Errorf("--out 缺少文件路径")
			}
			outPath = args[index+1]
			index++
		default:
			return "", "", fmt.Errorf("未知参数: %s（用法: export issued|revoked [--out 文件]）", args[index])
		}
	}
	if kind == "" {
		return "", "", fmt.Errorf("缺少导出类型（issued 或 revoked）")
	}
	return kind, outPath, nil
}

// newIndexCommand 构造 neca index sync。
func newIndexCommand() Command {
	return Command{
		Name:    "index",
		Summary: "索引维护（index sync：补索引与元数据）",
		Run: func(env Env) int {
			if len(env.Args) == 0 {
				return usageFail(env, "缺少子命令（用法: index sync）")
			}
			if env.Args[0] != "sync" {
				return usageFail(env, "未知子命令: index %s（当前仅支持 index sync）", env.Args[0])
			}
			if len(env.Args) > 1 {
				return usageFail(env, "index sync 不接受参数，收到: %v", env.Args[1:])
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			result, err := core.SyncIndex(context.Background(), deps.cfg, deps.store, deps.oss)
			if err != nil {
				return fail(env, err)
			}
			if env.JSON {
				return emitJSON(env, "index-sync", "", nil, map[string]any{"addedIndex": result.AddedIndex})
			}
			for _, file := range result.AddedIndex {
				fmt.Fprintf(env.Stdout, "[OK]   索引补充: %s\n", file)
			}
			fmt.Fprintln(env.Stdout, "[OK]   索引同步完成")
			return ExitOK
		},
	}
}
