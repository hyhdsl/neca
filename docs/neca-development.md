# neca 开发文档（v0.1 规划稿）

> 项目代号：**neca**（CA 管理工具；品牌、域名与服务器标识在仓库中一律使用占位值）
> 文档版本：2026-11-15 第 4 稿（AGPL-3.0-only、开发期暂不接受外部 PR、Linux/macOS 安装/卸载脚本、M3：JSON/dry-run/向导/语言/审计）
> 上游评估：[vpn-cert-manager-rewrite-assessment.md](vpn-cert-manager-rewrite-assessment.md)
> 服务端现状：Debian 12 / strongSwan 5.9.8 / OpenSSL 3.0.15 / `/opt/ipsec`（详见评估报告 §2）

---

## 0. 决策记录（本稿锁定）

评估报告推荐的是 Python，本稿按你的最终决定改为 **Go 一步到位**。以下 13 条为本项目已确认的决策，后续所有设计以此为准：

| # | 决策 | 说明 |
|---|---|---|
| D1 | 语言：**Go** | 一步到位：CLI 与未来 Web 面板共用同一套 core，单静态二进制部署 |
| D2 | 软件名：**neca** | 命令 `neca`；模块路径 `github.com/hyhdsl/neca`（仓库已建） |
| D3 | 旧 CLI 双轨 | 旧 `--issue` 等 flag 全部保留为兼容别名，至少保留一个版本周期 |
| D4 | p12 继续走 `openssl` 子进程 | 默认 `compat`（3DES+SHA1）是 MIUI 兼容性硬需求，重实现风险大于收益；算法 profile 见 D11 |
| D5 | `ca.key` 保持 `0600 root:root` | 不做组权限放宽；`neca` 以 root 身份执行所有涉及 CA 私钥的操作 |
| D6 | `swanctl --load-creds` 实验：**已执行，结论可用** | 2026-11-15 实测：charon 日志出现 `crl #10:0d is newer - existing crl #10:0b replaced`，热加载生效且未重启容器；吊销默认改为 `creds` 模式 |
| D7 | 开发环境：本地 Docker | 本机无 Go、无 WSL 发行版；用容器获得可复现的构建与测试环境 |
| D8 | 服务器验证：真实环境 | 用临时 PKI 与真实 strongSwan 各验一遍，生产操作一律先 `--dry-run` |
| D9 | GitHub 集成 | 由 DSH 插件 `dsh-github-flow`（复用 `gh` CLI 的 OAuth 登录）承担仓库、PR、Actions 与发布操作 |
| D10 | 平台扩展与安装脚本 | 支持 Windows/Linux/macOS/Android/iOS；一键安装脚本默认关闭，`--installers` 显式开启（2026-11-15 决定） |
| D11 | p12 算法 profile | 默认 `compat`（3DES+SHA1，MIUI 兼容）；新增 `modern`（AES-256/SHA256）可选，默认行为不变 |
| D12 | 多语言 | 语言文件 JSON，默认 `zh-CN`，内置 `en-US`；社区可扩展；Web 多语言 M6 单独实现（2026-11-15 决定） |
| D13 | 许可与仓库策略 | **AGPL-3.0-only**（OSI 开源，允许商业使用但须履行源码公开义务）；**开发期暂不接受外部 PR**，完成后重新评估开放；v0.1.0 后已转 public |

D1 与评估报告的偏差需要记录：评估报告给 Python 85.3、Go 83.7，差距仅 1.6 分，属于"按阶段优先级取舍"而非"语言能力差距"。选 Go 的理由是**一次到位**：未来 Web 面板与多用户都复用同一份强类型 core，不再做第二次语言迁移；代价是阶段一的模板与文本处理工作量增加约 30–50%，以及需要先搭 Go 工具链。

---

## 1. 目标与范围

### 1.1 一句话目标

用 Go 重写现有 506 行 bash 证书管理脚本，行为完全兼容，命令名 `neca`，并为阶段二的 Web 面板与阶段三的多用户预留结构。

### 1.2 范围内

- 证书签发、吊销、恢复吊销、重签发、补平台包、改平台标注、删除；
- 列表、p12 自检、CSV 导出、索引同步；
- 交互向导（菜单 0–10 与编号语义不变）；
- 平台包生成：Windows（PowerShell + cmd）、Linux/macOS（p12 + 说明，一键脚本可选）、Android（说明 + p12）、iOS（mobileconfig）；
- CRL 生成与引擎重载（默认 `creds` 热加载，可配置回 `restart`）；
- 锁、审计、结构化输出、`--dry-run`。

### 1.3 范围外（本阶段不做）

- 服务端证书签发与管理（`server.crt` 仍手工维护）；
- CA 生成与轮换；
- OCSP、CRL 增量、证书策略；
- Web 面板与多用户（阶段二/三，本稿只预留结构）；
- 客户端自助申请（阶段三再议）。

---

## 2. 术语

| 术语 | 含义 |
|---|---|
| CN | 证书 Common Name，即 VPN 用户标识；只允许 `A-Z a-z 0-9 . _ -` |
| PKI 目录 | `/opt/ipsec/pki`，CA 与客户端证书、索引、元数据、归档的根 |
| 客户包 | `/opt/ipsec/clients/<CN>/`，每个 CN 一个目录，内含各平台子目录 |
| 索引 | `pki/ca/index.txt`，openssl `ca` 数据库，6 字段制表符分隔，CRL 的唯一事实源 |
| 元数据 | `pki/meta/<CN>.env`，记录平台、签发日期、到期时间、序列号 |
| 引擎 | strongSwan 容器 `ipsec-vpn`，通过 `docker restart` 或 `swanctl --load-creds` 重载凭据 |
| 影子对比 | 同一 fixture 分别跑旧 bash 与新 neca，比对输出与产物 |

---

## 3. 总体架构

```text
            ┌──────────────┐        ┌──────────────┐
            │  neca CLI    │        │  neca web    │   阶段二
            │ (cmd/neca)   │        │ (cmd/necad)  │
            └──────┬───────┘        └──────┬───────┘
                   │                       │
                   └──────────┬────────────┘
                              ▼
                   ┌────────────────────┐
                   │   internal/core    │   唯一业务实现
                   │ config / pki /     │
                   │ index / meta /     │
                   │ crl / engine /     │
                   │ bundle / audit     │
                   └─────────┬──────────┘
                             ▼
        ┌────────────┬───────────────┬──────────────┐
        │  openssl   │     docker    │   文件系统    │
        │  (子进程)   │   (子进程)     │  (原子写+锁)  │
        └────────────┴───────────────┴──────────────┘
```

三条不可破坏的架构约束：

1. `internal/core` 不依赖 `cmd/*`，也不直接打印到 stdout；返回结构化结果，由前端决定渲染（人类可读或 JSON）。
2. 所有外部命令集中在 `internal/core/proc`，统一超时、stderr 捕获、退出码与错误类型；其余包禁止直接 `exec.Command`。
3. 路径与开关只在 `internal/core/config` 出现一次。

未来 Web 面板（阶段二）在这个结构里只需要新增 `cmd/necad` 与 `internal/web`，core 一行不改。

### 3.1 扩展点（Extension Points）

目标：把"未来可能被替换或新增"的能力收敛为少量接口与注册表，让签发/吊销/CRL 等主流程保持不变；以后接新后端、加新命令时，不改业务代码、不改分发 switch。

设计原则（遵循 Go 惯例，避免过度抽象）：

1. **接口定义在使用方**：接口放在 `internal/core/ports` 这个只含接口与数据类型的叶子包，实现方反向依赖它，避免 `core → 子包 → core` 的循环依赖。
2. **有替换需求才抽接口**：本轮只落地已经明确的 `IndexStore` 与配置 `Source`/`Resolver`；其余先登记为候选，等 M1/M2 出现第一个真实实现时再抽象，不为假想需求写空接口。
3. **构造函数注入**：core 的领域函数接收接口参数，不在包内直接 new 具体实现；测试用内存假实现替换。
4. **注册表集中管理**：CLI 子命令通过 `cli.Registry` 注册，未来平台包生成器（Bundler）也用同样方式注册；业务代码里禁止出现按类型分支的 type switch。
5. **internal 不承诺兼容**：v0.1.0 之前接口可演进，但每次演进必须同步更新本节与测试假实现。

#### 3.1.1 已落地契约

| 扩展点 | 接口/类型 | 位置 | 首个实现（计划） | 后续可接入 |
|---|---|---|---|---|
| 索引存储 | `ports.IndexStore`、`ports.Row`、`ports.Index` | `internal/core/ports/index.go` | M1：`index.txt` 文件后端（flock + 原子替换） | 测试内存实现、Web 只读查询、只读缓存 |
| 配置来源 | `ports.Source`、`ports.Resolver` | `internal/core/ports/config.go` | M1：环境变量、`--config` 文件 | Web 设置、数据库覆盖、测试注入 |
| CLI 子命令 | `cli.Command`、`cli.Env`、`cli.Registry` | `internal/cli/command.go` | 已实现：`version`、`help` | `list`/`issue` 等、兼容别名、命令元数据复用 |

