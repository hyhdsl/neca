// Package templates 保存平台包模板，通过 embed 编译进二进制。
//
// 只有 Windows 模板是纯 ASCII（PowerShell 5.1 无 BOM 时按 ANSI 读取），
// CI 的 make check-ascii 会校验 templates/win/*。
package templates

import "embed"

// FS 包含 Windows 平台模板；文件名后缀 .tmpl 用 text/template 渲染。
//
//go:embed win/*.tmpl
var FS embed.FS
