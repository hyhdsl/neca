# templates

平台包模板，M1 起通过 `embed` 编译进二进制（开发文档 §9）。

| 目录 | 内容 | 编码要求 |
|---|---|---|
| `win/` | Windows PowerShell / cmd 脚本、使用说明 | **纯 ASCII**（硬规则，`make check-ascii` 强制） |
| `android/` | Android 使用说明文本 | UTF-8 |
| `ios/` | iOS mobileconfig 片段（M1 起用 plist 生成器，可能无需模板） | UTF-8 |

**为什么 Windows 模板必须是 ASCII**：PowerShell 5.1 在无 BOM 时按系统 ANSI
代码页读取脚本，中文会乱码并报语法错误。宁可英文提示，也不要在 `win/` 下
写中文；CI 的 `make check-ascii` 会拦截任何非 ASCII 字节。

模板占位符约定（M1 落地时使用）：`{{.CN}}`、`{{.Profile}}`、`{{.Server}}`。