`IndexStore` 契约要点：

- `Load` 返回只读快照，不持有也不暴露锁；
- `Update` 在全局锁内执行一次读-改-写：fn 返回错误或 panic 时不落盘，成功时由实现负责 fsync + 原子替换；
- 拿不到锁时返回包装了 `ErrIndexLocked` 的错误，CLI 侧据此返回退出码 3；
- 生产实现只能是 `index.txt`（§10 唯一事实源），接口不得用于引入第二份证书状态存储。

`Source` / `Resolver` 契约要点：

- 一个 Source 只读自己那层配置，不填默认值、不做类型校验；错误必须带来源名；
- `Resolver` 按注册顺序合并，**先注册的优先**，即 `NewResolver(env, file)` 表达"环境变量高于配置文件"（§7）；
- key 与旧脚本环境变量名一致（`PKI_DIR`、`CRL_FILE` 等）；默认值与类型校验留给 M1 的 `core/config`。

CLI 注册表要点：

- `Command.Run` 只接收 `Env`（注入 stdout/stderr/args），禁止直接访问 `os.Stdout`、`os.Args`，保证可测试、可复用；
- 注册冲突（重名、别名冲突、nil Run）在启动时立即报错；内置命令用 `MustRegister` 在测试阶段暴露问题；
- `help` 文本由注册表自动生成，新增命令不需要改帮助代码。

#### 3.1.2 候选扩展点（先登记，暂不落接口）

| 扩展点 | 预计触发时机 | 未来实现示例 |
|---|---|---|
| 引擎重载 Engine | M2 实现 `engine.go` | docker restart、`swanctl --load-creds`、systemd、K8s Job |
| 审计输出 AuditSink | M2 实现 `audit.go` | 追加文件、SQLite、远程日志 |
| 平台包 Bundler | M1 实现 `bundle/` | Windows、Android、iOS、未来 macOS/Linux |
| 签名后端 Signer | 出现外部 CA/HSM 需求时 | 本地 openssl、企业 CA、PKCS#11 |
| 通知回调 Notifier | 需要 IM/Webhook 时 | 飞书、企业微信、邮件 |

候选扩展点不写空接口：M1/M2 实现时按 §3.1.3 的步骤抽象，避免接口形状被假想需求带偏。

#### 3.1.3 新增扩展点的步骤

1. 在 `internal/core/ports`（或对应领域的消费方包）定义接口与最小数据类型，写清并发、错误、锁与幂等语义；
2. 在 `internal/core/<领域>` 写第一个真实实现，构造函数接收依赖；
3. 在注册表登记：CLI 命令用 `cli.Registry.Register`，平台包用 M1 的 Bundler 注册表；
4. 补一个测试假实现（`_test.go`）与至少一个契约测试（成功、失败回滚、边界）；
5. 更新本节的契约表与 §19.2 变更记录，并在 PR 描述里说明兼容性影响。

#### 3.1.4 示例：新增一个子命令

```go
// 在 newBuiltinRegistry 中追加；Run 的分发逻辑无需改动。
registry.MustRegister(Command{
    Name:    "list",
    Summary: "列出全部证书",
    Run: func(env Env) int {
        // 调用 core 返回结构化结果，再在这里渲染人类可读文本或 JSON。
        return ExitOK
    },
})
```

---

## 4. 仓库与包结构

```text
neca/
├─ go.mod
├─ Makefile
├─ Dockerfile              # 运行镜像（scratch 或 distroless + openssl）
├─ Dockerfile.dev          # 开发/测试镜像（golang + openssl + git）
├─ docker-compose.yml      # 本地开发与测试
├─ .github/workflows/
│  ├─ ci.yml               # push/PR：vet + test + 构建
│  ├─ release.yml          # tag：交叉编译 + GitHub Release
│  └─ docker.yml           # tag：构建并推送 ghcr.io/hyhdsl/neca
├─ cmd/neca/main.go        # CLI 入口（薄）
├─ internal/
│  ├─ core/
│  │  ├─ ports/            # 扩展点接口与数据类型（§3.1）
│  │  │  ├─ index.go       # IndexStore / Row / Index
│  │  │  └─ config.go      # Source / Resolver
│  │  ├─ config.go         # 环境变量、默认值、校验
│  │  ├─ proc.go           # 外部命令执行器
│  │  ├─ openssl.go        # openssl 调用与输出解析
│  │  ├─ pki.go            # 签发/重签/归档
│  │  ├─ index.go          # index.txt 读写 + flock + 原子替换
│  │  ├─ meta.go           # meta/<CN>.env 读写
│  │  ├─ crl.go            # gencrl + 校验
│  │  ├─ engine.go         # docker restart / swanctl --load-creds
│  │  ├─ audit.go          # 追加式审计日志
│  │  ├─ table.go          # 中文对齐表格
│  │  ├─ csv.go            # 导出
│  │  ├─ models.go         # Cert / Meta / IssueRequest
│  │  └─ bundle/           # windows.go android.go ios.go
│  ├─ cli/
│  │  ├─ cli.go            # 入口、全局开关、内置命令装配
│  │  └─ command.go        # 子命令注册表（扩展点）
│  └─ app/                 # 阶段二：HTTP 服务
├─ templates/              # embed 的 PS1 / mobileconfig / 说明文本
└─ testdata/               # 黄金文件与 fixture
```

M2/M3 规划新增：`internal/core/p12.go`、`package.go`、`downloads.go`、`bundle/linux.go`、`bundle/macos.go`、`internal/i18n/{i18n.go,locales/*.json}`、`templates/linux/`、`templates/mac/`。

---

## 5. 依赖策略

| 类别 | 选择 | 理由 |
|---|---|---|
| CLI 解析 | **标准库 `flag` + 子命令 switch** | 10 个子命令，不值得引入 cobra；零依赖、帮助文本可控 |
| 中文宽度 | `golang.org/x/text/width` | `--list` 对齐需要 East Asian Width |
| 密码哈希 | `golang.org/x/crypto/bcrypt` | Web 管理员密码只存 bcrypt 哈希；M6a 引入 |
| plist | 手写 XML 生成（`encoding/xml` + `encoding/base64`） | mobileconfig 结构固定，约 100 行；用测试解析回来校验 |
| CSV | `encoding/csv` | 标准库 |
| 文件锁 | `syscall.Flock`（Linux） | 标准库，无依赖 |
| 原子写 | `os.CreateTemp` + `os.Rename` | 标准库 |
| 模板 | `text/template` + `embed` | 标准库；模板随二进制内置 |
| 日志 | `log/slog` | 标准库，结构化输出 |
| 测试 | `testing` + `testdata` | 标准库 |
| zip | `archive/zip` | 标准库；不依赖外部 `zip` 命令 |
| 多语言 | `encoding/json` + `embed` | 标准库；语言包随二进制内置 |

**总共 2 个第三方模块**（`x/text`、`x/crypto`），交叉编译无 cgo，`CGO_ENABLED=0`。

---
## 6. CLI 设计

### 6.1 子命令

| 子命令 | 兼容旧写法 | 作用 |
|---|---|---|
| `neca issue <CN> --platforms a,b --days N [--installers …] [--p12-profile …] [--zip]` | `--issue CN` | 签发新证书 |
| `neca revoke <CN>` | `--revoke CN` | 吊销 |
| `neca unrevoke <CN>` | `--unrevoke CN` | 恢复吊销 |
| `neca reissue <CN> [--installers …] [--p12-profile …] [--zip]` | `--reissue CN` | 归档旧证书并重签 |
| `neca platform add <CN> --platforms ios [--installers …] [--zip]` | `--add-platform CN` | 补平台包 |
| `neca platform set <CN> --platforms …` | `--set-platforms CN` | 只改元数据标注 |
| `neca delete <CN> [--yes] [--purge-revoked]` | `--delete CN --yes` | 删除（默认拒删 R 行） |
| `neca list [--json]` | `--list` | 证书列表 |
| `neca verify [CN…] [--json]` | `--verify` | p12 自检 |
| `neca export issued\|revoked [--out F]` | `--export-issued` / `--export-revoked` | CSV 导出 |
| `neca index sync` | `--sync` | 补索引与元数据 |
| `neca package <CN> [--platforms …] [--installers …] [--p12-profile …] [--zip]` | — | 从现有证书重建平台包，可选 zip |
| `neca downloads list\|clean [--expired\|--all]` | — | 管理下载目录中的 zip 与有效期 |
| `neca config init\|show\|validate\|export-env` | — | 管理 `neca.json` 设置 |
| `neca web status` | — | Web 配置/只读/空闲超时/helper 状态 |
| `neca web readonly status\|on\|off` | — | 只读开关；M6a 的 `off` 拒绝（helper 未实现） |
| `neca web idle-timeout status\|<duration>` | — | 查看/设置空闲超时（5m–24h） |
| `neca web hash-password` | — | 从 stdin 生成 bcrypt 管理员密码哈希 |
| `neca`（无参数） | 无参数 | 进入交互向导 |
| `neca version` | `-V` | 版本与构建信息 |

