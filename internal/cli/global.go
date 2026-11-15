package cli

import (
	"fmt"
	"strings"
)

// globalOptions 是 M3 通用开关；命令专属开关仍由各命令解析。
type globalOptions struct {
	JSON       bool
	DryRun     bool
	Quiet      bool
	Lang       string
	Version    bool
	Config     string
	ReloadMode string
}

// parseGlobals 从任意位置提取通用开关，返回剩余参数。
func parseGlobals(args []string) ([]string, globalOptions, error) {
	var opts globalOptions
	var rest []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--dry-run":
			opts.DryRun = true
		case arg == "-q" || arg == "--quiet":
			opts.Quiet = true
		case arg == "--version" || arg == "-V":
			opts.Version = true
		case arg == "--config":
			if index+1 >= len(args) {
				return nil, opts, fmt.Errorf("--config 缺少文件路径")
			}
			opts.Config = args[index+1]
			index++
		case strings.HasPrefix(arg, "--config="):
			opts.Config = strings.TrimPrefix(arg, "--config=")
		case arg == "--reload-mode":
			if index+1 >= len(args) {
				return nil, opts, fmt.Errorf("--reload-mode 缺少值")
			}
			opts.ReloadMode = args[index+1]
			index++
		case strings.HasPrefix(arg, "--reload-mode="):
			opts.ReloadMode = strings.TrimPrefix(arg, "--reload-mode=")
		case arg == "--lang":
			if index+1 >= len(args) {
				return nil, opts, fmt.Errorf("--lang 缺少值")
			}
			opts.Lang = args[index+1]
			index++
		case strings.HasPrefix(arg, "--lang="):
			opts.Lang = strings.TrimPrefix(arg, "--lang=")
		default:
			rest = append(rest, arg)
		}
	}
	return rest, opts, nil
}

// compatArgs 把旧版 flag 映射为等价子命令（文档 §6.1 双轨期）。
// 未知位置参数不改写；映射只发生在首个参数。
func compatArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	first := args[0]
	tail := append([]string(nil), args[1:]...)
	switch first {
	case "--list":
		return append([]string{"list"}, tail...)
	case "--verify":
		return append([]string{"verify"}, tail...)
	case "--sync":
		return append([]string{"index", "sync"}, tail...)
	case "--export-issued":
		return append([]string{"export", "issued"}, tail...)
	case "--export-revoked":
		return append([]string{"export", "revoked"}, tail...)
	case "--issue":
		return append([]string{"issue"}, tail...)
	case "--reissue":
		return append([]string{"reissue"}, tail...)
	case "--revoke":
		return append([]string{"revoke"}, tail...)
	case "--unrevoke":
		return append([]string{"unrevoke"}, tail...)
	case "--delete":
		return append([]string{"delete"}, tail...)
	case "--add-platform":
		return append([]string{"platform", "add"}, tail...)
	case "--set-platforms":
		return append([]string{"platform", "set"}, tail...)
	default:
		return args
	}
}

// isWriteCommand 判断命令是否会修改 PKI；用于 --dry-run 拦截。
func isWriteCommand(name string, args []string) bool {
	switch name {
	case "issue", "reissue", "revoke", "unrevoke", "delete", "platform", "package", "export":
		return true
	case "index":
		return len(args) > 0 && args[0] == "sync"
	case "downloads":
		return len(args) > 0 && args[0] == "clean"
	case "web":
		if len(args) == 0 {
			return false
		}
		switch args[0] {
		case "readonly":
			return len(args) > 1 && (args[1] == "on" || args[1] == "off")
		case "idle-timeout":
			return len(args) > 1 && args[1] != "status"
		default:
			return false
		}
	default:
		return false
	}
}

// guessCN 提取第一个位置参数，供 JSON 信封使用。
func guessCN(name string, args []string) string {
	if name == "platform" {
		args = args[1:]
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			return arg
		}
	}
	return ""
}
