# Neko's Efficient Certificate Administrator

**README 语言 / Language：** [简体中文（当前）](README.md) · [English](README_EN.md)

> [!NOTE]
> 本文件为**中文主版本**。其他语言版本将另行安排时间统一更新，可能滞后；日常只维护中文版本。英文翻译：[README_EN.md](README_EN.md)。

> [!WARNING]
> **AI 披露（使用前必读）**：本项目的代码、调试、测试、CI/构建/部署脚本、文档和帮助站内容主要由 AI 编程助手完成；人类维护者负责需求、审查、功能验收与生产操作。完整边界见 [AI-DISCLOSURE.md](AI-DISCLOSURE.md)。
>
> **责任限制**：软件按“现状”提供，不提供明示或默示担保；在适用法律允许的范围内，作者与 AI 工具提供方不对使用或误用造成的损失承担责任。
>
> **开发期暂不接受外部 PR**：请先 fork；二次开发须遵守 AGPL 与 [开发政策](docs/SECONDARY-DEVELOPMENT.md)。
>
> 相关文件：[LICENSE](LICENSE) · [NOTICE](NOTICE) · [AI-DISCLOSURE.md](AI-DISCLOSURE.md) · [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)

**Neko's Efficient Certificate Administrator（neca，亦写作 NECA）** 是面向 strongSwan IKEv2 的轻量证书管理工具。一个 Go CLI、一个可选且默认只读的 Web 面板，以及一条受控的 root helper 写边界。

🌐 [主页](https://neca.nekobiglazyer.work) · 🇨🇳 [中文文档](https://neca.nekobiglazyer.work/zh/) · 🇬🇧 [English](https://neca.nekobiglazyer.work/en/) · 📦 [v1.0.1](https://github.com/hyhdsl/neca/releases/tag/v1.0.1)

## ✨ 特性

- 🔐 **安全默认**：Web 默认只读；Web 容器非 root、无 <code>ca.key</code>、无 <code>docker.sock</code>。
- 🧾 **完整生命周期**：issue / reissue / revoke / unrevoke / delete，自动维护 CRL。
- 📦 **平台包**：Windows / Linux / macOS / Android / iOS；可选安装脚本、p12 profile 与 zip 下载。
- 🐳 **部署简单**：单静态二进制；lite 与 neca-web 双镜像，支持 amd64 / arm64。
- ⚙️ **可运维**：JSON 配置、<code>--json</code> / <code>--dry-run</code>、审计日志、SIGHUP 热重载与 requestId 幂等缓存。
- 🧩 **可扩展**：<code>internal/core/ports</code> 提供索引存储与配置来源接口。

## 🚀 快速开始

固定版本标签，先 dry-run，再签发：

~~~bash
docker pull ghcr.io/hyhdsl/neca:v1.0.1
neca version
neca cert list
neca cert issue alice --platforms windows,android --zip --dry-run
neca cert issue alice --platforms windows,android --zip
~~~

生产环境把 <code>/opt/ipsec</code> 挂载给容器，并通过 wrapper 或 compose 运行；完整步骤见 [安装、升级与回滚](https://neca.nekobiglazyer.work/zh/#/install)。

## 🖥️ Web 面板

默认只读；需要写操作时，显式启动 root helper 并关闭只读：

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web up -d web
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web --profile web-write up -d web web-helper
neca web readonly off
~~~

生产配置位于 <code>/opt/ipsec/config/neca.json</code>；Web 通过配置目录挂载感知 SIGHUP 热重载。默认仍建议使用 OpenSSH 本地转发，不直接暴露公网。

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

- **当前稳定版**：v1.0.1（2026-11-23 发布）
- **已完成**：M0–M12（CLI、Web、root helper、M9 命令迁移、M10 安全运维、M11 回收站/设置、M12 可维护性/可观测性）
- **下一步**：M13 公网访问后置，M14 OpenAPI 长期搁置
- **生产环境**：v1.0.0 已部署；v1.0.1 升级随后执行；v0.4.1 回滚路径已验证；RC 版本仅保留 Git tag 与源码

## ⚖️ 许可与致谢

**AGPL-3.0-only**：允许商业使用，但分发或网络服务须按 AGPL 公开对应源码；必须保留原作者、仓库链接与许可。详见 [LICENSE](LICENSE) 与 [二次开发政策](docs/SECONDARY-DEVELOPMENT.md)。

感谢 Go、strongSwan、OpenSSL、Docker 及所有上游开源项目。
