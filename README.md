# Neko's Efficient Certificate Administrator

**README 语言 / Language：** [简体中文（当前）](README.md) · [繁體中文](README_zh-TW.md) · [English](README_EN.md)

> [!NOTE]
> 本文件为**中文主版本**。其他语言版本将另行安排时间统一更新，可能滞后；日常只维护中文版本。英文翻译：[README_EN.md](README_EN.md)。

> [!WARNING]
> **AI 披露（使用前必读）**：本项目的代码、调试、测试、CI/构建/部署脚本、文档和帮助站内容主要由 AI 编程助手完成；人类维护者负责需求、审查、功能验收与生产操作。完整边界见 [AI-DISCLOSURE.md](AI-DISCLOSURE.md)。
>
> **责任限制**：软件按“现状”提供，不提供明示或默示担保；在适用法律允许的范围内，作者与 AI 工具提供方不对使用或误用造成的损失承担责任。
>
> **安全边界**：安全性测试尚不完整，不建议用于商业用途或高风险生产环境；如使用，请自行完成独立安全评审、备份与回滚验证。
>
> **贡献与反馈**：请先通过 Issue 说明问题或需求（[选择 Bug / Feature / Question 模板](https://github.com/hyhdsl/neca/issues/new/choose)）；提交 PR 前请阅读 [开发政策](docs/SECONDARY-DEVELOPMENT.md)。二次开发须遵守 AGPL。
>
> 相关文件：[LICENSE](LICENSE) · [NOTICE](NOTICE) · [AI-DISCLOSURE.md](AI-DISCLOSURE.md) · [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)

**Neko's Efficient Certificate Administrator（neca，亦写作 NECA）** 是面向 strongSwan IKEv2 的轻量证书管理工具。一个 Go CLI、一个可选且默认只读的 Web 面板，以及一条受控的 root helper 写边界。

🌐 [主页](https://neca.nekobiglazyer.work) · 🇨🇳 [中文文档](https://neca.nekobiglazyer.work/zh/) · 🇬🇧 [English](https://neca.nekobiglazyer.work/en/) · 📦 [v1.5.0](https://github.com/hyhdsl/neca/releases/tag/v1.5.0)

## ✨ 特性

- 🔐 **安全默认**：Web 默认只读；Web 容器非 root、无 <code>ca.key</code>、无 <code>docker.sock</code>。
- 🧾 **完整生命周期**：issue / reissue / revoke / unrevoke / delete，自动维护 CRL。
- 📦 **平台包**：Windows / Linux / macOS / Android / iOS；可选安装脚本、p12 profile 与 zip 下载。
- 🐳 **部署简单**：单静态二进制；lite 与 neca-web 双镜像，支持 amd64 / arm64。
- ⏳ **到期概览**：CLI 与 Web 提供 30/60/90 天只读到期视图，不修改证书或配置。
- ⚙️ **可运维**：JSON 配置、<code>--json</code> / <code>--dry-run</code>、审计日志、SIGHUP 热重载与 requestId 幂等缓存。
- ⌨️ **Shell completion**：bash / zsh / fish / PowerShell 补全命令、子命令与主要开关。
- 🔗 **证书链校验**：<code>neca cert verify</code> 使用 CA 校验证书链；链失败只告警，不改变退出码契约。
- 📥 **批量导入**：按 CN 目录批量导入 p12 或 cert+key；可选 <code>entry.json</code> 覆盖，单条失败不影响其他成功项。
- 🧩 **可扩展**：<code>internal/core/ports</code> 提供索引存储与配置来源接口。

## 🚀 快速开始

固定版本标签，先 dry-run，再签发：

~~~bash
docker pull ghcr.io/hyhdsl/neca:v1.5.0
neca version
neca cert list
neca cert issue alice --platforms windows,android --zip --dry-run
neca cert issue alice --platforms windows,android --zip

# 批量导入（建议先 dry-run）
neca cert import-batch ./batch --password-dir ./secrets --dry-run
~~~

部署时把 <code>/opt/ipsec</code> 挂载给容器，并通过 wrapper 或 compose 运行；完整步骤见 [安装、升级与回滚](https://neca.nekobiglazyer.work/zh/#/install)。

## 🔐 验证发布签名

v1.5.0 起，正式 Release 提供 GPG 签名的 <code>checksums.txt.asc</code> 与公钥 <code>neca-gpg-key.asc</code>；Git tag 也已签名。下载对应版本后验证：

~~~bash
gh release download v1.5.0 --repo hyhdsl/neca --pattern checksums.txt --pattern checksums.txt.asc --pattern neca-gpg-key.asc
gpg --import neca-gpg-key.asc
gpg --verify checksums.txt.asc checksums.txt
sha256sum -c checksums.txt
git fetch --tags && git tag -v v1.5.0
~~~

当前签名使用既有 neca GPG 密钥；专用发布签名子密钥与 CI 自动化在 v1.5.1 规划中。

## 🧮 运行资源基线

> 以下为 2026-11-28 在 Linux amd64、2 vCPU、1.6 GiB 内存环境下的低负载实测与安全余量；CLI 与 Web 分开给出，不包含宿主 Docker、strongSwan、反向代理或高并发/大证书库的额外开销。

| 组件 | 最低（低负载余量） | 建议 |
|---|---|---|
| `neca` CLI | 1 vCPU / 64 MiB 内存 / 256 MiB 可用磁盘 | 1 vCPU / 256 MiB / 1 GiB |
| `neca-web` + helper | 1 vCPU / 256 MiB / 512 MiB 可用磁盘 | 2 vCPU / 512 MiB / 2 GiB |

实测参考：CLI 在 16 MiB 容器限制下可执行 `version`，常驻约 2.8 MiB；Web/helper 常驻合计约 15.5 MiB，healthcheck 为 healthy；两个镜像分别约 126.5 MiB 与 135.7 MiB。主机 Docker、OpenSSL 与浏览器前置按安装页执行；未单独列出的宿主细节以安装页验证通过、可以正常工作为准。

## 🖥️ Web 面板

默认只读；需要写操作时，显式启动 root helper 并关闭只读：

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web up -d web
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web --profile web-write up -d web web-helper
neca web readonly off
~~~

部署配置位于 <code>/opt/ipsec/config/neca.json</code>；Web 通过配置目录挂载感知 SIGHUP 热重载。默认仍建议使用 OpenSSH 本地转发，不直接暴露公网。

## 📚 文档

| 入口 | 内容 |
|---|---|
| 🌐 [主页](https://neca.nekobiglazyer.work) | 项目介绍与快速入口 |
| 🇨🇳 [中文帮助](https://neca.nekobiglazyer.work/zh/) | 安装、证书、Web、API、安全、FAQ |
| ⌨️ [常用命令速查](https://neca.nekobiglazyer.work/zh/#/cheatsheet) | 复制即可使用的常用命令 |
| ⌨️ [子命令参考](https://neca.nekobiglazyer.work/zh/#/commands) | 全局开关、全部子命令、参数、确认与退出码 |
| 🇬🇧 [English](https://neca.nekobiglazyer.work/en/) | English core pages |
| 🧪 [Web API](https://neca.nekobiglazyer.work/zh/#/api) | 实验性接口说明 |
| 🐳 [安装、升级与回滚](https://neca.nekobiglazyer.work/zh/#/install) | Docker/二进制安装、升级、回滚 |
| 🛠️ [开发、许可与反馈](https://neca.nekobiglazyer.work/zh/#/development) | 本地构建、架构、发布流程、贡献 |
| 🤖 [AI 披露](AI-DISCLOSURE.md) | AI 参与范围与验收边界 |

## 🛠️ 开发

~~~bash
docker compose run -T --rm dev make all      # 格式 + vet + 文档 + 测试 + 交叉编译
docker compose run -T --rm dev make shadow   # 新旧实现影子对比
docker compose run -T --rm dev make test     # go test -race ./...
~~~

常用检查：<code>make check-docs</code>、<code>make check-locales</code>、<code>make check-ascii</code>。详细目标与架构说明见 [帮助站开发页](https://neca.nekobiglazyer.work/zh/#/development)。

## 📌 状态

- **当前稳定版**：v1.5.0（2026-11-28 发布）
- **已完成**：M0–M12；v1.4 cert+key 导入、事务标记/恢复、Node 24 Actions 与运行资源基线
- **后续**：批量导入；v1.5.0 配置 schema 与 Passkey 评估
- **边界**：Passkey 仅有配置和接口预留，未注册登录路由；Web API 为实验性接口

## ⚖️ 许可与致谢

**AGPL-3.0-only**：允许商业使用，但分发或网络服务须按 AGPL 公开对应源码；必须保留原作者、仓库链接与许可。详见 [LICENSE](LICENSE) 与 [二次开发政策](docs/SECONDARY-DEVELOPMENT.md)。

本项目建立在大量上游工作之上，特别感谢：

- **Go** 标准库与工具链，以及 `golang.org/x` 系列依赖；
- **strongSwan** 与 IKEv2 社区，为证书、CRL 与 VPN 配置提供实践基础；
- **OpenSSL**，承担证书、密钥与 PKCS#12 的核心操作；
- **Docker / Buildx / QEMU**，支持多架构镜像与可复现的开发环境；
- **GitHub Actions**，提供 CI、发布与镜像流水线；
- **Docsify** 与帮助站相关工具，以及所有提交 Issue、测试和建议的使用者。

上游项目的许可证与版权归其各自作者所有；本项目仅保留必要的引用与许可声明。
