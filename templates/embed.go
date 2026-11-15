// Package templates 保存平台包模板，通过 embed 编译进二进制。
//
// Windows 模板必须纯 ASCII（PowerShell 5.1 无 BOM 时按 ANSI 读取）；
// Linux/macOS 脚本也保持 ASCII，避免不同 locale 下的执行差异。
// CI 的 make check-ascii 会校验 templates/ 下的脚本模板。
package templates

import "embed"

// FS 包含 Windows/Linux/macOS 平台模板，文件名后缀 .tmpl 用 text/template 渲染。
//
//go:embed win/*.tmpl linux/*.tmpl macos/*.tmpl
var FS embed.FS
