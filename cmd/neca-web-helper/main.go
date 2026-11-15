package main

import (
	"fmt"
	"os"

	"github.com/hyhdsl/neca/internal/version"
)

// neca-web-helper 是 M6b 的 root 写边界占位程序。
//
// M6a 只发布只读 Web；本程序故意不监听 socket、不读取 CA 私钥。
// 这样 neca-web 镜像可以保持固定入口，但不会在 M6a 提前开放写能力。
func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-V" {
			fmt.Println(version.Get().String())
			return
		}
	}
	fmt.Fprintln(os.Stderr, "neca-web-helper: M6a 未实现写能力（计划 M6b）")
	os.Exit(1)
}
