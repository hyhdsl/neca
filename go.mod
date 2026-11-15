// neca 模块定义。
//
// 模块路径按开发文档 D2 固定为 github.com/hyhdsl/neca。
// Go 版本与开发镜像 golang:1.25-bookworm 内实测的 go1.25.14 对齐（文档 §18.3），
// 因此这里声明 go 1.25 而不是更低的兼容版本。
//
// 依赖策略见文档 §5：M1 引入 golang.org/x/text/width（中文表格对齐），
// M6a 引入 golang.org/x/crypto/bcrypt（管理员密码哈希）；除此之外只使用标准库。
module github.com/hyhdsl/neca

go 1.25

require (
	golang.org/x/crypto v0.48.0
	golang.org/x/text v0.34.0
)
