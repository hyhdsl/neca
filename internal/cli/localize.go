// 本文件将 CLI 文本本地化并处理语言回退。
package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// runLocalizedCommand 捕获命令输出并翻译；向导等交互命令不走这里。
func runLocalizedCommand(env Env, cmd *Command, stdout, stderr io.Writer) int {
	var out, errOut bytes.Buffer
	env.Stdout = &out
	env.Stderr = &errOut
	code := cmd.Run(env)
	fmt.Fprint(stdout, localizeOutput(out.String()))
	fmt.Fprint(stderr, localizeOutput(errOut.String()))
	return code
}

// localizeOutput 把中文人类输出转换为英文；仅在 --lang en-US 且非 JSON 时使用。
// 核心仍返回中文结构化结果，本层负责展示，避免把 i18n 依赖下沉到 core。
func localizeOutput(text string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = localizeLine(lines[i])
	}
	return strings.Join(lines, "\n")
}

func localizeLine(line string) string {
	if line == "" {
		return line
	}
	if strings.HasPrefix(line, "CN ") && strings.Contains(line, "状态") {
		return strings.NewReplacer(
			"状态", "Status",
			"序列号", "Serial",
			"平台", "Platforms",
			"到期", "Expires",
			"剩余天数", "Days left",
			"备注", "Note",
		).Replace(line)
	}
	if strings.HasPrefix(line, "[OK]   p12 检查: ") {
		if output, ok := localizeP12Check(line); ok {
			return output
		}
	}
	if strings.HasPrefix(line, "[INFO] ") && strings.Contains(line, " 天内到期: ") {
		rest := strings.TrimPrefix(line, "[INFO] ")
		days, count, ok := strings.Cut(rest, " 天内到期: ")
		if ok {
			return "[INFO] expiring within " + days + " days: " + count
		}
	}
	if strings.HasPrefix(line, "[WARN] ") {
		return localizeWarning(line)
	}
	if strings.HasPrefix(line, "[INFO] 生成密钥与证书 ") {
		if output, ok := localizeIssueStart(line); ok {
			return output
		}
	}
	if strings.HasPrefix(line, "[INFO] 旧证书已归档并吊销（") {
		serial := strings.TrimSuffix(strings.TrimPrefix(line, "[INFO] 旧证书已归档并吊销（"), "）")
		return "[INFO] Old certificate archived and revoked (" + serial + ")"
	}
	if strings.HasPrefix(line, "[OK]   已签发 ") {
		if output, ok := localizeIssued(line); ok {
			return output
		}
	}
	if strings.HasPrefix(line, "[OK]   已吊销 ") {
		rest := strings.TrimPrefix(line, "[OK]   已吊销 ")
		cn, tail, ok := strings.Cut(rest, "（序列 ")
		if ok {
			return "[OK]   Revoked " + cn + " (serial " + strings.TrimSuffix(tail, "）") + ")"
		}
	}
	if strings.HasPrefix(line, "[OK]   已恢复 ") {
		rest := strings.TrimPrefix(line, "[OK]   已恢复 ")
		cn, tail, ok := strings.Cut(rest, "（序列 ")
		if ok {
			return "[OK]   Unrevoked " + cn + " (serial " + strings.TrimSuffix(tail, "）") + ")"
		}
	}
	if strings.HasPrefix(line, "[OK]   zip: ") {
		rest := strings.TrimPrefix(line, "[OK]   zip: ")
		path, tail, ok := strings.Cut(rest, "（有效期至 ")
		if ok {
			return "[OK]   zip: " + path + " (expires " + strings.TrimSuffix(tail, "）") + ")"
		}
	}
	return replaceCommon(line)
}

func localizeP12Check(line string) (string, bool) {
	rest := strings.TrimPrefix(line, "[OK]   p12 检查: ")
	cn, meta, ok := strings.Cut(rest, " (")
	if !ok || !strings.HasSuffix(meta, ")") {
		return "", false
	}
	meta = strings.TrimSuffix(meta, ")")
	parts := strings.Split(meta, ", ")
	if len(parts) != 3 {
		return "", false
	}
	algorithm := strings.ReplaceAll(parts[0], "未知", "unknown")
	count := strings.TrimSuffix(strings.TrimPrefix(parts[1], "证书 "), " 张")
	password := strings.TrimSuffix(strings.TrimPrefix(parts[2], "密码 "), " 位")
	return fmt.Sprintf("[OK]   p12 check: %s (%s, %s cert(s), password %s chars)", cn, algorithm, count, password), true
}

