// 本文件实现 neca pass：通过 helper 管理 necap 的 TOTP/Passkey/恢复码与策略。
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hyhdsl/neca/internal/helper"
	"github.com/hyhdsl/neca/internal/i18n"
)

type passChallenge struct {
	Complete     bool     `json:"complete"`
	UserID       string   `json:"userId"`
	Username     string   `json:"username"`
	PendingToken string   `json:"pendingToken"`
	Methods      []string `json:"methods"`
}

type passResult struct {
	Complete        bool   `json:"complete"`
	UserID          string `json:"userId"`
	ManagementToken string `json:"managementToken"`
}

type passStatus struct {
	Policy        string `json:"Policy"`
	TOTP          bool   `json:"TOTP"`
	Passkeys      int    `json:"Passkeys"`
	RecoveryCodes int    `json:"RecoveryCodes"`
}

type passTOTPEnrollment struct {
	ChallengeID string `json:"ChallengeID"`
	URI         string `json:"URI"`
}

type passkeyInfo struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"createdAt"`
}

func newPassCommand() Command {
	return Command{
		Name:    "pass",
		Summary: "管理 necap 的 TOTP、Passkey、恢复码与策略（需再认证）",
		Run: func(env Env) int {
			if len(env.Args) > 0 && (env.Args[0] == "--help" || env.Args[0] == "-h") {
				fmt.Fprintln(env.Stdout, i18n.T("pass.help"))
				return ExitOK
			}
			if len(env.Args) == 0 || env.Args[0] == "status" {
				return runPassStatus(env)
			}
			switch env.Args[0] {
			case "totp":
				return runPassTOTP(env, env.Args[1:])
			case "passkey":
				return runPassKey(env, env.Args[1:])
			case "recovery":
				return runPassRecovery(env, env.Args[1:])
			case "policy":
				return runPassPolicy(env, env.Args[1:])
			default:
				return usageFail(env, "未知 pass 子命令: %s（可用 status/totp/passkey/recovery/policy）", env.Args[0])
			}
		},
	}
}

func runPassStatus(env Env) int {
	token, userID, err := passReauth(env)
	if err != nil {
		return fail(env, err)
	}
	var status passStatus
	if err := passNeCap(context.Background(), "/v1/factors/status", map[string]any{"token": token, "userId": userID}, &status); err != nil {
		return fail(env, err)
	}
	if env.JSON {
		return emitJSON(env, "pass-status", "", nil, map[string]any{
			"policy": status.Policy, "totp": status.TOTP, "passkeys": status.Passkeys, "recoveryCodes": status.RecoveryCodes,
		})
	}
	fmt.Fprintf(env.Stdout, "[INFO] 策略: %s\n", status.Policy)
	fmt.Fprintf(env.Stdout, "[INFO] TOTP: %t\n", status.TOTP)
	fmt.Fprintf(env.Stdout, "[INFO] Passkey: %d\n", status.Passkeys)
	fmt.Fprintf(env.Stdout, "[INFO] 剩余恢复码: %d\n", status.RecoveryCodes)
	return ExitOK
}

func runPassTOTP(env Env, args []string) int {
	if len(args) == 0 {
		return usageFail(env, "用法: pass totp <add|remove>")
	}
	token, userID, err := passReauth(env)
	if err != nil {
		return fail(env, err)
	}
	switch args[0] {
	case "add":
		var enrollment passTOTPEnrollment
		if err := passNeCap(context.Background(), "/v1/factors/totp/begin", map[string]any{"token": token, "userId": userID}, &enrollment); err != nil {
			return fail(env, err)
		}
		fmt.Fprintf(env.Stdout, "[INFO] 将以下 URI 导入验证器（只显示一次）:\n%s\n", enrollment.URI)
		code, err := passPrompt(env, "TOTP 验证码: ")
		if err != nil {
			return fail(env, err)
		}
		var response struct {
			RecoveryCodes []string `json:"recoveryCodes"`
		}
		if err := passNeCap(context.Background(), "/v1/factors/totp/finish", map[string]any{
			"token": token, "userId": userID, "challengeId": enrollment.ChallengeID, "code": code,
		}, &response); err != nil {
			return fail(env, err)
		}
		fmt.Fprintln(env.Stdout, "[OK]   TOTP 已启用")
		if len(response.RecoveryCodes) > 0 {
			fmt.Fprintln(env.Stdout, "[INFO] 一次性恢复码（只显示一次）:")
			for _, value := range response.RecoveryCodes {
				fmt.Fprintln(env.Stdout, value)
			}
		}
		return ExitOK
	case "remove":
		if err := passNeCap(context.Background(), "/v1/factors/totp/delete", map[string]any{"token": token, "userId": userID}, nil); err != nil {
			return fail(env, err)
		}
		fmt.Fprintln(env.Stdout, "[OK]   TOTP 已删除")
		return ExitOK
	default:
		return usageFail(env, "未知 pass totp 子命令: %s", args[0])
	}
}

func runPassKey(env Env, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		token, userID, err := passReauth(env)
		if err != nil {
			return fail(env, err)
		}
		var response struct {
			Passkeys []passkeyInfo `json:"passkeys"`
		}
		if err := passNeCap(context.Background(), "/v1/factors/passkeys", map[string]any{"token": token, "userId": userID}, &response); err != nil {
			return fail(env, err)
		}
		if env.JSON {
			return emitJSON(env, "pass-passkey-list", "", nil, map[string]any{"passkeys": response.Passkeys})
		}
		if len(response.Passkeys) == 0 {
			fmt.Fprintln(env.Stdout, "[INFO] 没有已注册的 Passkey")
			return ExitOK
		}
		for _, item := range response.Passkeys {
			fmt.Fprintf(env.Stdout, "%s  %s  %s\n", item.ID, item.Label, item.CreatedAt)
		}
		return ExitOK
	}
	if args[0] != "remove" {
		return usageFail(env, "用法: pass passkey <list|remove>")
	}
	if len(args) != 2 {
		return usageFail(env, "用法: pass passkey remove <id>")
	}
	token, userID, err := passReauth(env)
	if err != nil {
		return fail(env, err)
	}
	if err := passNeCap(context.Background(), "/v1/factors/passkey/delete", map[string]any{"token": token, "userId": userID, "id": args[1]}, nil); err != nil {
		return fail(env, err)
	}
	fmt.Fprintln(env.Stdout, "[OK]   Passkey 已删除")
	return ExitOK
}

func runPassRecovery(env Env, args []string) int {
	if len(args) != 1 || args[0] != "regenerate" {
		return usageFail(env, "用法: pass recovery regenerate")
	}
	token, userID, err := passReauth(env)
	if err != nil {
		return fail(env, err)
	}
	var response struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := passNeCap(context.Background(), "/v1/factors/recovery/regenerate", map[string]any{"token": token, "userId": userID}, &response); err != nil {
		return fail(env, err)
	}
	fmt.Fprintln(env.Stdout, "[OK]   恢复码已重新生成（只显示一次）:")
	for _, value := range response.RecoveryCodes {
		fmt.Fprintln(env.Stdout, value)
	}
	return ExitOK
}

func runPassPolicy(env Env, args []string) int {
	if len(args) != 1 {
		return usageFail(env, "用法: pass policy <password|totp|passkey|any>")
	}
	token, userID, err := passReauth(env)
	if err != nil {
		return fail(env, err)
	}
	if err := passNeCap(context.Background(), "/v1/factors/policy", map[string]any{"token": token, "userId": userID, "mode": args[0]}, nil); err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "[OK]   策略已设置为 %s\n", args[0])
	return ExitOK
}

func passReauth(env Env) (string, string, error) {
	reader := bufio.NewReader(env.Stdin)
	username := strings.TrimSpace(os.Getenv("NECA_NECAP_USERNAME"))
	if username == "" {
		username = "admin"
	}
	fmt.Fprint(env.Stdout, "管理员密码: ")
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("读取密码: %w", err)
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return "", "", fmt.Errorf("密码不能为空")
	}
	var challenge passChallenge
	if err := passNeCap(context.Background(), "/v1/auth/password", map[string]any{
		"username": username, "password": password, "reauth": true,
	}, &challenge); err != nil {
		return "", "", err
	}
	if challenge.Complete && challenge.PendingToken != "" {
		return challenge.PendingToken, challenge.UserID, nil
	}
	if !hasPassMethod(challenge.Methods, "totp") && !hasPassMethod(challenge.Methods, "recovery") {
		return "", "", fmt.Errorf("当前仅支持 Passkey，CLI 不能完成 Passkey；请使用恢复码或 Web 设置")
	}
	fmt.Fprint(env.Stdout, "TOTP 或恢复码: ")
	code, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("读取验证码: %w", err)
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", "", fmt.Errorf("验证码不能为空")
	}
	if hasPassMethod(challenge.Methods, "totp") {
		var result passResult
		if err := passNeCap(context.Background(), "/v1/auth/totp", map[string]any{"pendingToken": challenge.PendingToken, "code": code}, &result); err == nil && result.Complete && result.ManagementToken != "" {
			return result.ManagementToken, result.UserID, nil
		}
	}
	if hasPassMethod(challenge.Methods, "recovery") {
		var result passResult
		if err := passNeCap(context.Background(), "/v1/auth/recovery", map[string]any{"pendingToken": challenge.PendingToken, "code": code}, &result); err == nil && result.Complete && result.ManagementToken != "" {
			return result.ManagementToken, result.UserID, nil
		}
	}
	return "", "", fmt.Errorf("第二因素验证失败")
}

func hasPassMethod(methods []string, want string) bool {
	for _, method := range methods {
		if method == want {
			return true
		}
	}
	return false
}

func passPrompt(env Env, label string) (string, error) {
	fmt.Fprint(env.Stdout, label)
	reader := bufio.NewReader(env.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func passNeCap(ctx context.Context, path string, payload any, out any) error {
	client := helper.NewClient(helperSocketPath())
	status, body, err := client.NeCap(ctx, path, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("necap HTTP %d", status)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("解析 necap 响应: %w", err)
		}
	}
	return nil
}