兼容规则（D3）：

- 旧 flag 在 `main` 开头做一次预处理，映射为等价子命令，**行为与文案不变**；
- 未知位置参数仍按 CN 处理（兼容 `./script CN` 风格）；
- 一个版本周期后（v1.0）再考虑移除，移除前先打印一次性弃用提示到 stderr。

### 6.2 通用开关

| 开关 | 作用 | 默认 |
|---|---|---|
| `--json` | 结构化输出，关闭 `[OK]` 等人类文案（M3 已实现） | 关 |
| `--dry-run` | 只打印将执行的动作，不落盘、不调 docker（M3 已实现） | 关 |
| `--no-reload` | 只生成 CRL，不重载引擎 | 关 |
| `--reload-mode restart\|creds` | 重载方式；CLI 开关待接入，当前用 `NECA_RELOAD_MODE` | creds |
| `--config <file>` | 读取 JSON 配置文件；优先级低于环境变量、高于默认值 | 无 |
| `--lang zh-CN\|en-US` | 控制台语言；M3 起从语言文件加载，默认 `zh-CN` | zh-CN |
| `-q` / `--quiet` | 仅输出错误 | 关 |
| `--version` | 打印版本 | — |

### 6.3 输出与退出码

人类可读输出必须保留四个前缀，**这是下游契约**：`[OK]  `、`[INFO] `、`[WARN] `、`[ERR]  `。

| 退出码 | 含义 |
|---|---|
| 0 | 成功（可以包含 `[WARN]`） |
| 1 | 用法错误（缺参数、CN 非法、未知子命令） |
| 2 | 运行期失败（openssl、docker、文件系统） |
| 3 | 前置条件不满足（证书已存在、容器未找到、缺 CA、锁定冲突） |

### 6.4 `--json` 输出约定

统一信封，便于未来 Web 直接复用：

```json
{
  "ok": true,
  "action": "issue",
  "cn": "TMP01",
  "warnings": ["p12 密码长度 12 位"],
  "data": { "serial": "7063A6…", "expires": "2027-11-15T00:00:00Z", "platforms": ["windows","android"] },
  "durationMs": 1234
}
```

错误时 `ok=false`，附 `error: {code, message}`；退出码不变。

M3 已实现：`list/verify/version/issue/reissue/revoke/unrevoke/delete/platform/package/downloads/export/index sync` 原生结构化输出；M6a 增加 `web status/readonly/idle-timeout` 的 JSON 输出。其余命令以 `data.output` 兜底，stdout 始终是单个合法 JSON 对象。

### 6.5 交互向导

菜单编号与旧版完全一致（1 签发、2 吊销、3 恢复、4 重签、5 加平台、6/7 导出、8 列表、9 同步、10 删除、0 退出）。M3 已实现：`neca` 无参数进入向导，每个动作执行前显示等价 CLI 命令。

---

## 7. 配置

沿用"环境变量优先、否则默认值"的写法，变量名与旧脚本完全一致：

| 变量 | 默认值 |
|---|---|
| `PKI_DIR` | `/opt/ipsec/pki` |
| `CA_CERT` / `CA_KEY` | `$PKI_DIR/ca.crt` / `$PKI_DIR/ca.key` |
| `CADB_DIR` | `$PKI_DIR/ca` |
| `INDEX_FILE` / `CRLNUM_FILE` / `CA_CONF` | `$CADB_DIR/index.txt` 等 |
| `CRL_DIR` / `CRL_FILE` | `/opt/ipsec/swanctl/x509crl` / `…/ca.crl.pem` |
| `CLIENTS_DIR` | `/opt/ipsec/clients` |
| `META_DIR` / `ARCHIVE_DIR` / `EXPORT_DIR` | `$PKI_DIR/meta` 等 |
| `CONTAINER` / `DOCKER_BIN` | `ipsec-vpn` / `docker` |
| `SERVER_ID` / `ORG` / `CA_CN` | `vpn.example.com` / `ExampleOrg` / `ExampleOrg VPN CA` |
| `DEFAULT_DAYS` / `KEY_BITS` | `365` / `3072` |
| `NECA_RELOAD_MODE` | `creds`（热加载；设为 `restart` 则重启容器） |
| `NECA_LOCK_FILE` | `$PKI_DIR/.lock` |
| `NECA_DOWNLOAD_DIR` | `/opt/ipsec/downloads`（zip 下载目录） |
| `NECA_DOWNLOAD_TTL` | `30m`（下载有效期） |
| `NECA_DOWNLOAD_MAX_FILES` | `50`（防积压上限） |
| `NECA_P12_PROFILE` | `compat`（3DES+SHA1；可选 `modern`） |
| `NECA_INSTALLERS` | `none`（可选 `all` 或平台列表） |
| `NECA_AUDIT_LOG` | `$PKI_DIR/audit.log`（追加式审计） |
| `NECA_CONFIG` | `/opt/ipsec/neca.json`（JSON 配置路径） |
| `NECA_LANG` | `zh-CN`（M3 已生效） |
| `NECA_LOCALE_DIR` | 未启用；当前仅内置语言包 |

> `SERVER_ID` / `ORG` / `CA_CN` 的默认值是仓库占位值（`vpn.example.com` / `ExampleOrg` / `ExampleOrg VPN CA`）。生产部署必须用环境变量注入真实值，不要把真实服务器标识写回仓库。

`config.Load()` 负责读取、补默认值、校验目录可写、校验 CA 存在；缺失时返回退出码 3 的错误。

### 7.1 JSON 配置（v0.2.0）

优先级固定为：**CLI flag > 环境变量 > `neca.json` > 默认值**。`--config`
（或 `NECA_CONFIG`）指定路径，默认 `/opt/ipsec/neca.json`；文件不存在时全部使用
环境变量/默认值，不影响旧部署。

```json
{
  "schemaVersion": 1,
  "serverId": "vpn.example.com",
  "org": "ExampleOrg",
  "caCn": "ExampleOrg VPN CA",
  "language": "zh-CN",
  "auditLog": "/opt/ipsec/pki/audit.log",
  "paths": { "pkiDir": "/opt/ipsec/pki", "clientsDir": "/opt/ipsec/clients" },
  "defaults": { "days": 365, "keyBits": 3072, "p12Profile": "compat", "installers": "none" },
  "reload": { "mode": "creds", "container": "ipsec-vpn", "dockerBin": "docker" },
  "download": { "dir": "/opt/ipsec/downloads", "ttl": "30m", "maxFiles": 50 }
}
```

- 只保存路径与普通配置，不保存 CA 私钥、p12 密码；文件 0600，写入使用临时文件 + rename。
- `config init [--from-env]` 生成配置；`config show/validate/export-env` 查看、校验、导出环境变量。
- `pki/meta/<CN>.env`、`index.txt`、CRL 仍保持原有格式，不迁入 JSON；SQLite 仅在 M6 Web 有明确并发需求时评估。

---

## 8. 核心模块设计

### 8.1 proc：命令执行器

```go
type Result struct { Stdout, Stderr string; Code int; Duration time.Duration }
func (r *Runner) Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error)
```

- 统一 `context.WithTimeout`（默认 60s，docker restart 用 120s）；
- 捕获 stdout/stderr，错误时把 stderr 尾部带进 error message；
- `--dry-run` 时只记录不执行（`Runner.Dry`）；
- 测试注入 `Runner`，不碰真实 openssl/docker。

### 8.2 openssl：调用与解析

固定使用的命令（与旧脚本一一对应）：`genrsa`、`req -new -subj`、`x509 -req -CAcreateserial`、`x509 -noout -serial/-subject/-enddate/-startdate/-fingerprint/-ext`、`pkcs12 -export`、`pkcs12 -in -info`、`ca -gencrl`、`crl -in -text`。

解析要点：CN 用 `-nameopt sep_multiline`（替换旧脚本的 RFC2253 + `tr`），避免 subject 含逗号时解析错。

### 8.3 index：索引读写

- 结构体 `Row{Status, Expires, RevokedAt, Serial, File, Subject}`；
- 读：逐行 6 字段；写：先 `flock(LOCK_EX)`，再写临时文件 `O_CREATE|O_EXCL`，`fsync` 后 `os.Rename`；
- 所有修改（增/改/删）都在同一把锁内完成，锁粒度到整个 PKI；
- 序列号统一大写十六进制，到期时间 `YYMMDDHHMMSSZ`（UTC）。

### 8.4 meta：元数据

`KEY=VALUE` 逐行，保留旧键，新增 `SCHEMA=2`、`OWNER=`、`DEVICE=`（可选）。读时忽略未知键，写时保持键顺序稳定。

M1 的 `index sync` 仍按旧脚本的 5 个键（CN/PLATFORMS/CREATED/EXPIRES/SERIAL）写入，保证影子对比逐字节一致；`SCHEMA=2` 与 `OWNER`/`DEVICE` 在 M2 写命令时再增加。

