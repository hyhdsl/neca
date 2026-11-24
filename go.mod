// neca 模块定义。
//
// 模块路径按开发文档 D2 固定为 github.com/hyhdsl/neca。
// Go 版本与开发镜像 golang:1.26.11-bookworm 对齐（文档 §18.3）；
// 1.26.11 覆盖了 govulncheck 报告的标准库安全修复，因此不再声明 1.25。
//
// 依赖策略见文档 §5：M1 引入 golang.org/x/text/width（中文表格对齐），
// M6a 引入 golang.org/x/crypto/bcrypt（管理员密码哈希）；除此之外只使用标准库。
module github.com/hyhdsl/neca

go 1.26.11

require (
	golang.org/x/crypto v0.48.0
	golang.org/x/text v0.34.0
)