func localizeWarning(line string) string {
	payload := strings.TrimPrefix(line, "[WARN] ")
	if strings.HasPrefix(payload, "p12 不存在: ") {
		return "[WARN] p12 missing: " + strings.TrimPrefix(payload, "p12 不存在: ")
	}
	if cn, rest, ok := strings.Cut(payload, ": p12 使用 PBES2/AES"); ok {
		_ = rest
		return "[WARN] " + cn + ": p12 uses PBES2/AES (MIUI may stall at \"extract certificate\")"
	}
	if cn, _, ok := strings.Cut(payload, ": p12 未见 3DES 加密"); ok {
		return "[WARN] " + cn + ": p12 does not use 3DES encryption"
	}
	if cn, rest, ok := strings.Cut(payload, ": p12 内含 "); ok {
		count, _, _ := strings.Cut(rest, " 张证书")
		return "[WARN] " + cn + ": p12 contains " + count + " certificate(s) (MIUI recommends client cert only; install CA separately)"
	}
	if cn, rest, ok := strings.Cut(payload, ": p12 密码长度 "); ok {
		length, _, _ := strings.Cut(rest, " 过短")
		return "[WARN] " + cn + ": p12 password is too short (" + length + " bytes)"
	}
	if cn, _, ok := strings.Cut(payload, ": p12 未见 3DES 加密"); ok {
		return "[WARN] " + cn + ": p12 does not use 3DES encryption"
	}
	return replaceCommon(line)
}

func localizeIssueStart(line string) (string, bool) {
	rest := strings.TrimPrefix(line, "[INFO] 生成密钥与证书 ")
	cn, tail, ok := strings.Cut(rest, "（")
	if !ok {
		return "", false
	}
	days, _, ok := strings.Cut(tail, " 天）...")
	if !ok {
		return "", false
	}
	return "[INFO] Generating key and certificate " + cn + " (" + days + " days)...", true
}

func localizeIssued(line string) (string, bool) {
	rest := strings.TrimPrefix(line, "[OK]   已签发 ")
	cn, tail, ok := strings.Cut(rest, "（序列 ")
	if !ok {
		return "", false
	}
	serial, tail, ok := strings.Cut(tail, "，平台 ")
	if !ok {
		return "", false
	}
	return "[OK]   Issued " + cn + " (serial " + serial + ", platforms " + strings.TrimSuffix(tail, "）") + ")", true
}