### 8.5 pki：签发与重签

签发流程与旧脚本一致：校验 CN → 生成密钥（0600）→ CSR → 写 `.ext` → `x509 -req` → 索引 → 元数据 → p12 → 平台包。每个失败路径用 `defer` 清理未完成产物（旧脚本会残留密钥）。

p12 默认走 `compat` profile（§8.9）；`issue/reissue/package` 可用 `--p12-profile` 选择其他算法，默认行为与旧脚本一致。

### 8.6 crl / engine

- `crl.Gen()`：调用 `openssl ca -gencrl`，先写临时文件再原子替换，`chmod 0644`；
- `engine.Reload()`：按 `NECA_RELOAD_MODE` 二选一：
  - `restart`：`docker restart`，轮询 `swanctl --stats` 最多 40s（旧行为）；
  - `creds`：`docker exec ipsec-vpn swanctl --load-creds`，不重启、不断线（2026-11-15 实测通过，charon 日志确认 CRL 被热替换）。

### 8.7 audit

`/opt/ipsec/pki/audit.log`（可用 `NECA_AUDIT_LOG` 覆盖），每行：`RFC3339 操作者(uid:user) 动作 CN 序列号 结果`。只追加，不阻塞主流程；写失败只告警。M3 已实现 `core.AppendAudit`，写入 0600 文件。

### 8.8 table：中文对齐

按 East Asian Width 计算显示宽度（W/F 记 2），列宽取最大值，输出两空格分隔；与旧 python 实现逐行等价。

### 8.9 p12 算法 profile

| profile | 参数 | 用途 |
|---|---|---|
| `compat`（默认） | `-keypbe PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -macalg sha1` | 旧契约，MIUI/旧 Android 兼容 |
| `modern` | `-keypbe AES-256-CBC -certpbe AES-256-CBC -macalg sha256` | 新客户端；MIUI 可能卡在“提取证书” |

- 开关：`--p12-profile compat|modern`，环境变量 `NECA_P12_PROFILE`，默认 `compat`。
- 默认 profile 必须与旧脚本逐字节兼容；影子测试固定用 `compat`。
- 非默认 profile 可在 meta 写 `P12_PROFILE`（可选键）；默认不写，避免旧脚本 diff。
- 密码仍为 12 位字母数字、仅客户端证书；算法切换不改变证书、密码与目录布局。

### 8.10 package / downloads

- `bundle/` 新增 `linux.go`、`macos.go`；`package` 负责重建平台包（不重签证书）。
- `downloads` 负责 zip 生命周期：创建、列出、按 TTL/上限清理；目录与元数据见 §9.3。
- zip 用 `archive/zip`；打包前校验路径，拒绝绝对路径与 `..`。
- 删除/清理只作用于 `NECA_DOWNLOAD_DIR`，不触碰 `clients/`、`exports/`、`pki/`。

### 8.11 多语言

- 语言文件：`internal/i18n/locales/<lang>.json`，`embed` 进二进制；内置 `zh-CN`（默认）与 `en-US`（M3 已实现）。当前覆盖帮助、通用开关、命令摘要、向导与 M3 提示；命令业务输出后续按 key 逐步迁移。
- 选择顺序：`--lang` > `NECA_LANG` > `LANG/LC_ALL` > `zh-CN`；缺 key 回退到 `zh-CN`，再回退到 key 本身并告警。
- CI 校验内置 locale 的 key 集合一致；社区可先通过 fork/Issue 增加语言包，PR 待开发完成后开放。
- 输出前缀 `[OK]/[INFO]/[WARN]/[ERR]` 不翻译，保持下游契约；Web 多语言在 M6 单独实现。

---

## 9. 模板与平台包

### 9.1 平台矩阵

面向 5 个平台：Windows / Linux / macOS / Android / iOS。

| 平台 | 目录 | 默认产物 | 可选一键脚本（默认关闭） | 说明 |
|---|---|---|---|---|
| Windows | `Windows/` | `p12`、`pass`、`ca.crt`、`使用说明.txt` | `install.ps1`、`uninstall.ps1`、`connect.ps1` + 三个 `.cmd` | 脚本由旧实现验证；现在需 `--installers windows` 才生成 |
| Linux | `Linux/` | 同上 | `install.sh`、`uninstall.sh`（`--installers linux`） | 面向 strongSwan；安装 CA/证书/私钥、写 conf.d、更新系统 CA、load-all；支持 `NECA_ROOT`/`NECA_DRY_RUN` |
| macOS | `macOS/` | 同上 + `<CN>.mobileconfig` | `install.command`、`uninstall.command`（`--installers macos`） | 导入 CA/p12 到系统钥匙串并安装 mobileconfig；卸载删证书/描述文件 |
| Android | `Android/` | `p12`、`pass`、`ca.crt`、`使用说明.txt` | — | 与旧实现一致 |
| iOS | `iOS/` | `<CN>.mobileconfig`（CA + 身份 + IKEv2） | — | 与旧实现一致 |

- 平台名统一为 `windows/linux/macos/android/ios`；兼容别名：`win/windows`、`mac/macos/darwin/osx`、`and/android`。
- 平台目录共同产物：`<CN>.p12`、`<CN>.p12.pass`、`ca.crt`、`使用说明.txt`；`MANIFEST.txt` 只放在 zip 根部，平台目录保持旧文件树以兼容影子测试。
- **不进入平台包**：`<CN>.key`、`ca.key`、`index.txt`、meta 文件、审计日志。

### 9.2 一键安装脚本选项

- 统一开关：`--installers none|all|windows|linux|macos`（逗号分隔可多选），默认 **none**。
- 兼容说明：旧脚本默认生成 Windows 脚本；neca 改为默认不生成，需要时显式 `--installers windows`。M2 影子测试必须带该开关才能与旧 `clients/` 树逐项对比。
- Windows 脚本保留现有内容与文件名，模板必须纯 ASCII（§9.4）。
- Linux/macOS 脚本已实现，默认仍不生成；启用后需在目标发行版/版本做人工验证（脚本支持 `NECA_DRY_RUN=1` 预览、`NECA_ROOT` 沙箱测试）。
- `issue/reissue/platform add/package` 均支持该开关；`NECA_INSTALLERS` 可作为环境级默认值。

### 9.3 zip 打包与下载有效期

- CLI：`neca package <CN> [--platforms ...] [--installers ...] [--p12-profile ...] [--zip]` 重新生成平台包；`--zip` 额外生成压缩包。`issue/reissue` 也接受 `--zip`，签发完成后直接打包。
- **不新增独立 `zip` 子命令**：zip 是打包输出而非独立业务动作，用 `--zip` 更符合现有流程，减少子命令数量；生命周期管理由 `neca downloads` 承担：
  - `neca downloads list`：列出 zip、CN、生成时间、过期时间、大小；
  - `neca downloads clean [--expired|--all]`：清理过期/全部 zip。
- 存储位置：`NECA_DOWNLOAD_DIR`（默认 `/opt/ipsec/downloads`），不放进 `clients/` 或 `exports/`，避免污染证书目录与备份范围。
- 命名与元数据：`<CN>-<YYYYmmdd-HHmmss>-<6位随机>.zip` + 同名 `.json`（CN、createdAt、expiresAt、size、sha256、platforms、p12Profile）。
- 权限：目录 `0700`、zip/元数据 `0600`；zip 含 p12 与密码文件，按敏感凭据处理。
- 有效期：`NECA_DOWNLOAD_TTL` 默认 `30m`；创建时写入 expiresAt。CLI 在每次创建前清理过期项；`downloads clean` 手动清理；Web（M6）在下载前检查并在后台定时清理，过期返回 410/404。
- 防积压：TTL + `NECA_DOWNLOAD_MAX_FILES`（默认 50）；不依赖 cron 也能自净，生产可另配 systemd timer 兜底。
- 压缩实现：标准库 `archive/zip`，**不调用外部 `zip` 命令**。理由：debian-slim 默认无 `zip`、跨平台行为不一致、需防命令注入；标准库产物可被常见工具打开且便于单测。
- 压缩包以 `<CN>/<Platform>/...` 为根，不含绝对路径、`..`、私钥、CA 私钥；生成前校验所有路径。
- CLI 打印 zip 路径、过期时间与清理命令；下载 URL 的鉴权由 Web（M6）负责。

### 9.4 硬规则

1. **平台脚本模板必须是纯 ASCII**（PowerShell 5.1 无 BOM 时按 ANSI 读；Linux/macOS 也避免 locale 差异）。用 `make check-ascii` 在 CI 里校验 `templates/{win,linux,macos}/*` 无 >127 字节。
2. Linux/macOS 安装脚本默认不生成；分发前完成目标系统人工验证。
3. iOS 的 `LocalIdentifier` 取 CN、`RemoteAddress`/`RemoteIdentifier` 取 `SERVER_ID`；plist 生成后要用解析器回读校验键值。
4. zip 与 `MANIFEST.txt` 不得包含私钥、CA 私钥、审计或服务器敏感标识。

