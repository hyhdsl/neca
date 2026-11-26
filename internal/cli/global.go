// 本文件解析全局开关并构建 CLI 运行环境。
package cli

import (
	"fmt"
	"strings"
)

// globalOptions 是 M3 通用开关；命令专属开关仍由各命令解析。
type globalOptions struct {
	JSON       bool
	DryRun     bool
	Yes        bool
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
		case arg == "--yes" || arg == "-y":
			opts.Yes = true
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

// isWriteCommand 判断命令是否会修改 PKI；用于 --dry-run 拦截。
func isWriteCommand(name string, args []string) bool {
	switch name {
	case "cert":
		if len(args) == 0 {
			return false
		}
		switch args[0] {
		case "issue", "reissue", "revoke", "unrevoke", "trash", "platform", "package", "export":
			return true
		}
		return false
	case "pki":
		if len(args) == 0 {
			return false
		}
		switch args[0] {
		case "index":
			return len(args) > 1 && args[1] == "sync"
		case "downloads":
			return len(args) > 1 && args[1] == "clean"
		}
		return false
	case "trash":
		return len(args) > 0 && (args[0] == "restore" || args[0] == "purge")
	case "web":
		if len(args) == 0 {
			return false
		}
		switch args[0] {
		case "readonly":
			return len(args) > 1 && (args[1] == "on" || args[1] == "off")
		case "idle-timeout":
			return len(args) > 1 && args[1] != "status"
		case "export":
			return len(args) > 1 && (args[1] == "on" || args[1] == "off")
		case "lock", "lockdown":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// guessCN 提取第一个位置参数，供 JSON 信封使用。
func guessCN(name string, args []string) string {
	switch name {
	case "cert":
		// cert <subcommand> [CN...]
		args = args[1:]
	case "trash":
		// trash <subcommand> [ID]
		args = args[1:]
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			return arg
		}
	}
	return ""
}
