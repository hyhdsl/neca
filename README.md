# neca

> [!WARNING]
> **重要声明（使用前必读）**
>
> 1. **AI 披露（重要）**：本项目的**代码编写、调试、测试、CI/构建脚本、部署脚本、文档和帮助站内容均由 AI 编程助手（DeepSeek Harness / DeepSeek 模型）完成**；人类维护者主要负责需求、审查、功能验收与生产操作。完整范围、边界与限制见 [AI-DISCLOSURE.md](AI-DISCLOSURE.md)。AI 输出可能存在错误、遗漏或不适用，使用前请自行审查、测试并自行承担风险。
> 2. **责任限制**：在适用法律允许的最大范围内，原作者、AI 模型/工具提供方及其所属公司不对因使用、无法使用或误用本软件造成的任何直接、间接、偶然、特殊或后果性损失（包括但不限于数据丢失、业务中断、证书/密钥泄露、VPN 不可用）承担责任。本软件按“现状”提供，不提供任何明示或默示担保。
> 3. **开发期暂不接受外部 PR**：项目尚未完全开发完毕，暂不接受外部 Pull Request，仅维护者内部合并；开发完成后会重新评估并开放 PR。如需修改请先 fork，并遵守 AGPL 与 [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)。
>
> 完整授权与二次开发要求见 [LICENSE](LICENSE)、[NOTICE](NOTICE) 与 [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)。

基于 Go 的 strongSwan IKEv2 用户证书管理工具（CLI + 可选 Web 面板；默认只读，root helper 受控开放写能力）。

前身是 [legacy/vpn-cert-manager.sh](legacy/vpn-cert-manager.sh)（506 行 bash），
负责 `/opt/ipsec` 下的证书签发、吊销、重签发、平台包生成与 CRL 维护。

- 仓库：https://github.com/hyhdsl/neca （public；v0.4.0 正式版）
- 技术路线：Go 1.25，单静态二进制，`CGO_ENABLED=0`
- 运行形态：服务器上以 root 执行，`ca.key` 保持 `0600 root:root`
- 开发环境：本地 Docker（开发机不需要安装 Go，见文档 §14）

> **使用文档**：<https://neca.nekobiglazyer.work>（中文全量 + 英文核心页，含实验性 Web API）。

## 快速开始

全部构建与测试都在 Docker 开发镜像里完成：

```bash
# 1. 构建开发镜像（首次约 1 分钟；镜像已缓存则秒级）
docker build -f Dockerfile.dev -t neca-dev .

# 2. 容器内执行完整校验（等价 CI：格式 + vet + ASCII + 测试 + 交叉编译）
docker compose run -T --rm dev make all

# 3. 新旧实现影子对比（旧 bash vs 新 neca，diff 必须为 0）
docker compose run -T --rm dev make shadow

# 4. 查看版本与命令列表
docker compose run -T --rm dev ./dist/neca-linux-amd64 version
docker compose run -T --rm dev ./dist/neca-linux-amd64 help
```

进入交互式开发容器（宿主机需有 make 与 docker）——`make dev`。

## 命令（M1–M6）

| 命令 | 作用 |
|---|---|
| `neca list` | 列出证书（状态/序列号/平台/到期） |
| `neca verify [CN…]` | 检查客户端 p12（3DES、证书数、密码长度） |
| `neca export issued\|revoked [--out F]` | 导出 CSV（默认写到 EXPORT_DIR） |
| `neca index sync` | 为证书补索引与元数据 |
| `neca issue <CN> [--platforms ...] [--days N] [--installers ...] [--p12-profile ...] [--zip]` | 签发证书与平台包 |
| `neca reissue <CN> ...` | 归档旧证书并重签 |
| `neca revoke <CN>` / `neca unrevoke <CN>` | 吊销 / 恢复吊销（自动更新 CRL） |
| `neca delete <CN> --yes [--purge-revoked]` | 删除证书与客户端包 |
| `neca platform add\|set <CN> --platforms ...` | 补平台包 / 改平台标注 |
| `neca package <CN> [--platforms ...] [--zip]` | 从现有证书重建平台包 |
| `neca downloads list` / `clean [--expired\|--all]` | 管理下载 zip（默认 30 分钟过期） |
| `neca config init\|show\|validate\|export-env` | 管理 `/opt/ipsec/neca.json` 设置 |
| `neca web status` | 查看 Web 配置、只读模式、空闲超时与 helper 状态 |
| `neca web readonly status\|on\|off` | 只读开关；`off` 需要运行中的 root helper |
| `neca web idle-timeout status\|<duration>` | 查看/设置空闲超时（5m–24h，绝对上限 24h） |
| `neca web hash-password` | 从 stdin 生成 bcrypt 哈希（用于 `web-secrets/password.hash`） |
| `neca`（无参数）/ `neca wizard` | 进入交互向导（菜单 0–10） |
| `neca <命令> --json` | 结构化 JSON 输出 |
| `neca <写命令> --dry-run` | 只打印将执行的动作，不落盘 |
| `neca --lang en-US <命令>` | 英文控制台（命令输出/帮助） |
| `neca version` / `-V` | 版本信息 |