---

## 10. 数据与兼容

- 不迁移、不重命名任何已有目录；新旧实现共用同一份 `index.txt`、`meta/`、`clients/`、`exports/`。
- 新增 `downloads/`（默认 `/opt/ipsec/downloads`）只存临时 zip 与 sidecar，可随时清理，不属于证书事实源。
- 平台标识扩展为 `windows/linux/macos/android/ios`；meta 的 PLATFORMS 仍兼容旧值。
- 默认 `--installers none` 会让 `clients/` 树比旧脚本少 Windows 脚本；M2 影子测试必须用 `--installers windows` 才能与旧产物对比，另测默认 none。
- p12 默认 `compat`；非默认 profile 可写 meta `P12_PROFILE`，默认不写以保持 diff=0。
- `index.txt` 是 CRL 的唯一事实源，**不允许**引入第二份证书状态存储；SQLite（阶段三）只存用户、审计、归属。
- 版本切换用 `git` 管脚本与二进制，回退只需换回旧 `legacy/vpn-cert-manager.sh`。

---

## 11. 错误处理与回滚

| 场景 | 处理 |
|---|---|
| 签发中途失败 | `defer` 删除已生成的 key/csr/ext/crt，提示可重试 |
| 索引写失败 | 临时文件丢弃，索引保持原样（原子替换保证） |
| gencrl 失败 | 不回滚索引，但**不替换**旧 CRL；提示手工排查 |
| 容器重载失败 | 保留新 CRL，`[WARN]` 提示需手工重启（下次重启自动生效） |
| 并发执行 | `flock` 拿不到锁立即失败（退出码 3），提示已有实例在运行 |
| 平台包/zip 失败 | 删除临时文件与半成品 zip，保留已成功平台包；`[ERR]` 提示可重试 |
| 下载清理失败 | 只 `[WARN]`，不阻断主流程；下次创建/访问时重试 |

---

## 12. 安全设计

### 12.1 现状（D5）

`ca.key` 保持 `0600 root:root`，`neca` 以 root 运行。理由：放开组权限等于让服务进程长期持有 CA 私钥读取权，收益不明、暴露面更大。代价是所有操作必须 root，非 root 用户只能做只读命令（`list`、`export`、`verify` 在权限足够时也可运行）。

### 12.2 阶段二的约束

Web 服务若还需要读写 CA 私钥，只有三种选择：

1. 以 root 运行 `necad`，仅监听 `127.0.0.1`，前面挂认证反向代理；
2. 服务以非 root 运行，特权操作通过一个 root 的本地 helper（Unix socket + 命令白名单）；
3. 把服务放到独立容器，只挂载必要的 `/opt/ipsec` 子目录并配合 sudoers 重启容器。

本稿不选型，但明确：**不要**把 `ca.key` 复制到服务进程可读的目录，也不要让 Web 进程直接接受任意路径参数。

### 12.3 其他

- 私钥、p12、密码文件权限：`0600`；
- 审计日志只追加；
- `--dry-run` 默认用于任何生产变更前的演练；
- 不记录密码、私钥到日志；错误信息里只出现路径与序列号；
- `downloads/` 目录 `0700`、zip/元数据 `0600`；zip 仅含 p12/pass/ca.crt/说明/可选脚本，严禁私钥与 CA 私钥；
- 下载有效期默认 30 分钟，Web 需鉴权 + TTL；不接受任意路径参数，防目录遍历。

---
## 13. 测试策略

### 13.1 分层

| 层 | 内容 | 依赖 |
|---|---|---|
| 单元测试 | index 解析、meta 读写、表格宽度、CSV 转义、模板渲染、plist 生成 | 纯 Go，无外部命令 |
| 黄金测试 | `testdata/golden/` 存期望输出，`-update` 更新；覆盖 list/export/verify 文本 | 需要 fixture 证书 |
| 集成测试 | 临时 PKI（`t.TempDir()` + `PKI_DIR` 重定向）跑完整签发/吊销/重签 | 需要 openssl；docker 用 stub |
| 影子测试 | 同一 fixture 跑旧 bash 与新 neca，比对产物与索引三元组 | 需要 bash + openssl |
| 平台包测试 | Windows/Linux/macOS 文件树、安装脚本开关、MANIFEST、zip 内容与路径安全 | 纯 Go + fixture |
| 多语言测试 | zh-CN/en-US key 对齐、缺 key 回退、`NECA_LANG`/`--lang` | 纯 Go |
| 手工验收 | 真实服务器、真实手机导入 p12、真实 VPN 连接 | 人工 |

### 13.2 fixture 生成

`testdata/gen-fixture.sh`：在目标目录生成独立 CA（RSA 2048 加速）、TMP01/TMP02（有效）、TMP03（已吊销）、TMP05（孤立证书，用于 sync）以及一条仅存在于索引的已吊销记录；同时生成 CRL、meta 与客户端 p12。所有路径通过环境变量注入，**任何测试都不得触碰 `/opt/ipsec`**。

### 13.3 影子对比（迁移期最重要）

同一份 fixture 下依次执行，逐项比对：

```text
issue TMP01 windows,android → list → verify → add-platform ios → export issued
  → revoke → export revoked → unrevoke → reissue → delete
```

比对项：`--list` 表体（忽略列宽）、导出 CSV（去时间戳列）、`index.txt` 的 (状态,序列号,CN) 集合、CRL 序列号集合、`clients/` 目录树与文件大小。差异必须为 0 才允许合并。

M1 已落地：`scripts/shadow.sh` + `make shadow` 自动生成 fixture 的独立副本，比较 list/verify/export/index sync 的输出、CSV、index.txt 与 meta 目录；CI 将差异视为失败。

M2 已加入写命令影子对比：issue（归一化序列号/日期/路径）、revoke、unrevoke、platform set/add、delete；issue/reissue 使用 `--installers windows` 保证 `clients/` 树与旧脚本一致。M3 新增 i18n key 对齐、Linux/macOS 脚本渲染与 `bash -n`、JSON/dry-run/向导测试。

### 13.4 CI 门禁

- `go vet ./...`、`gofmt -l` 无输出、`go test -race ./...`；
- `make check-ascii`（Windows/Linux/macOS 脚本模板纯 ASCII）；
- `make build` 产出 linux/amd64 与 linux/arm64 两个二进制；
- `make shadow`（新旧实现输出与产物 diff = 0）；
- `make check-locales`（M3 已接入，校验内置语言包 key 一致）；
- 提交信息遵循 Conventional Commits。

---

## 14. 开发环境（D7）

本机没有 Go、没有 WSL 发行版，**用 Docker 作为唯一构建环境**，保证与服务器同为 Debian 家族。

### 14.1 镜像

`Dockerfile.dev`：`golang:1.25-bookworm` + `openssl` + `git` + `ca-certificates` + `make` + `python3`（影子测试）；Go module 代理优先走 `goproxy.cn`。

### 14.1.1 本机实际环境（2026-11-15 实测）

| 项 | 值 |
|---|---|
| OS / Shell | Windows 11，DSH 内置 MSYS bash |
| Docker | Docker Desktop，Engine 与 CLI 均为 **29.9.3** |
| 基础镜像 | `golang:1.25-bookworm`（1.2 GB），内含 `go1.25.14` 与 `/usr/bin/openssl` |
| 镜像源 | Docker Hub 直连不可达；`~/.docker/daemon.json` 已配 `registry-mirrors`：`docker.1ms.run`、`docker.xuanyuan.me`、`dockerproxy.net` |
| docker 命令 | `~/.dsh/bin/docker` wrapper（DSH 进程的 PATH 是启动快照，系统 PATH 里的 Docker 目录在会话内不可见）；同目录放置 `docker-credential-desktop.exe` |
| git | git 2.56（Windows） |
| gh | `~/.dsh/bin/gh.exe` 2.104.0，已 OAuth 登录 **hyhdsl** |

> 重启 DSH 后原生 PATH 生效，wrapper 变为冗余但无害。以上为实测结果：`hello-world` 拉取运行成功，Go 镜像可直接用于构建。

```bash
# 构建开发镜像
docker build -f Dockerfile.dev -t neca-dev .

# 进入开发容器（Windows 下把 docker 加到 PATH：C:\Program Files\Docker\Docker\resources\bin）
docker run --rm -it -v "$PWD":/src -w /src \
  -v neca-gomod:/go/pkg/mod neca-dev bash
```

### 14.2 常用命令（Makefile）

| 目标 | 作用 |
|---|---|
| `make dev` | 起开发容器 |
| `make build` | 构建 linux/amd64 + arm64 到 `dist/` |
| `make test` | 容器内 `go test -race ./...` |
| `make vet` / `make fmt` | 静态检查与格式化 |
| `make check-ascii` | 校验 Windows 模板 |
| `make shadow` | 新旧实现影子对比（只读命令 + M2 写命令 + issue；bash + openssl + python3，diff 必须为 0） |
| `make check-locales` | 校验内置语言包 key 一致（M3 已实现） |
| `make docker` | 构建运行镜像 `ghcr.io/hyhdsl/neca:dev` |