func replaceCommon(line string) string {
	replacer := strings.NewReplacer(
		"[INFO] CRL 吊销条目:", "[INFO] CRL revoked entries:",
		"[INFO] 已过期证书:", "[INFO] expired certificates:",
		"[OK]   CRL 已更新:", "[OK]   CRL updated:",
		"[OK]   已导出已签发证书:", "[OK]   Exported issued certificates:",
		"[OK]   已导出已吊销证书:", "[OK]   Exported revoked certificates:",
		"[OK]   索引补充:", "[OK]   Index added:",
		"[OK]   索引同步完成", "[OK]   Index sync complete",
		"[OK]   已清理 ", "[OK]   Removed ",
		"[INFO] 配置文件:", "[INFO] Config file:",
		"[INFO] Web: 已启用", "[INFO] Web: enabled",
		"[INFO] Web: 未启用（lite 默认；使用 neca-web 镜像并启用 web.enabled）", "[INFO] Web: disabled (lite default; use the neca-web image and enable web.enabled)",
		"[INFO] Web: 未启用", "[INFO] Web: disabled",
		"[INFO] 监听:", "[INFO] Listen:",
		"只读（", "read-only (",
		"可写（", "writable (",
		"（绝对上限 ", " (absolute limit ",
		"，导出=", ", export=",
		"）", ")",
		"（当前仅保留接口）", " (interface only)",
		"（当前仅保留接口，不注册路由）", " (interface only)",
		"[INFO] helper: 不可用:", "[INFO] helper: unavailable:",
		"[INFO] 模式:", "[INFO] Mode:",
		"[INFO] 空闲超时:", "[INFO] Idle timeout:",
		"[WARN] 空闲超时配置无效:", "[WARN] invalid idle timeout:",
		"[INFO] Passkey: 已启用", "[INFO] Passkey: enabled",
		"[INFO] Passkey: 关闭", "[INFO] Passkey: disabled",
		"[INFO] 当前模式:", "[INFO] Current mode:",
		"[OK]   Web CSV 导出已开启", "[OK]   Web CSV export enabled",
		"[OK]   Web CSV 导出已关闭", "[OK]   Web CSV export disabled",
		"[OK]   Web 与 helper 已停止（web、web-helper）", "[OK]   Web and helper stopped (web, web-helper)",
		"[OK]   Web 已切换为只读", "[OK]   Web switched to read-only",
		"[OK]   Web 已切换为可写（root helper 已就绪）", "[OK]   Web switched to writable (root helper ready)",
		"[OK]   Web 已锁定：只读，sessionEpoch=", "[OK]   Web locked: read-only, sessionEpoch=",
		"移入时间", "Moved at",
		"已移入回收站", "moved to recycle bin",
		"没有超过保留期", "no entries older than retention",
		"回收站为空", "recycle bin is empty",
		"加载配置: 找不到 CA 证书:", "load config: CA certificate not found:",
		"加载配置:", "load config:",
		"找不到 CA 证书:", "CA certificate not found:",
		"未知 cert 子命令:", "unknown cert subcommand:",
		"未知 pki 子命令:", "unknown pki subcommand:",
		"未知 trash 子命令:", "unknown trash subcommand:",
		"未知 web 子命令:", "unknown web subcommand:",
		"未知 config 子命令:", "unknown config subcommand:",
		"用法:", "usage:",
		"不接受参数", "does not accept arguments",
		"未知参数:", "unknown argument:",
		"未知子命令:", "unknown subcommand:",
		"当前仅支持", "currently only supports",
		"，收到:", ", received:",
		"（当前仅支持", " (currently only supports",
		"配置文件已存在:", "config file already exists:",
		"回收站为空", "recycle bin is empty",
		"已移入回收站", "moved to recycle bin",
		"当前模式:", "current mode:",
		"个下载文件", "download file(s)",
		"Windows 包:", "Windows bundle:",
		"Linux 包:", "Linux bundle:",
		"macOS 包:", "macOS bundle:",
		"Android 包:", "Android bundle:",
		"iOS 包:", "iOS bundle:",
		"[OK]   已为 ", "[OK]   Added platforms for ",
		" 增加平台: ", ": ",
		"[OK]   平台标注已更新: ", "[OK]   Platform metadata updated: ",
		"[OK]   平台包已生成: ", "[OK]   Bundles generated: ",
		"[OK]   已删除 ", "[OK]   Deleted ",
		"字节", "bytes",
		"过期:", "expires:",
		"[WARN] 未找到容器 ", "[WARN] container ",
		"请手动重启 strongSwan", "not found; restart strongSwan manually",
		"[WARN] 重启容器失败，请手动检查", "[WARN] container restart failed; check manually",
		"[WARN] 等待 strongSwan 启动超时，请手动检查", "[WARN] timed out waiting for strongSwan; check manually",
		"[WARN] swanctl --load-creds 失败，请手动重启 strongSwan（下次重启自动生效）", "[WARN] swanctl --load-creds failed; restart strongSwan manually (next restart will apply the CRL)",
		"[WARN] 审计日志:", "[WARN] audit log:",
		"[ERR]  找不到证书:", "[ERR]  certificate not found:",
		"[ERR]  证书已存在:", "[ERR]  certificate already exists:",
		"[OK]   配置已写入:", "[OK]   Config written:",
		"[OK]   配置有效:", "[OK]   Configuration valid:",
		"(证书已删除)", "(deleted)",
		"文件已删除", "file deleted",
		"已处于吊销状态", "is already revoked",
		"未处于吊销状态", "is not revoked",
		"有效", "Valid",
		"已吊销", "Revoked",
		"未索引", "Unindexed",
	)
	return replacer.Replace(line)
}
