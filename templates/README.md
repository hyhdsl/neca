# templates

平台包模板，M1 起通过 `embed` 编译进二进制（开发文档 §9）。

| 目录 | 内容 | 编码要求 |
|---|---|---|
| `win/` | Windows PowerShell / cmd 脚本 | **纯 ASCII**（`make check-ascii` 强制） |
| `linux/` | Linux `install.sh` / `uninstall.sh` 模板 | **纯 ASCII** |
| `macos/` | macOS `install.command` / `uninstall.command` 模板 | **纯 ASCII** |

Android 使用说明与 iOS/macOS mobileconfig 由 Go 代码生成，不占用模板目录。

**为什么脚本模板必须是 ASCII**：PowerShell 5.1 在无 BOM 时按系统 ANSI
代码页读取脚本，中文会乱码并报语法错误；Linux/macOS 也要避免不同 locale
带来的执行差异。CI 的 `make check-ascii` 会拦截任何非 ASCII 字节。

模板占位符约定：`{{.CN}}`、`{{.Profile}}`、`{{.Server}}`、`{{.CA}}`、`{{.ProfileID}}`。