### 14.3 交叉编译

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD)" \
  -o dist/neca-linux-amd64 ./cmd/neca
```

### 14.4 服务器实验

1. `scp dist/neca-linux-amd64 root@<SERVER_IP>:/tmp/neca && chmod +x`；
2. 用 `PKI_DIR=$(mktemp -d)` 的临时 CA 先跑全流程（不碰生产）；
3. 生产只读命令直接跑：`/tmp/neca list`、`/tmp/neca verify`、`/tmp/neca export issued`；
4. 与旧脚本输出 diff；
5. 写操作先 `--dry-run`，再在维护窗口执行。

---

## 15. GitHub 仓库与 CI/CD（D9）

### 15.1 仓库规划

| 项 | 建议 |
|---|---|
| 仓库 | `github.com/hyhdsl/neca`，**public**（2026-11-15 随 v0.1.0 转公开） |
| 默认分支 | `main`，维护者内部合并必须 CI 通过；开发期暂不接受外部 PR。私有免费计划暂无法启用分支保护，以 CI 为门禁 |
| 分支命名 | `feat/…`、`fix/…`、`chore/…` |
| 提交信息 | Conventional Commits（`feat:`、`fix:`、`chore:`、`docs:`） |
| 版本 | SemVer；tag `v0.1.0` 起 |
| 许可证 | **AGPL-3.0-only**（OSI 认可；允许商业使用，但分发/网络服务须按 AGPL 公开对应源码）；见 `LICENSE` / `NOTICE` / `SECONDARY-DEVELOPMENT.md` |
| 公开时间 | ✅ v0.1.0 发布后（2026-11-15）已由私有转为 public |
| PR 策略 | **开发期暂不接受外部 PR**（仅维护者内部合并）；功能开发完成后重新评估并开放；问题走 Issue，修改请先 fork 并遵守 AGPL |
| 二次开发 | 保留原作者与仓库链接、保留 AGPL 许可、履行源码公开义务；商业使用本身允许 |

### 15.2 工作流

| 文件 | 触发 | 内容 |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR | setup-go、`go vet`、`go test -race`、`make check-ascii`、`make check-locales`、`make shadow`、构建 amd64 |
| `.github/workflows/release.yml` | tag `v*` | 交叉编译 amd64/arm64，生成 checksum，创建 GitHub Release |
| `.github/workflows/docker.yml` | tag `v*` / 手动 | buildx 构建，推送 `ghcr.io/hyhdsl/neca:<tag>` 与 `:latest` |

所需权限与密钥：

- `release.yml`：`permissions: contents: write`，用内置 `GITHUB_TOKEN`，无需额外 secret；
- `docker.yml`：`permissions: packages: write` + `GITHUB_TOKEN` 登录 `ghcr.io`，**无需 PAT**；
- 若未来需要跨仓操作，再考虑 GitHub App，不要引入长期 PAT。

当前状态（2026-11-15）：`ci.yml` 已落地并加入 M1/M2 shadow 与 M3 `check-locales`；M5 新增 `release.yml`（tag 构建二进制 + checksums + Release）与 `docker.yml`（buildx amd64/arm64 推送 GHCR）。

### 15.3 发布流程

```text
feature 分支 → 维护者内部合并（CI 全绿；开发期暂不接受外部 PR）→ squash merge 到 main
  → 打 tag v0.1.0 → release.yml 产出二进制 + Release
  → docker.yml 推送镜像 → 服务器下载二进制替换 /usr/local/bin/neca
```

回滚：保留上一个二进制为 `/usr/local/bin/neca.prev`，或直接从 Release 下载旧版本。

Docker CLI 部署：镜像包含 openssl + 静态 docker CLI，挂载 `/opt/ipsec` 与
`/var/run/docker.sock`；`deploy/docker-compose.neca.yml` 与服务器侧
`/opt/ipsec/neca.env`（0600）配合使用，真实 `SERVER_ID`/`ORG`/`CA_CN` 不进入仓库。
详见 `deploy.md`。

发布已完成（2026-11-15）：v0.1.0 的二进制、checksums 与 GHCR 镜像已发布，仓库已转 public；README 警告、AGPL `LICENSE`、`NOTICE`、`SECONDARY-DEVELOPMENT.md` 均已就位。外部 PR 在开发期暂不接收，完成后重新评估。

### 15.4 DSH 的 GitHub 集成

已安装 `dsh-github-flow@0.7.1`（桌面 profile 的 `dsh.profile.bundles` 已登记）。它复用宿主机 `gh auth login` 的 OAuth 凭据，不需要在配置文件里写 PAT。

前置条件：`gh` CLI 在 PATH 中并已登录。

实际安装方式（2026-11-15）：`winget install GitHub.cli` 在非交互会话中会等待提权而挂起，改用官方便携版 zip 解压到 `~/.dsh/bin/gh.exe`（该目录已在 PATH），版本 **2.104.0**。

```bash
# 便携安装（无需管理员）
curl -L -o gh.zip https://github.com/cli/cli/releases/download/v2.104.0/gh_2.104.0_windows_amd64.zip
tar -xf gh.zip -C /tmp/ghx && cp /tmp/ghx/bin/gh.exe ~/.dsh/bin/

