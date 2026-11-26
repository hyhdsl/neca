// 本文件实现 config 子命令的读取、校验与展示。
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hyhdsl/neca/internal/core"
)

// newConfigCommand 构造 neca config init|show|validate|export-env。
func newConfigCommand() Command {
	return Command{
		Name:    "config",
		Summary: "管理 JSON 配置文件（init/show/validate/export-env）",
		Run: func(env Env) int {
			if len(env.Args) == 0 {
				return usageFail(env, "用法: config init|show|validate|export-env")
			}
			if env.Args[0] == "--help" || env.Args[0] == "-h" {
				return printUsage(env, "config.help")
			}
			action := env.Args[0]
			switch action {
			case "init":
				return runConfigInit(env)
			case "show", "validate", "export-env":
				if len(env.Args) > 1 {
					return usageFail(env, "config %s 不接受额外参数", action)
				}
				return runConfigRead(env, action)
			case "path":
				fmt.Fprintln(env.Stdout, core.ConfigFilePath())
				return ExitOK
			default:
				return usageFail(env, "未知 config 子命令: %s", action)
			}
		},
	}
}

func runConfigInit(env Env) int {
	pos, _, bools, err := parseFlags(env.Args[1:], nil, []string{"force", "from-env"})
	if err != nil || len(pos) != 0 {
		if err == nil {
			err = fmt.Errorf("用法: config init [--force] [--from-env]")
		}
		return usageFail(env, "%v", err)
	}
	path := core.ConfigFilePath()
	if _, err := os.Stat(path); err == nil && !bools["force"] {
		return usageFail(env, "配置文件已存在: %s（使用 --force 覆盖）", path)
	}
	var data []byte
	if bools["from-env"] {
		cfg, err := core.LoadConfig()
		if err != nil {
			return fail(env, err)
		}
		data, err = core.ConfigFileJSON(cfg)
		if err != nil {
			return fail(env, err)
		}
	} else {
		data, err = core.ExampleConfigJSON()
		if err != nil {
			return fail(env, err)
		}
	}
	if err := core.WriteConfigFile(path, data); err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "[OK]   配置已写入: %s（新文件 0600；覆盖保留原权限）\n", path)
	return ExitOK
}

func runConfigRead(env Env, action string) int {
	cfg, err := core.LoadConfig()
	if err != nil {
		return fail(env, err)
	}
	switch action {
	case "show":
		snapshot := core.ConfigSnapshot(cfg)
		if env.JSON {
			return emitJSON(env, "config-show", "", nil, snapshot)
		}
		data, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			return fail(env, err)
		}
		fmt.Fprintln(env.Stdout, string(data))
		return ExitOK
	case "validate":
		fmt.Fprintf(env.Stdout, "[OK]   配置有效: %s\n", cfg.ConfigFile)
		return ExitOK
	case "export-env":
		values := core.ConfigEnvMap(cfg)
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(env.Stdout, "%s='%s'\n", key, strings.ReplaceAll(values[key], "'", "'\\''"))
		}
		return ExitOK
	default:
		return usageFail(env, "未知 config 子命令: %s", action)
	}
}
