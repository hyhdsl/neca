// neca 模块定义。
//
// 模块路径按开发文档 D2 固定为 github.com/hyhdsl/neca。
// Go 版本与开发镜像 golang:1.26.11-bookworm 对齐（文档 §18.3）；
// 1.26.11 覆盖了 govulncheck 报告的标准库安全修复，因此不再声明 1.25。
//
// 依赖策略见文档 §5：M1 引入 golang.org/x/text/width，M6a 引入
// golang.org/x/crypto/bcrypt；v1.7 necap 额外引入纯 Go SQLite 与 WebAuthn 库，
// 两者需单独供应链审查并锁定版本。
module github.com/hyhdsl/neca

go 1.26.11

require (
	github.com/go-webauthn/webauthn v0.18.2
	golang.org/x/crypto v0.59.0
	golang.org/x/text v0.44.0
	modernc.org/sqlite v1.63.0
)

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.4 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.3.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sys v0.50.0 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
