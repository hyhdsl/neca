# neca Makefile —— 唯一构建入口（开发文档 §14.2）。
#
# 设计原则：
#   1. 宿主机只负责跑 docker，真正的 go build / vet / test 全部在 neca-dev
#      容器内执行，避免"本机有 Go / 本机没 Go"带来的环境漂移（决策 D7）。
#   2. 所有产物落在 dist/，不进版本库（见 .gitignore）。
#   3. 版本、提交号与构建时间通过 -ldflags 注入 internal/version，
#      二进制自身即可报告来源（neca version），便于线上核对部署版本。
#
# 常用目标见 make help 或 README.md 的"开发命令"表。

SHELL := /bin/bash

GO      ?= go
MODULE  := github.com/hyhdsl/neca
DIST    := dist

# 版本信息：优先取最近的 tag（git describe），没有 tag 时退回短哈希/分支名。
# BUILD_DATE 固定使用 UTC，避免开发机与 CI 时区不同导致产物不可复现。
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# -s -w 去掉符号表与调试信息；-X 注入版本元数据。
# 变量路径必须与 internal/version 中的定义保持一致，否则 -X 会静默失效。
LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(BUILD_DATE)

.DEFAULT_GOAL := help

.PHONY: help all build build-lite build-web build-native test vet fmt fmt-check check-ascii check-locales dev docker shadow clean

help: ## 显示可用目标
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

all: fmt-check vet check-ascii check-locales test build ## 完整本地校验（等价 CI）

build: build-lite build-web ## 交叉编译 lite + neca-web 的全部二进制到 dist/

build-lite: ## 交叉编译 lite CLI（linux/amd64、linux/arm64）
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/neca-linux-amd64 ./cmd/neca
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/neca-linux-arm64 ./cmd/neca
	@echo "[OK]   构建完成：$(DIST)/neca-linux-amd64，$(DIST)/neca-linux-arm64"

build-web: ## 交叉编译 neca-web 的 necad/helper（linux/amd64、linux/arm64）
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/necad-linux-amd64 ./cmd/necad
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/necad-linux-arm64 ./cmd/necad
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/neca-web-helper-linux-amd64 ./cmd/neca-web-helper
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/neca-web-helper-linux-arm64 ./cmd/neca-web-helper
	@echo "[OK]   构建完成：necad 与 neca-web-helper（amd64/arm64）"

build-native: ## 构建当前平台的 CLI/necad 调试二进制到 dist/
	@mkdir -p $(DIST)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/neca ./cmd/neca
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/necad ./cmd/necad

test: ## 运行全部 Go 测试（-race 竞态检测）
	$(GO) test -race ./...

vet: ## go vet 静态检查
	$(GO) vet ./...

fmt: ## 格式化所有 Go 源码
	$(GO) fmt ./...

fmt-check: ## 校验 gofmt 无差异（CI 门禁）
	@diff="$$(gofmt -l .)"; \
	if [ -n "$$diff" ]; then \
		echo "[ERR]  以下文件未格式化，请运行 make fmt："; echo "$$diff"; exit 1; \
	fi
	@echo "[OK]   gofmt 检查通过"

check-ascii: ## 校验平台脚本模板为纯 ASCII（文档 §9 硬规则 1）
	bash scripts/check-ascii.sh templates/win templates/linux templates/macos

check-locales: ## 校验 CLI/Web 内置语言包 key 集合一致（文档 §8.11）
	$(GO) test ./internal/i18n -run TestLocaleKeysAligned -count=1
	$(GO) test ./internal/web -run TestWebLocaleKeysAligned -count=1

dev: ## 构建并进入开发容器（docker compose 解析挂载路径，Windows 同样可用）
	docker compose run --rm dev bash

docker: build ## 构建 lite 与 neca-web 运行镜像（先交叉编译）
	docker build --target lite -f Dockerfile -t ghcr.io/hyhdsl/neca:dev .
	docker build --target web -f Dockerfile -t ghcr.io/hyhdsl/neca-web:dev .

shadow: build ## 新旧实现影子对比（需 bash + openssl + python3）
	NECA_BIN=$(DIST)/neca-linux-amd64 bash scripts/shadow.sh

clean: ## 清理构建产物
	rm -rf $(DIST)