# OAuth 登录（device flow，不需要手工创建 PAT）
gh auth login --hostname github.com --git-protocol https --web
```

`gh auth login --web` 走 GitHub 官方 device flow：终端显示一次性代码，浏览器打开 `github.com/login/device` 输入即可。

当前状态：已登录账号 **hyhdsl**，scope 为 `read:org`、`repo`、`workflow`、`write:packages`；`github_api` 与 `github_repo` 已实测可调用。

已注册的工具族（与本项目相关）：

| 工具 | 用途 |
|---|---|
| `github_repo` | 仓库元数据、免 clone 读文件/目录、代码与仓库检索 |
| `github_pr` | 创建、查看、diff、CI 检查、评审、合并、checkout |
| `github_run` | Actions 运行列表、失败日志定位（`log_failed`）、触发、重跑、产物下载 |
| `github_release` | 查看/创建 Release、上传下载资产 |
| `github_secret` | Actions secret 与 variable 管理 |
| `github_api` | 任意 REST/GraphQL 兜底（GHCR 包管理等） |
| `github_task` | 任务级原语：审 PR、诊断 CI、准备发版 |

使用要点：

- 未指定 `repo` 时，工具按当前工作区识别仓库；本工作区需先 `git init` 并添加远端。
- Docker 发布仍由 `.github/workflows/docker.yml` 完成，插件负责触发、看日志、发 Release；GHCR 包查询可用 `github_api`。
- 首次使用前执行 `gh auth status` 确认已登录。

---

## 16. 里程碑

| 里程碑 | 内容 | 出口标准 | 粗估 |
|---|---|---|---|
| M0 | 仓库骨架、go.mod、Makefile、Dockerfile.dev、ci.yml、`neca version` | CI 绿，容器内 `make build` 成功 | 0.5 人日 |
| M1 | config / proc / openssl / index(只读) / meta / table / csv；`list`、`verify`、`export`、`index sync` | 与旧脚本输出 diff 为 0 | 1.5 人日 |
| M2 | pki / issue / reissue / revoke / unrevoke / platform / package / delete；flock；失败清理；Linux/macOS 平台包；安装脚本选项；p12 profile；zip/TTL/downloads | 临时 PKI 全流程；影子测试（`--installers windows`）通过；Linux/macOS 包与 zip TTL 测试通过 | 3 人日 |
| M3 | 向导、`--json`、`--dry-run`、audit、退出码、兼容 flag 双轨；语言包（zh-CN/en-US） | ✅ 已完成（2026-11-15）：JSON/向导/dry-run/audit/兼容 flag/locale 落地；`make check-locales` 通过 | 1.5 人日 |
| M4 | `swanctl --load-creds` 实验与 `--reload-mode creds` | ✅ 已完成（2026-11-15），详见附录 A 实验结果 | 0.5 人日 |
| M5 | v0.1.0 发布、服务器部署、旧脚本保留两周；README 警告/LICENSE/NOTICE 生效；首个正式 Release 后转 public | ✅ 2026-11-15：Docker 部署与全功能测试通过；v0.1.0 + GHCR 已发布；仓库已 public；旧脚本保留观察中 | 1 人日 |
| M6 | 阶段二 Web 面板（只读 + 可选 helper 写能力；`cmd/necad` + `internal/web`；默认 lite + 可选 neca-web 面板包；30 分钟下载 TTL、鉴权/清理、Web 多语言） | M6a 只读面板已实现（代码/测试/镜像验证），待发布；M6b 待开发。规格与实施记录见 [web-panel-development.md](web-panel-development.md) | M6a 约 5 人日；M6b +3 人日 |

> M2 已于 2026-11-15 完成：写命令、Linux/macOS 平台包、p12 profile、zip/TTL/downloads 均落地，`make shadow` 覆盖只读 + 写命令 + issue。

---

## 17. 风险

| 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|
| 重写引入行为偏差 | 中 | 高 | 只读先行 + 影子测试 + 旧脚本保留 |
| Go 工具链缺失拖慢起步 | 高 | 低 | Docker 开发镜像，一次性成本 |
| Windows 模板被改成非 ASCII | 低 | 高 | CI 强制 `check-ascii` |
| 以 root 运行带来的误操作 | 中 | 高 | 默认 `--dry-run` 提示；删除类操作二次确认；审计 |
| 并发写坏索引 | 中 | 中 | 全局 flock + 原子替换 |
| `--load-creds` 实验失败 | 已排除 | — | 2026-11-15 实测通过，默认 `creds`；`restart` 保留为回退 |
| GHCR 推送权限配置错误 | 中 | 低 | 用内置 `GITHUB_TOKEN` + `packages: write` |
| Docker CLI 容器挂载 docker.sock | 中 | 高 | 仅 root 可执行；network_mode none；env/目录 0600/0700；部署文档明确 root 等价风险 |
| AGPL 网络服务源码义务被忽略 | 中 | 中 | README/NOTICE/AGPL 第 13 条提示；分发与 Web 服务前复核 |
| zip 含 p12/pass 泄露 | 中 | 高 | 0700/0600、TTL、鉴权、清理；不提供目录列表 |
| Linux/macOS 脚本未覆盖所有发行版 | 中 | 中 | 默认关闭，提供 NECA_DRY_RUN/NECA_ROOT，发布前在目标系统验证 |
| 语言包 key 漂移 | 中 | 低 | CI `make check-locales`；缺 key 回退 |
| Windows 脚本默认关闭导致回归 | 中 | 中 | M2 影子测试固定 `--installers windows`；默认 none 单独测试 |

---

## 18. 待定事项

1. ~~GitHub 仓库 owner/名称~~：已定 `github.com/hyhdsl/neca`（私有）；
2. 运行镜像：已定 **debian-slim**（内置 openssl，约 80MB；scratch/distroless 无法运行 openssl 子进程）；
3. ~~Go 版本锁定~~：镜像内实测 `go1.25.14`，`go.mod` 声明 `go 1.25`；
4. ~~旧 bash 脚本归档~~：已归档到 `legacy/`（2026-11-15，M0）；
5. ~~许可与 PR 策略~~：已定 **AGPL-3.0-only**；**开发期暂不接受外部 PR**，完成后重新评估（2026-11-15 决定）；
6. **Linux/macOS 一键脚本**：已实现，默认关闭；在 Debian/Ubuntu、RHEL 系、macOS 目标版本完成人工验证后再决定是否默认开放；
7. **下载目录与 TTL**：CLI 与 Web 共用 `NECA_DOWNLOAD_DIR`；Web 鉴权与清理策略在 M6 前确定；
8. **社区语言包**：M3 已实现框架；开发期通过 Issue/fork 提供，PR 待开发完成后开放；
9. **阶段二 Web 权限方案**（§12.2 的三选一）；
10. ~~首个正式 Release 后转 public 的检查清单~~：已完成（2026-11-15）；后续仅保留旧脚本两周观察。

---

## 19. 进度与变更记录

### 19.1 当前进度（2026-11-15）

| 项 | 状态 |
|---|---|
| 仓库 | `github.com/hyhdsl/neca`（public）已发布 v0.1.0；`main` 历史已安全清理 |
| 代码 | ✅ M3 已完成：M1/M2 全部命令 + JSON/dry-run/向导/audit/兼容 flag/语言文件；影子对比 diff=0 |
| 扩展点 | ✅ §3.1 已落地：`IndexStore`/`Source` 接口契约、CLI 命令注册表；其余候选先登记 |
| 平台与打包 | ✅ Windows/Linux/macOS/Android/iOS；Linux/macOS 安装/卸载脚本已实现（默认 none）；zip/TTL/downloads |
| p12 算法 | ✅ M2 已实现 `compat`/`modern` profile，默认兼容不变 |
| 多语言 | ✅ M3：`internal/i18n` + zh-CN/en-US 语言文件、`--lang`/`NECA_LANG`、`make check-locales` |
| 配置 | ✅ v0.2.0：JSON 配置、`--config`/`NECA_CONFIG`、config init/show/validate/export-env；优先级 CLI > env > JSON > 默认值 |
| 许可/公开 | ✅ **AGPL-3.0-only**；**开发期暂不接受外部 PR**（完成后重新评估）；v0.1.0 后已转 public |
| `load-creds` 实验 | ✅ 已完成，`creds` 设为默认重载模式 |
| Docker 开发环境 | ✅ 可用：镜像源已配、`golang:1.25-bookworm` 已拉取 |
| DSH GitHub 集成 | ✅ `dsh-github-flow@0.7.1` 已装，`gh` 已 OAuth 登录 |
| CI/CD 工作流 | ✅ `ci.yml` 已落地，M1/M2 影子对比 + M3 `check-locales`；`release.yml` / `docker.yml` 随 M5 |
| 服务器 | ✅ Docker CLI 容器已部署；v0.2.0 全量验收 96/0（2026-11-15 快照后）：索引 8 行、CRL 5 条、容器未重启、既有 IKE SA 保留；测试后索引哈希 `redacted…redacted`、CRL/meta/clients 恢复基线，临时 CN/下载/配置/审计残留清零 |

### 19.2 变更记录

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-11-15 | 第 1 稿 | 决策 D1–D9、架构、CLI 契约、CI/CD 规划、路线图、实验步骤 |
| 2026-11-15 | 第 2 稿 | 仓库落地（hyhdsl/neca）；load-creds 实测通过并改默认；运行镜像定 debian-slim；补充本机环境实测与镜像源说明 |
| 2026-11-15 | M0 | 仓库骨架落地：Go 模块、`neca version`、Makefile、Dockerfile.dev、`ci.yml`、ASCII 门禁；旧 bash 实现归档到 `legacy/` |
| 2026-11-15 | 扩展点 | 新增 §3.1：索引存储/配置来源接口契约（`internal/core/ports`）与 CLI 子命令注册表；候选扩展点登记，无行为变更 |
| 2026-11-15 | M1 | 只读命令落地：config/proc/openssl/index/meta/table/csv；list/verify/export/index sync；新增 fixture 生成器与 `make shadow`，新旧实现 diff=0 |
| 2026-11-15 | 安全 | 清理仓库中的服务器标识（IP、VPN 内网段、真实域名、组织/CA 名称），统一为占位值；生产值改为环境变量注入，并重写 Git 历史 |
| 2026-11-15 | 第 3 稿 | Linux/macOS 平台包、安装脚本选项、zip/TTL 下载管理、p12 profile、多语言语言包；许可与仓库公开/PR 策略草案；README 顶部警告 |
| 2026-11-15 | M2 | 写命令落地：issue/reissue/revoke/unrevoke/delete/platform/package/downloads；Linux/macOS 包、安装脚本选项、p12 profile、zip/TTL；影子对比覆盖写命令与 issue |
| 2026-11-15 | 第 4 稿 | 许可改为 **AGPL-3.0-only**，开发期暂不接受外部 PR（完成后重新评估）；实现 Linux/macOS 安装/卸载脚本；M3：通用 `--json`/`--dry-run`/`--lang`、交互向导、audit、兼容 flag、语言包与 `check-locales` |
| 2026-11-15 | M5 | Docker CLI 镜像与生产 compose；服务器全流程测试通过（issue/verify/平台包/zip/revoke/unrevoke/reissue/delete/JSON/dry-run/向导/audit）；索引/CRL 已恢复基线；v0.1.0 Release + GHCR 镜像发布；仓库转 public |
| 2026-11-15 | v0.1.1 | 修复 `--lang en-US` 仅翻译帮助、不翻译命令输出的问题；新增 `deploy/neca-wrapper.sh`，转发 `NECA_LANG` 并处理非交互 TTY；服务器镜像升级 |
| 2026-11-15 | v0.2.0 | 新增 JSON 配置文件（`neca.json`）与 `--config`/`NECA_CONFIG`；优先级 CLI > env > JSON > 默认值；`config init/show/validate/export-env`；保持 index/meta/CRL 格式不变 |
| 2026-11-15 | v0.2.0 验收 | 服务器快照后全量测试 **96 通过 / 0 失败**：config/language、五平台 issue/installers、p12、zip、downloads、verify、platform/package、revoke/unrevoke/reload、reissue/delete、JSON/错误/向导/wrapper；测试后索引/CRL/meta/clients 恢复基线，strongSwan 未重启 |
| 2026-11-15 | M6a | 只读 Web 面板、签名会话/空闲超时、CSRF/限流、Web 配置 CLI、Passkey 接口占位、lite/neca-web 双镜像与 compose `web` profile；`go test -race`、镜像内容与 necad 容器健康检查通过；待 v0.3.0 发布与浏览器验收 |

## 20. M6 开发准备（M6a 已实现）

> Web 面板的评估见 [web-panel.md](web-panel.md)，开发规格与 M6a 实施记录见 [web-panel-development.md](web-panel-development.md)。Q1–Q11 已确认；本节以下保留筹备期记录，若与开发文档 §18 不一致，以开发文档为准。
>
> **状态（2026-11-15）**：M6a 只读面板、会话/空闲超时、Web 配置 CLI、Passkey 接口占位、lite/neca-web 双镜像与 compose `web` profile 已实现；M6b helper 未开始。

### 20.1 已验证的输入

- v0.2.0 在服务器完成全量验收（2026-11-15，快照 15:12:34 之后）：最终 **96 项通过 / 0 失败**。
- 覆盖：config/language、dry-run、五平台 issue、installers、compat/modern p12、zip/manifest/no key、downloads TTL/clean、verify、platform add/set、package、export、revoke/unrevoke/幂等/reload、reissue/archive/delete、吊销证书删除拒绝与 purge、JSON 错误、help/version/wizard/wrapper。
- 测试后：索引哈希 `redacted…redacted`、CRL 5 条、证书 8 个、meta 3 个、clients 3 个、archive/downloads 为空；测试 CN、配置、CSV、审计与下载残留清零；strongSwan 容器 healthy 且未重启，既有 IKE SA 保留。
- 生产运行镜像仍为 `neca:v0.2.0`；本次未改变 CLI/core 代码。

### 20.2 推荐范围：只读 Web MVP

M6 按原出口标准先做**只读面板**，不把签发/吊销等写操作放进第一阶段：

- 新增 `cmd/necad` + `internal/web`，复用 `internal/core` 的 list/verify/export 只读逻辑；不复制业务规则。
- `necad` 以**非 root** 运行，只读挂载 `neca.json`、`pki/ca/index.txt`、`pki/meta`、`clients`、`downloads` 和 CA 公钥；**不挂载 `ca.key`、`docker.sock`**。
- 监听 `127.0.0.1:18080`，由 Caddy/Nginx 终止 TLS 并对外提供 HTTPS；不让应用直接暴露公网。
- 页面：概览（证书数/CRL 数/到期提醒）、证书列表与搜索、证书详情、p12 verify、下载列表/下载。
- 鉴权：单管理员；密码只以 Argon2id/bcrypt 哈希形式通过 `NECA_WEB_PASSWORD_HASH` 注入；会话密钥放 0600 文件，使用 HttpOnly + SameSite 的签名 Cookie；登录限流。
- 下载：必须登录；只接受由 sidecar 元数据解析出的下载 ID，不接受任意路径；TTL 30 分钟，过期返回 410/404；后台定时清理；不提供目录列表。
- 多语言：复用 `internal/i18n` 的 zh-CN/en-US；前端使用 Go 模板 + 少量原生 JS，静态资源 embed，避免外部 CDN。
- 配置：只读 `neca.json`；M6 不提供设置写回。
- SQLite：**M6 不引入**。只读查询和签名 Cookie 会话不需要数据库；等写操作/多用户阶段再评估 SQLite（只存会话、任务、归属，不替代 index.txt）。

### 20.3 M6 暂不做

- Web 端 issue/revoke/reissue/delete/package 与设置编辑；
- 多用户/RBAC；
- 内置 TLS（优先反向代理）；
- 把 `ca.key` 暴露给 Web 进程。

如果后续要把写操作放进 Web，再按 §12.2 方案 2 增加 root helper（Unix socket + 命令白名单），Web 进程继续保持非 root。

### 20.4 开发顺序

1. 确认下列 Q1–Q5，冻结 M6 只读契约与 API；
2. 新增 `cmd/necad` + `internal/web` 骨架、只读 config 加载、健康检查；
3. 登录/会话/CSRF 基础设施与限流；
4. 只读 API + Go 模板页面 + 下载 TTL/鉴权；
5. 非 root 容器、反向代理示例、i18n 与安全测试；
6. `go test`、`httptest`、路径遍历/TTL/鉴权/并发测试，服务器灰度验收后发 v0.3.0。

### 20.5 需要确认的决策

| # | 决策 | 推荐 |
|---|---|---|
| Q1 | M6 是否只做只读面板 | **是**，写操作留到下一阶段 |
| Q2 | Web 进程权限 | **非 root + 只读挂载，不挂 docker.sock** |
| Q3 | 对外暴露方式 | **127.0.0.1 + 反向代理 TLS**，不直接暴露 |
| Q4 | 登录凭据与会话 | **env 注入密码哈希 + 0600 会话密钥文件 + 签名 Cookie** |
| Q5 | UI 技术 | **Go 模板 + 原生 JS/embed**，不做 SPA；SQLite 继续延后 |

M6 粗估 4–5 人日；上述 5 项确认后即可开始。

---

## 附录 A：`swanctl --load-creds` 实验步骤

> 目标：验证「重新生成 CRL 后，`swanctl --load-creds` 能否在不重启 charon 的情况下加载新 CRL」。D6 已确认执行，建议维护窗口。

```bash
# 0. 备份
cp -a /opt/ipsec/swanctl/x509crl/ca.crl.pem /root/ca.crl.pem.bak-$(date +%s)
docker exec ipsec-vpn swanctl --list-sas | head -30   # 记录当前会话数

