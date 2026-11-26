// 本文件实现 cert expiring 到期概览子命令。
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

// newExpiringCommand 构造 neca cert expiring [--days N]。
// 只读：不签发、不改配置；默认查看未来 30 天。
func newExpiringCommand() Command {
	return Command{
		Name:    "expiring",
		Summary: "列出即将到期的证书（默认 30 天）",
		Run: func(env Env) int {
			pos, values, _, err := parseFlags(env.Args, []string{"days"}, nil)
			if err != nil {
				return usageFail(env, "%v", err)
			}
			if len(pos) != 0 {
				return usageFail(env, "cert expiring 不接受位置参数: %v", pos)
			}
			days, err := atoiDefault(values["days"], 30)
			if err != nil {
				return usageFail(env, "--days %v", err)
			}
			deps, err := newCoreDeps()
			if err != nil {
				return fail(env, err)
			}
			now := time.Now().UTC()
			list, err := core.ListWebCerts(context.Background(), deps.cfg, deps.store, deps.oss)
			if err != nil {
				return fail(env, err)
			}
			summary := core.BuildExpirySummary(list.Certs, now)
			certs := core.ExpiringWithin(list.Certs, now, days)
			if env.JSON {
				items := make([]map[string]any, 0, len(certs))
				for _, cert := range certs {
					items = append(items, map[string]any{
						"cn": cert.CN, "status": cert.Status, "expires": cert.Expires,
						"expiresAt": cert.ExpiresAt, "daysLeft": cert.DaysLeft,
					})
				}
				return emitJSON(env, "cert expiring", "", nil, map[string]any{
					"days": days, "expired": summary.Expired,
					"within30": summary.Within30, "within60": summary.Within60,
					"within90": summary.Within90, "certs": items,
				})
			}
			rows := [][]string{{"CN", "状态", "到期", "剩余天数"}}
			for _, cert := range certs {
				rows = append(rows, []string{cert.CN, "有效", cert.Expires, fmt.Sprintf("%d", cert.DaysLeft)})
			}
			fmt.Fprint(env.Stdout, core.FormatTable(rows))
			fmt.Fprintf(env.Stdout, "[INFO] 已过期证书: %d\n", summary.Expired)
			fmt.Fprintf(env.Stdout, "[INFO] %d 天内到期: %d\n", days, len(certs))
			return ExitOK
		},
	}
}