所有命令都遵循 `[OK]/[INFO]/[WARN]/[ERR]` 前缀与 0/1/2/3 退出码契约（开发文档 §6.3）。

## Web 面板（M6）

M6 提供可选 Web 面板：默认只读，登录、证书列表/详情、p12 检查、受保护下载与
深浅色 UI；启用 root helper 后可显式开放 issue/revoke/unrevoke/reissue 写能力。
Web 以独立 `neca-web` 镜像发布，默认 **lite** 镜像不含 Web 服务端。

```bash
# 默认只跑 CLI
neca list

# 可选只读 Web（服务端，默认）
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web up -d web

# M6b：加 root helper 后可显式开启写能力
mkdir -p /run/neca
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web --profile web-write up -d web web-helper
neca web readonly off
```

Web 默认监听 `127.0.0.1:18080`；本次生产部署因 18080 已被 other service 占用，使用
`127.0.0.1:18080`。通过系统原生 OpenSSH 本地转发或反向代理访问；不引入
Tailscale/WireGuard 等第三方 VPN。登录、会话与权限细节见
[docs/web-panel-development.md](docs/web-panel-development.md)，部署步骤见
[docs/deploy.md](docs/deploy.md)。

M6b 增加独立 root helper 与 issue/revoke/unrevoke/reissue 写能力。Web 容器
始终非 root；只有 helper 容器挂载 CA 私钥与 docker.sock。默认 `readOnly=true`，
root 执行 `neca web readonly off` 且 helper 健康后才开放写操作。

平台包支持 Windows / Linux / macOS / Android / iOS；Linux/macOS 安装/卸载脚本已实现，但仍默认关闭，需 `--installers` 显式开启。

## Docker 部署

服务器使用独立 CLI 容器，镜像内包含 openssl 与静态 docker CLI；容器挂载
`/opt/ipsec` 和 `/var/run/docker.sock`，通过 `swanctl --load-creds` 热加载 CRL：

```bash
make build
# 默认 lite 镜像
docker build --target lite -t ghcr.io/hyhdsl/neca:v0.4.0 .
# 可选 Web 面板包（默认只读）
docker build --target web -t ghcr.io/hyhdsl/neca-web:v0.4.0 .
install -m 0755 deploy/neca-wrapper.sh /usr/local/bin/neca
neca list
```

Web 面板作为可选的 **neca-web** 面板包提供；默认 **lite** 只含 CLI，不发布 full 镜像。lite + neca-web 共享同一 core 与数据卷，按 compose profile 选择组件；设计见 [docs/web-panel.md](docs/web-panel.md) 第 11 节，实施规格见 [docs/web-panel-development.md](docs/web-panel-development.md)。

业务设置放在服务器 `/opt/ipsec/neca.json`。默认 lite 使用 0600；启用 Web 时改为
`root:neca-web 0640`，否则非 root 的 necad 无法读取。`neca.env` 只保留
`NECA_CONFIG=/opt/ipsec/neca.json` 作为 bootstrap；真实 `SERVER_ID` / `ORG` /
`CA_CN` 不提交仓库。优先级：**CLI flag > 环境变量 > neca.json > 默认值**。
完整步骤、备份/回滚和 docker.sock 安全说明见 [docs/deploy.md](docs/deploy.md)。