# 1. 用测试 CN 制造一条新的吊销记录（不触发旧脚本的自动重启）
openssl ca -config /opt/ipsec/pki/ca/openssl.cnf -revoke /opt/ipsec/pki/TEST.crt 2>&1 | tail -2
openssl ca -config /opt/ipsec/pki/ca/openssl.cnf -gencrl -out /opt/ipsec/swanctl/x509crl/ca.crl.pem
openssl crl -in /opt/ipsec/swanctl/x509crl/ca.crl.pem -noout -text | grep -c "Serial Number"

# 2. 热加载（不重启）
docker exec ipsec-vpn swanctl --load-creds
docker logs ipsec-vpn --since 2m 2>&1 | grep -iE "crl|certificate" | tail -10

# 3. 判定
docker exec ipsec-vpn swanctl --list-sas | head -30   # 会话数应不变
docker exec ipsec-vpn swanctl --list-crls 2>&1 | head   # 若支持，查看CRL哈希是否变化
```

判定标准：

- **成功**：日志显示新 CRL 已加载，且被吊销的测试证书无法再建立连接，同时既有会话数量不变；
- **失败**：日志无新 CRL 记录，或必须重启才生效 → 保持 `restart` 模式，文档记录结论。

### 实验结果（2026-11-15 03:20 执行）

用临时证书 `NECA-CRLTEST`（序列 `7063A61F…D9935`）做了一次完整验证：

| 步骤 | 结果 |
|---|---|
| 签发测试证书 | 成功，索引新增 V 行，CRL 仍 5 条 |
| 索引置 R + `openssl ca -gencrl` | CRL 变为 6 条，哈希变化 |
| `swanctl --load-creds` | 输出 `loaded certificate from /etc/swanctl/x509crl/ca.crl.pem` |
| charon 日志 | `crl #10:0d is newer - existing crl #10:0b replaced`，即内存中的 CRL 被热替换 |
| 容器状态 | `RestartCount=0`，`StartedAt` 未变，确认未重启 |
| 清理 | 索引恢复 8 行、CRL 恢复 5 条，重新 `--load-creds` 回滚内存 CRL；测试证书与客户包已删除 |

结论：**`swanctl --load-creds` 会重新读取并替换 CRL，吊销可以做到立即生效且不断线**，`neca` 默认使用 `creds` 模式，`restart` 仅作回退。唯一未覆盖的是用被吊销证书实际拨号被拒（需要客户端），但 CRL 内容已确认进入 charon。

---

## 附录 B：产物兼容性清单

| 产物 | 必须一致 |
|---|---|
| 证书 subject | `/C=CN/O=<ORG>/CN=<CN>` |
| 证书扩展 | basicConstraints、keyUsage、extendedKeyUsage=clientAuth、SAN、SKI/AKI |
| p12 | 3DES + SHA1、仅客户端证书、密码 12 位字母数字、无换行 |
| 目录 | `clients/<CN>/{Windows,Android,iOS}` 与文件名 |
| 索引 | 6 字段、TAB、序列号大写、时间 `YYMMDDHHMMSSZ` |
| 输出 | `[OK]/[INFO]/[WARN]/[ERR]` 前缀 |
| 退出码 | 0/1/2/3（见 §6.3） |

---

## 附录 C：参考资料

- strongSwan swanctl：`https://www.strongswan.org/testing/testresults/`（本机为 5.9.8）
- OpenSSL 3.0 `ca`、`pkcs12` 手册
- 现有脚本与说明（已归档）：[legacy/vpn-cert-manager.sh](../legacy/vpn-cert-manager.sh)、[legacy-vpn-cert-manager.md](legacy-vpn-cert-manager.md)
- 技术选型评估：[vpn-cert-manager-rewrite-assessment.md](vpn-cert-manager-rewrite-assessment.md)
