// Package version 集中定义构建期注入的版本元数据。
//
// 变量由 Makefile / CI 通过 -ldflags "-X ..." 注入，例如：
//
//	-X github.com/hyhdsl/neca/internal/version.Version=v0.1.0
//	-X github.com/hyhdsl/neca/internal/version.Commit=8fa1f7d
//	-X github.com/hyhdsl/neca/internal/version.Date=2026-11-15T00:00:00Z
//
// 未注入时保留开发期默认值，因此 go run ./cmd/neca version 也能直接工作。
// 本包刻意不依赖任何内部包，避免版本信息把依赖图变复杂。
package version

import (
	"fmt"
	"runtime"
)

// 构建期可注入的变量。必须是可写变量才能被 -ldflags -X 覆盖；
// 运行期不应再修改它们。
var (
	// Version 是语义化版本号，例如 v0.1.0；开发构建为 dev。
	Version = "dev"
	// Commit 是构建来源的 git 提交短哈希；无法获取时为 none。
	Commit = "none"
	// Date 是构建时间（UTC，RFC3339）；无法获取时为 unknown。
	Date = "unknown"
)

// Info 是版本信息的结构化表示，供 CLI 渲染，未来也可直接放进 --json 信封。
type Info struct {
	Version   string // 语义化版本号
	Commit    string // git 提交短哈希
	Date      string // 构建时间（UTC RFC3339）
	GoVersion string // 编译期 Go 工具链版本
	OS        string // 目标操作系统
	Arch      string // 目标架构
}

// Get 采集当前二进制的版本信息。
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String 渲染一行人类可读的版本摘要，例如：
//
//	neca dev (commit none, built unknown, go1.25.14 linux/amd64)
//
// 格式一旦发布就会被运维脚本 grep，改动前请先更新版本测试。
func (i Info) String() string {
	return fmt.Sprintf("neca %s (commit %s, built %s, %s %s/%s)",
		i.Version, i.Commit, i.Date, i.GoVersion, i.OS, i.Arch)
}