```bash
neca config show           # 查看有效设置
neca config validate       # 校验 JSON + PKI
neca config init --from-env  # 从现有环境变量生成 neca.json
```

语言优先级：**`--lang` > `NECA_LANG` > `neca.json` 的 `language` > `LANG/LC_ALL` > `zh-CN`**。

持久设置写入 `neca.json`：

```json
{ "language": "en-US" }
```

```bash
neca config validate
neca list
```

临时覆盖：

```bash
neca --lang en-US list
# 当前 shell 会话内覆盖（包装命令会转发给容器）
export NECA_LANG=en-US
neca list
```

## 目录结构

```text
cmd/neca/           CLI 入口（薄壳，仅转发给 internal/cli）
cmd/necad/          Web 服务端入口（仅 neca-web 镜像）
cmd/neca-web-helper/ M6b root helper（Unix socket 白名单写边界）
internal/cli/       参数解析、子命令注册表、输出渲染
internal/core/      业务核心（M1 只读 + M2 写命令 + M3 audit；不依赖展示层）
internal/web/       Go 模板、会话、只读 API、静态资源与 Web 配置
internal/core/ports/ 扩展点接口（IndexStore / 配置来源）
internal/i18n/      内置 zh-CN/en-US 语言文件与翻译
templates/          平台包模板（Windows/Linux/macOS 脚本必须纯 ASCII）
testdata/           黄金文件与 fixture 生成器（gen-fixture.sh）
scripts/            辅助脚本（check-ascii、shadow）
.github/workflows/  CI、release 与 GHCR 镜像工作流
docs/              项目文档、Web 面板评估与部署说明
deploy/            服务器 Docker 部署文件
legacy/             上一代 bash 实现（只读归档，影子测试基准）
dist/               构建产物（不入库）
```

## 扩展点

可能被替换或新增的能力以接口形式集中在 `internal/core/ports`（当前：索引存储 `IndexStore`、配置来源 `Source`/`Resolver`）；CLI 子命令通过 `internal/cli` 的注册表挂载。设计原则、候选扩展点与新增步骤见开发文档 §3.1。

## 敏感信息

仓库内的服务器 IP、VPN 内网段、真实域名、组织与 CA 名称均为占位值：`<SERVER_IP>`、`<VPN_SUBNET>`、`vpn.example.com`、`ExampleOrg`。
生产部署通过环境变量注入真实的 `SERVER_ID`、`ORG`、`CA_CN`、`PKI_DIR` 等值，不要把真实服务器标识写回仓库。

## 开发命令

| 命令 | 作用 |
|---|---|
| `make dev` | 构建并进入开发容器 |
| `make all` | gofmt 检查 + vet + ASCII 校验 + 文档检查 + 测试 + 交叉编译 |
| `make build` | 交叉编译 CLI + necad/helper（linux/amd64、linux/arm64）到 `dist/` |
| `make test` | 容器内 `go test -race ./...` |
| `make check-ascii` | 校验 Windows/Linux/macOS 脚本模板为纯 ASCII |
| `make shadow` | 新旧实现影子对比（只读 + M2 写命令 + issue，diff 必须为 0） |
| `make check-locales` | 校验 zh-CN/en-US 语言包 key 一致 |
| `make check-docs` | 校验 Markdown 链接、代码围栏与帮助站结构 |
| `make publish-help` | 生成 `docs/help` → `neca-help` 发布差异（dry-run） |
| `make publish-help-apply` | 推送 `docs/help` 到 `neca-help`（需先提交） |
| `make docker` | 构建 lite 与 neca-web 运行镜像（本地 `:dev`） |

