# neca

基于 Go 的 strongSwan IKEv2 用户证书管理工具（CLI，后续含 Web 面板）。

前身是 [legacy/vpn-cert-manager.sh](legacy/vpn-cert-manager.sh)（506 行 bash），
负责 `/opt/ipsec` 下的证书签发、吊销、重签发、平台包生成与 CRL 维护。

- 仓库：https://github.com/hyhdsl/neca （私有）
- 技术路线：Go 1.25，单静态二进制，`CGO_ENABLED=0`
- 运行形态：服务器上以 root 执行，`ca.key` 保持 `0600 root:root`
- 开发环境：本地 Docker（开发机不需要安装 Go，见文档 §14）

## 快速开始

全部构建与测试都在 Docker 开发镜像里完成：

```bash
# 1. 构建开发镜像（首次约 1 分钟；镜像已缓存则秒级）
docker build -f Dockerfile.dev -t neca-dev .

# 2. 容器内执行完整校验（等价 CI：格式 + vet + ASCII + 测试 + 交叉编译）
docker compose run -T --rm dev make all

# 3. 查看版本子命令
docker compose run -T --rm dev ./dist/neca-linux-amd64 version
```

进入交互式开发容器（宿主机需有 make 与 docker）——`make dev`。

## 目录结构

```text
cmd/neca/           CLI 入口（薄壳，仅转发给 internal/cli）
internal/cli/       参数解析、子命令分发、输出渲染
internal/core/      业务核心（M1 起填充；不依赖展示层、不直接打印）
internal/version/   构建期注入的版本元数据
templates/          平台包模板（Windows 模板必须纯 ASCII）
testdata/           黄金文件与 fixture
scripts/            辅助脚本（如 check-ascii）
.github/workflows/  CI（M0 落 ci.yml；release/docker 随 M5 落地）
legacy/             上一代 bash 实现（只读归档，影子测试基准）
dist/               构建产物（不入库）
```

## 开发命令

| 命令 | 作用 |
|---|---|
| `make dev` | 构建并进入开发容器 |
| `make all` | gofmt 检查 + vet + ASCII 校验 + 测试 + 交叉编译 |
| `make build` | 交叉编译 linux/amd64、linux/arm64 到 `dist/` |
| `make test` | 容器内 `go test -race ./...` |
| `make check-ascii` | 校验 Windows 模板为纯 ASCII |
| `make docker` | 构建运行镜像（M5 完善 buildx / 多架构） |

## 里程碑

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 仓库骨架、Makefile、Dockerfile.dev、CI、`neca version` | ✅ 2026-11-15 |
| M1 | 只读命令（list / verify / export / index sync） | 待开始 |
| M2 | 写命令（issue / reissue / revoke / platform / delete） | 待开始 |
| M3 | 向导、JSON、dry-run、审计、退出码 | 待开始 |
| M4 | 热加载模式（已完成实验） | ✅ 2026-11-15 |
| M5 | v0.1.0 发布与服务器切换 | 待开始 |
| M6 | Web 面板 | 待开始 |

## 文档

| 文档 | 内容 |
|---|---|
| [neca-development.md](neca-development.md) | 开发文档：架构、CLI 契约、CI/CD、路线图、实验记录 |
| [vpn-cert-manager-rewrite-assessment.md](vpn-cert-manager-rewrite-assessment.md) | 技术选型评估（Python vs Go） |
| [legacy/vpn-cert-manager.md](legacy/vpn-cert-manager.md) | 旧 bash 脚本的使用与二次开发说明（归档） |

## 现状

- M0 已完成：Go 模块、`neca version`、Makefile、Dockerfile.dev、CI 工作流、ASCII 门禁。
- 2026-11-15 完成 `swanctl --load-creds` 实测：新 CRL 可热替换，吊销不再需要重启容器。
- 旧 bash 实现已移入 `legacy/`，保留作为影子测试基准与回退手段。
