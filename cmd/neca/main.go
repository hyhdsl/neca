// Command neca 是 strongSwan IKEv2 用户证书管理工具的 CLI 入口。
//
// 本文件刻意保持"薄"：只负责把 os.Args 交给 internal/cli，并把返回的
// 退出码传给 os.Exit。参数解析、子命令分发与输出渲染全部在 internal/cli，
// 这样阶段二新增 cmd/necad（Web 面板）时可以复用同一套 core，而不必重复入口逻辑。
package main

import (
	"os"

	"github.com/hyhdsl/neca/internal/cli"
)

func main() {
	// args[0] 是程序名，不参与子命令解析，因此显式切掉。
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