## 里程碑

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 仓库骨架、Makefile、Dockerfile.dev、CI、`neca version` | ✅ 2026-11-15 |
| M1 | 只读命令（list / verify / export / index sync） | ✅ 2026-11-15 |
| M2 | 写命令、Linux/macOS 平台包、安装脚本选项、p12 算法 profile、zip 打包与清理 | ✅ 2026-11-15 |
| M3 | 向导、JSON、dry-run、审计、退出码、多语言语言包 | ✅ 2026-11-15 |
| M4 | 热加载模式（已完成实验） | ✅ 2026-11-15 |
| M5 | v0.1.0 发布、服务器切换、首个正式 Release 后仓库转 public | ✅ 2026-11-15（v0.1.0 → v0.1.1 语言补丁 → v0.2.0 JSON 配置，仓库已 public） |
| M6a | 只读 Web 面板、会话/空闲超时、Web 配置 CLI、Passkey 接口占位、lite + neca-web 双镜像 | ✅ 2026-11-15 v0.3.0 正式发布 |
| M6b | root helper、issue/revoke/unrevoke/reissue 写能力、Web 写 API/UI | ✅ 2026-11-15 v0.3.0 正式发布；Web 默认只读 |
| M7 | 文档与发布自动化：`check-docs`、CI 门禁、`docs/help` → `neca-help` 本地一键发布 | ✅ 2026-11-15：`make check-docs`、`make publish-help` / `publish-help-apply`；本地脚本发布验证；Actions 自动同步未采用 |
| M8 | Web 运维加固：SIGHUP 热重载、helper 幂等缓存持久化、访问架构评估 | ✅ 2026-11-15 v0.4.0 正式发布并部署；readOnly/idleTimeout 热重载、cache 落盘 `/run/neca`、继续 OpenSSH |

> **预览包策略**：正式版发布后，历史预览只保留 GitHub 自动源代码归档；使用
> [scripts/archive-preview-release.sh](scripts/archive-preview-release.sh) 删除旧资产。
> **Web 多语言**：`internal/web/locales/` 提供 zh-CN/en-US 翻译文件；页面支持
> 侧栏/登录页语言切换（`neca_lang` Cookie），默认跟随 `neca.json` 的 `language`。

## 许可与二次开发

