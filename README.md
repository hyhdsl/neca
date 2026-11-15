# neca

> [!WARNING]
> **重要声明（使用前必读）**
>
> 1. **AI 披露**：本项目的代码、测试与文档由 AI 编程助手（DeepSeek Harness / DeepSeek 模型）辅助生成与修改。AI 输出可能存在错误、遗漏或不适用，使用前请自行审查、测试并自行承担风险。
> 2. **责任限制**：在适用法律允许的最大范围内，原作者、AI 模型/工具提供方及其所属公司不对因使用、无法使用或误用本软件造成的任何直接、间接、偶然、特殊或后果性损失（包括但不限于数据丢失、业务中断、证书/密钥泄露、VPN 不可用）承担责任。本软件按“现状”提供，不提供任何明示或默示担保。
> 3. **暂不接受 PR**：本项目当前不接受 Pull Request；如需修改请 fork，并遵守 [SECONDARY-DEVELOPMENT.md](SECONDARY-DEVELOPMENT.md)。
>
> 完整授权与二次开发要求见 [LICENSE](LICENSE)、[NOTICE](NOTICE) 与 [SECONDARY-DEVELOPMENT.md](SECONDARY-DEVELOPMENT.md)。

基于 Go 的 strongSwan IKEv2 用户证书管理工具（CLI，后续含 Web 面板）。

前身是 [legacy/vpn-cert-manager.sh](legacy/vpn-cert-manager.sh)（506 行 bash），
负责 `/opt/ipsec` 下的证书签发、吊销、重签发、平台包生成与 CRL 维护。

- 仓库：https://github.com/hyhdsl/neca （私有；首个正式 Release 后转 public）
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

# 3. 新旧实现影子对比（旧 bash vs 新 neca，diff 必须为 0）
docker compose run -T --rm dev make shadow

# 4. 查看版本与命令列表
docker compose run -T --rm dev ./dist/neca-linux-amd64 version
docker compose run -T --rm dev ./dist/neca-linux-amd64 help
```

进入交互式开发容器（宿主机需有 make 与 docker）——`make dev`。

## 命令（M1）

| 命令 | 作用 |
|---|---|
| `neca list` | 列出证书（状态/序列号/平台/到期） |
| `neca verify [CN…]` | 检查客户端 p12（3DES、证书数、密码长度） |
| `neca export issued\|revoked [--out F]` | 导出 CSV（默认写到 EXPORT_DIR） |
| `neca index sync` | 为证书补索引与元数据 |
| `neca version` / `-V` | 版本信息 |

所有命令都遵循 `[OK]/[INFO]/[WARN]/[ERR]` 前缀与 0/1/2/3 退出码契约（开发文档 §6.3）。

## 规划中的命令（尚未实现）

以下命令按里程碑落地，设计细节见开发文档 §6 与 §9：

| 命令/开关 | 里程碑 | 作用 |
|---|---|---|
| `neca package <CN> [--platforms] [--installers] [--p12-profile] [--zip]` | M2 | 从现有证书重新生成平台包，可选打包 zip |
| `neca downloads list` / `neca downloads clean [--expired\|--all]` | M2 | 查看/清理下载目录中的 zip（默认 30 分钟过期） |
| `neca issue/reissue ... --installers ... --p12-profile ... --zip` | M2 | 签发/重签时选择一键脚本、p12 算法并直接打包 |
| `--lang zh-CN\|en-US` / `NECA_LANG` | M3 | 控制台语言切换（语言文件由社区扩展） |

平台包扩展为 Windows / Linux / macOS / Android / iOS；一键安装脚本默认全部关闭，需显式开启。

## 目录结构

```text
cmd/neca/           CLI 入口（薄壳，仅转发给 internal/cli）
internal/cli/       参数解析、子命令注册表、输出渲染
internal/core/      业务核心（M1 已落地只读命令；不依赖展示层、不直接打印）
internal/core/ports/ 扩展点接口（IndexStore / 配置来源）
internal/version/   构建期注入的版本元数据
templates/          平台包模板（Windows 模板必须纯 ASCII）
testdata/           黄金文件与 fixture 生成器（gen-fixture.sh）
scripts/            辅助脚本（check-ascii、shadow）
.github/workflows/  CI（M0 落 ci.yml；release/docker 随 M5 落地）
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
| `make all` | gofmt 检查 + vet + ASCII 校验 + 测试 + 交叉编译 |
| `make build` | 交叉编译 linux/amd64、linux/arm64 到 `dist/` |
| `make test` | 容器内 `go test -race ./...` |
| `make check-ascii` | 校验 Windows 模板为纯 ASCII |
| `make shadow` | 新旧实现影子对比（bash + openssl + python3，diff 必须为 0） |
| `make docker` | 构建运行镜像（M5 完善 buildx / 多架构） |

## 里程碑

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M0 | 仓库骨架、Makefile、Dockerfile.dev、CI、`neca version` | ✅ 2026-11-15 |
| M1 | 只读命令（list / verify / export / index sync） | ✅ 2026-11-15 |
| M2 | 写命令、Linux/macOS 平台包、安装脚本选项、p12 算法 profile、zip 打包与清理 | 待开始 |
| M3 | 向导、JSON、dry-run、审计、退出码、多语言语言包 | 待开始 |
| M4 | 热加载模式（已完成实验） | ✅ 2026-11-15 |
| M5 | v0.1.0 发布、服务器切换、首个正式 Release 后仓库转 public | 待开始 |
| M6 | Web 面板（含 30 分钟下载有效期与 Web 多语言） | 待开始 |

## 许可与二次开发

- **许可**：PolyForm Noncommercial 1.0.0（源码可见、禁止商业用途；不是 OSI 定义的开源协议），全文见 [LICENSE](LICENSE)。
- **二次开发**：必须保留原作者与仓库链接、公开完整源码、继续采用非商业许可，且不得用于商业用途；详见 [SECONDARY-DEVELOPMENT.md](SECONDARY-DEVELOPMENT.md)。
- **仓库状态**：当前为私有开发期；首个正式 Release（v0.1.0）后转为 public。当前**不接受 PR**，问题请通过 Issue 反馈。

## 文档

| 文档 | 内容 |
|---|---|
| [neca-development.md](neca-development.md) | 开发文档：架构、CLI 契约、CI/CD、路线图、实验记录 |
| [vpn-cert-manager-rewrite-assessment.md](vpn-cert-manager-rewrite-assessment.md) | 技术选型评估（Python vs Go） |
| [legacy/vpn-cert-manager.md](legacy/vpn-cert-manager.md) | 旧 bash 脚本的使用与二次开发说明（归档） |
| [LICENSE](LICENSE) | PolyForm Noncommercial 1.0.0 许可全文 |
| [NOTICE](NOTICE) | 版权、AI 披露与署名 |
| [SECONDARY-DEVELOPMENT.md](SECONDARY-DEVELOPMENT.md) | 二次开发、开源与非商业要求 |

## 现状

- M1 已完成：list / verify / export / index sync；影子对比（旧 bash vs 新 neca）diff = 0。
- M0 已完成：Go 模块、`neca version`、Makefile、Dockerfile.dev、CI 工作流、ASCII 门禁。
- 扩展点契约已落地：`IndexStore`、配置来源接口与 CLI 命令注册表（无行为变更）。
- 2026-11-15 完成 `swanctl --load-creds` 实测：新 CRL 可热替换，吊销不再需要重启容器。
- 旧 bash 实现已移入 `legacy/`，保留作为影子测试基准与回退手段。
- 仓库内服务器标识均为占位值，生产值通过环境变量注入（详见下文「敏感信息」）。
