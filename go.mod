// neca 模块定义。
//
// 模块路径按开发文档 D2 固定为 github.com/hyhdsl/neca。
// Go 版本与开发镜像 golang:1.25-bookworm 内实测的 go1.25.14 对齐（文档 §18.3），
// 因此这里声明 go 1.25 而不是更低的兼容版本。
//
// 依赖策略见文档 §5：M0 只使用标准库；M1 引入唯一第三方依赖
// golang.org/x/text/width（中文宽度对齐），届时再更新本文件与 go.sum。
module github.com/hyhdsl/neca

go 1.25