- **许可**：**AGPL-3.0-only**（OSI 认可的开源协议；允许商业使用，但分发/网络服务须按 AGPL 公开对应源码），全文见 [LICENSE](LICENSE)。
- **二次开发**：必须保留原作者与仓库链接、保留 AGPL 许可与源码公开义务；详见 [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)。
- **仓库状态**：已于 2026-11-15 随 [v0.1.0](https://github.com/hyhdsl/neca/releases/tag/v0.1.0) 转为 **public**；正式 Release 为 [v0.4.0](https://github.com/hyhdsl/neca/releases/tag/v0.4.0)。M6 历史预览包（preview.1–preview.3）仅保留源代码归档。**开发期暂不接受外部 PR**，仅维护者内部合并；完成后重新评估开放。

## 帮助文档

- **中文帮助中心（全量）**：<https://neca.nekobiglazyer.work>
- **English core pages**：<https://neca.nekobiglazyer.work/en/>
- **文档源码**：[docs/help/README.md](docs/help/README.md)
- **实验性 Web API**：[API 说明](https://neca.nekobiglazyer.work/#/api)

> 使用或部署前先看帮助中心；开发、设计和历史评估仍见下列文档索引。

## 文档索引

完整索引见 [docs/README.md](docs/README.md)。

| 文档 | 内容 |
|---|---|
| [docs/neca-development.md](docs/neca-development.md) | 开发文档：架构、CLI 契约、CI/CD、路线图、验收记录 |
| [docs/web-panel.md](docs/web-panel.md) | Web 面板设计评估：只读开关、权限、VPN/mTLS、UI、SQLite 迁移 |
| [docs/web-panel-development.md](docs/web-panel-development.md) | M6 Web 面板开发规格：组件、CLI、会话、权限、Docker 与验收 |
| [docs/vpn-cert-manager-rewrite-assessment.md](docs/vpn-cert-manager-rewrite-assessment.md) | 技术选型评估（Python vs Go） |
| [docs/legacy-vpn-cert-manager.md](docs/legacy-vpn-cert-manager.md) | 旧 bash 脚本的使用与二次开发说明（归档） |
| [docs/deploy.md](docs/deploy.md) | Docker 服务器部署、备份/回滚与安全说明 |
| [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md) | AGPL 义务、署名与贡献政策（开发期暂不接受外部 PR） |
| [LICENSE](LICENSE) | GNU AGPL-3.0-only 许可全文 |
| [NOTICE](NOTICE) | 版权、AI 披露与署名 |
| [AI-DISCLOSURE.md](AI-DISCLOSURE.md) | AI 参与范围、人类职责、验收边界与工具说明 |

## 现状

- **v0.4.0（M8）已完成**：v0.4.0 正式发布；SIGHUP 热重载 readOnly/idleTimeout；helper 幂等缓存持久化；继续 OpenSSH 访问。
- **v0.3.0（M6）已完成**：[v0.3.0](https://github.com/hyhdsl/neca/releases/tag/v0.3.0) 正式发布；lite + neca-web + root helper 已部署，Web 默认只读；Playwright/axe、zh-CN/en-US、深浅色与移动端验收通过；用户帮助见 [帮助中心](https://neca.nekobiglazyer.work)。
- M5 已完成：[v0.1.0](https://github.com/hyhdsl/neca/releases/tag/v0.1.0) 已发布并转 public；[v0.1.1](https://github.com/hyhdsl/neca/releases/tag/v0.1.1) 修复 `--lang`；[v0.2.0](https://github.com/hyhdsl/neca/releases/tag/v0.2.0) 引入 JSON 配置。Docker CLI 镜像已部署到服务器；全功能测试通过；生产索引与 CRL 已恢复到测试前状态。
- M3 已完成：`--json` / `--dry-run` / `--lang`、交互向导、审计日志、旧 flag 兼容、zh-CN/en-US 语言文件与 `make check-locales`。
- M2 已完成：issue / reissue / revoke / unrevoke / delete / platform / package / downloads；Linux/macOS 平台包、p12 profile、zip TTL 与安装脚本。
- M1 已完成：list / verify / export / index sync；影子对比（旧 bash vs 新 neca）diff = 0。
- M0 已完成：Go 模块、`neca version`、Makefile、Dockerfile.dev、CI 工作流、ASCII 门禁。
- 扩展点契约已落地：`IndexStore`、配置来源接口与 CLI 命令注册表（无行为变更）。
- 2026-11-15 完成 `swanctl --load-creds` 实测：新 CRL 可热替换，吊销不再需要重启容器。
- 旧 bash 实现已移入 `legacy/`，保留作为影子测试基准与回退手段。
- 仓库内服务器标识均为占位值，生产值通过环境变量/服务器侧 `neca.json` 注入（详见上文「敏感信息」）。

## 致谢与第三方组件

neca 的实现和运行依赖以下开源项目/发行版组件，感谢维护者与社区：

| 组件 | 用途 | 许可证 |
|---|---|---|
| [Go 标准库](https://go.dev/) | CLI、HTTP、加密、归档与模板 | BSD-3-Clause |
| [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) | 中文表格显示宽度计算 | BSD-3-Clause |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | Web 管理员密码 bcrypt 哈希 | BSD-3-Clause |
| [OpenSSL](https://www.openssl.org/) | CA/证书/p12/CRL 操作（子进程调用） | Apache-2.0 |
| [Debian bookworm-slim](https://www.debian.org/) | 运行镜像基础系统 | Debian 各组件许可 |
| [Docker CLI](https://www.docker.com/) | 容器内通过 docker.sock 触发 CRL 热加载 | Apache-2.0 |
| [ca-certificates](https://salsa.debian.org/debian/ca-certificates) | 运行镜像 CA 根证书 | MPL-2.0（Mozilla CA bundle） |
| [strongSwan](https://www.strongswan.org/) | 外部 IKEv2 服务端，neca 不包含其代码 | GPL-2.0 |
| [GitHub Actions](https://github.com/actions) / [Docker Buildx](https://github.com/docker/buildx) | CI、发布与多架构镜像构建 | MIT / Apache-2.0（各组件不同） |

以上为主要组件；完整许可文本以各项目发布为准。neca 自身以 **AGPL-3.0-only** 发布，详见 [LICENSE](LICENSE)。测试使用的 bash、openssl、python3 等工具不随发布二进制分发。
