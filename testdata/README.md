# testdata

测试固定数据（开发文档 §13）。

| 路径 | 内容 | 里程碑 |
|---|---|---|
| `golden/` | list / export / verify 的期望输出（`-update` 更新） | M1 |
| `gen-fixture.sh` | 用 mktemp 生成独立 CA 与两张证书，路径写入 `fixture.env` | M2 |

约束：**任何测试都不得触碰 `/opt/ipsec`**（文档 §13.2）。测试一律通过
`t.Setenv` 把 `PKI_DIR` 等变量重定向到临时目录；`fixture.env` 属于本地
生成产物，已被 `.gitignore` 忽略。
