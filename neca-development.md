# neca 开发文档（v0.1 规划稿）

> 项目代号：**neca**（取自 example.com 的 Example + CA）
> 文档版本：2026-11-15 第 2 稿（新增：仓库落地、load-creds 实测、Docker 环境实测）
> 上游评估：[vpn-cert-manager-rewrite-assessment.md](vpn-cert-manager-rewrite-assessment.md)
> 服务端现状：Debian 12 / strongSwan 5.9.8 / OpenSSL 3.0.15 / `/opt/ipsec`（详见评估报告 §2）

---

## 0. 决策记录（本稿锁定）

评估报告推荐的是 Python，本稿按你的最终决定改为 **Go 一步到位**。以下 9 条为本项目已确认的决策，后续所有设计以此为准：

| # | 决策 | 说明 |
|---|---|---|
| D1 | 语言：**Go** | 一步到位：CLI 与未来 Web 面板共用同一套 core，单静态二进制部署 |
| D2 | 软件名：**neca** | 命令 `neca`；模块路径 `github.com/hyhdsl/neca`（仓库已建） |
| D3 | 旧 CLI 双轨 | 旧 `--issue` 等 flag 全部保留为兼容别名，至少保留一个版本周期 |
| D4 | p12 继续走 `openssl` 子进程 | 3DES + SHA1 是 MIUI 兼容性硬需求，重实现风险大于收益 |
| D5 | `ca.key` 保持 `0600 root:root` | 不做组权限放宽；`neca` 以 root 身份执行所有涉及 CA 私钥的操作 |
| D6 | `swanctl --load-creds` 实验：**已执行，结论可用** | 2026-11-15 实测：charon 日志出现 `crl #10:0d is newer - existing crl #10:0b replaced`，热加载生效且未重启容器；吊销默认改为 `creds` 模式 |
| D7 | 开发环境：本地 Docker | 本机无 Go、无 WSL 发行版；用容器获得可复现的构建与测试环境 |
| D8 | 服务器验证：真实环境 | 用临时 PKI 与真实 strongSwan 各验一遍，生产操作一律先 `--dry-run` |
| D9 | GitHub 集成 | 由 DSH 插件 `dsh-github-flow`（复用 `gh` CLI 的 OAuth 登录）承担仓库、PR、Actions 与发布操作 |

D1 与评估报告的偏差需要记录：评估报告给 Python 85.3、Go 83.7，差距仅 1.6 分，属于"按阶段优先级取舍"而非"语言能力差距"。选 Go 的理由是**一次到位**：未来 Web 面板与多用户都复用同一份强类型 core，不再做第二次语言迁移；代价是阶段一的模板与文本处理工作量增加约 30–50%，以及需要先搭 Go 工具链。

---

## 1. 目标与范围

### 1.1 一句话目标

用 Go 重写现有 506 行 bash 证书管理脚本，行为完全兼容，命令名 `neca`，并为阶段二的 Web 面板与阶段三的多用户预留结构。

### 1.2 范围内

- 证书签发、吊销、恢复吊销、重签发、补平台包、改平台标注、删除；
- 列表、p12 自检、CSV 导出、索引同步；
- 交互向导（菜单 0–10 与编号语义不变）；
- 平台包生成：Windows（PowerShell + cmd）、Android（说明 + p12）、iOS（mobileconfig）；
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
│  ├─ cli/                 # 参数解析、子命令、向导（薄）
│  └─ app/                 # 阶段二：HTTP 服务
├─ templates/              # embed 的 PS1 / mobileconfig / 说明文本
└─ testdata/               # 黄金文件与 fixture
```

---

## 5. 依赖策略

| 类别 | 选择 | 理由 |
|---|---|---|
| CLI 解析 | **标准库 `flag` + 子命令 switch** | 10 个子命令，不值得引入 cobra；零依赖、帮助文本可控 |
| 中文宽度 | `golang.org/x/text/width` | 唯一的第三方依赖；`--list` 对齐需要 East Asian Width |
| plist | 手写 XML 生成（`encoding/xml` + `encoding/base64`） | mobileconfig 结构固定，约 100 行；用测试解析回来校验 |
| CSV | `encoding/csv` | 标准库 |
| 文件锁 | `syscall.Flock`（Linux） | 标准库，无依赖 |
| 原子写 | `os.CreateTemp` + `os.Rename` | 标准库 |
| 模板 | `text/template` + `embed` | 标准库；模板随二进制内置 |
| 日志 | `log/slog` | 标准库，结构化输出 |
| 测试 | `testing` + `testdata` | 标准库 |

**总共 1 个第三方模块**（`x/text`），交叉编译无 cgo，`CGO_ENABLED=0`。

---
## 6. CLI 设计

### 6.1 子命令

| 子命令 | 兼容旧写法 | 作用 |
|---|---|---|
| `neca issue <CN> --platforms a,b --days N` | `--issue CN` | 签发新证书 |
| `neca revoke <CN>` | `--revoke CN` | 吊销 |
| `neca unrevoke <CN>` | `--unrevoke CN` | 恢复吊销 |
| `neca reissue <CN>` | `--reissue CN` | 归档旧证书并重签 |
| `neca platform add <CN> --platforms ios` | `--add-platform CN` | 补平台包 |
| `neca platform set <CN> --platforms …` | `--set-platforms CN` | 只改元数据标注 |
| `neca delete <CN> [--yes] [--purge-revoked]` | `--delete CN --yes` | 删除（默认拒删 R 行） |
| `neca list [--json]` | `--list` | 证书列表 |
| `neca verify [CN…] [--json]` | `--verify` | p12 自检 |
| `neca export issued\|revoked [--out F]` | `--export-issued` / `--export-revoked` | CSV 导出 |
| `neca index sync` | `--sync` | 补索引与元数据 |
| `neca`（无参数） | 无参数 | 进入交互向导 |
| `neca version` | `-V` | 版本与构建信息 |

兼容规则（D3）：

- 旧 flag 在 `main` 开头做一次预处理，映射为等价子命令，**行为与文案不变**；
- 未知位置参数仍按 CN 处理（兼容 `./script CN` 风格）；
- 一个版本周期后（v1.0）再考虑移除，移除前先打印一次性弃用提示到 stderr。

### 6.2 通用开关

| 开关 | 作用 | 默认 |
|---|---|---|
| `--json` | 结构化输出，关闭 `[OK]` 等人类文案 | 关 |
| `--dry-run` | 只打印将执行的动作，不落盘、不调 docker | 关 |
| `--no-reload` | 只生成 CRL，不重载引擎 | 关 |
| `--reload-mode restart\|creds` | 重载方式，默认 `creds` | creds |
| `--config <file>` | 读取配置文件（未来用），优先级低于环境变量 | 无 |
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

### 6.5 交互向导

菜单编号与旧版完全一致（1 签发、2 吊销、3 恢复、4 重签、5 加平台、6/7 导出、8 列表、9 同步、10 删除、0 退出）。新增：每个动作执行前显示一行等价命令，便于用户学习 CLI 用法。

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

`config.Load()` 负责读取、补默认值、校验目录可写、校验 CA 存在；缺失时返回退出码 3 的错误。

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

### 8.5 pki：签发与重签

签发流程与旧脚本一致：校验 CN → 生成密钥（0600）→ CSR → 写 `.ext` → `x509 -req` → 索引 → 元数据 → p12 → 平台包。每个失败路径用 `defer` 清理未完成产物（旧脚本会残留密钥）。

### 8.6 crl / engine

- `crl.Gen()`：调用 `openssl ca -gencrl`，先写临时文件再原子替换，`chmod 0644`；
- `engine.Reload()`：按 `NECA_RELOAD_MODE` 二选一：
  - `restart`：`docker restart`，轮询 `swanctl --stats` 最多 40s（旧行为）；
  - `creds`：`docker exec ipsec-vpn swanctl --load-creds`，不重启、不断线（2026-11-15 实测通过，charon 日志确认 CRL 被热替换）。

### 8.7 audit

`/opt/ipsec/pki/audit.log`，每行：`RFC3339 操作者(uid:user) 动作 CN 序列号 结果`。只追加，不阻塞主流程；写失败只告警。

### 8.8 table：中文对齐

按 East Asian Width 计算显示宽度（W/F 记 2），列宽取最大值，输出两空格分隔；与旧 python 实现逐行等价。

---

## 9. 模板与平台包

| 平台 | 产物 | 生成方式 |
|---|---|---|
| Windows | `install.ps1`、`uninstall.ps1`、`connect.ps1`、三个 `.cmd`、`使用说明.txt`、p12/pass/ca.crt | `text/template` + `embed`，占位符 `{{.CN}}`、`{{.Profile}}`、`{{.Server}}` |
| Android | p12/pass/ca.crt、`使用说明.txt` | 同上 |
| iOS | `<CN>.mobileconfig`（CA + 身份 + IKEv2，OverridePrimary=0） | 手写 plist XML，证书用 base64(DER)，UUID 用 `crypto/rand` v4 |

两条硬规则：

1. **Windows 模板必须是纯 ASCII**（PowerShell 5.1 无 BOM 时按 ANSI 读，中文会乱码报语法错误）。用 `make check-ascii` 在 CI 里校验 `templates/win/*` 无 >127 字节。
2. iOS 的 `LocalIdentifier` 取 CN、`RemoteAddress`/`RemoteIdentifier` 取 `SERVER_ID`，与旧实现一致；plist 生成后要用解析器回读校验键值。

---

## 10. 数据与兼容

- 不迁移、不重命名任何目录；新旧实现共用同一份 `index.txt`、`meta/`、`clients/`、`exports/`。
- `index.txt` 是 CRL 的唯一事实源，**不允许**引入第二份证书状态存储；SQLite（阶段三）只存用户、审计、归属。
- 版本切换用 `git` 管脚本与二进制，回退只需换回旧 `vpn-cert-manager.sh`。

---

## 11. 错误处理与回滚

| 场景 | 处理 |
|---|---|
| 签发中途失败 | `defer` 删除已生成的 key/csr/ext/crt，提示可重试 |
| 索引写失败 | 临时文件丢弃，索引保持原样（原子替换保证） |
| gencrl 失败 | 不回滚索引，但**不替换**旧 CRL；提示手工排查 |
| 容器重载失败 | 保留新 CRL，`[WARN]` 提示需手工重启（下次重启自动生效） |
| 并发执行 | `flock` 拿不到锁立即失败（退出码 3），提示已有实例在运行 |

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
- 不记录密码、私钥到日志；错误信息里只出现路径与序列号。

---
## 13. 测试策略

### 13.1 分层

| 层 | 内容 | 依赖 |
|---|---|---|
| 单元测试 | index 解析、meta 读写、表格宽度、CSV 转义、模板渲染、plist 生成 | 纯 Go，无外部命令 |
| 黄金测试 | `testdata/golden/` 存期望输出，`-update` 更新；覆盖 list/export/verify 文本 | 需要 fixture 证书 |
| 集成测试 | 临时 PKI（`t.TempDir()` + `PKI_DIR` 重定向）跑完整签发/吊销/重签 | 需要 openssl；docker 用 stub |
| 影子测试 | 同一 fixture 跑旧 bash 与新 neca，比对产物与索引三元组 | 需要 bash + openssl |
| 手工验收 | 真实服务器、真实手机导入 p12、真实 VPN 连接 | 人工 |

### 13.2 fixture 生成

`testdata/gen-fixture.sh`：`mktemp -d` 生成一套独立 CA（RSA 2048 加速）+ 两张证书，所有路径写入 `fixture.env`；测试用 `t.Setenv` 注入。**任何测试都不得触碰 `/opt/ipsec`**。

### 13.3 影子对比（迁移期最重要）

同一份 fixture 下依次执行，逐项比对：

```text
issue TMP01 windows,android → list → verify → add-platform ios → export issued
  → revoke → export revoked → unrevoke → reissue → delete
```

比对项：`--list` 表体（忽略列宽）、导出 CSV（去时间戳列）、`index.txt` 的 (状态,序列号,CN) 集合、CRL 序列号集合、`clients/` 目录树与文件大小。差异必须为 0 才允许合并。

### 13.4 CI 门禁

- `go vet ./...`、`gofmt -l` 无输出、`go test -race ./...`；
- `make check-ascii`（Windows 模板纯 ASCII）；
- `make build` 产出 linux/amd64 与 linux/arm64 两个二进制；
- 提交信息遵循 Conventional Commits。

---

## 14. 开发环境（D7）

本机没有 Go、没有 WSL 发行版，**用 Docker 作为唯一构建环境**，保证与服务器同为 Debian 家族。

### 14.1 镜像

`Dockerfile.dev`：`golang:1.25-bookworm` + `openssl` + `git` + `ca-certificates` + `make`。

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
| `make shadow` | 跑影子对比（需要 bash + openssl） |
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
| 仓库 | `github.com/hyhdsl/neca`，**私有**，已创建并推送基线（2026-11-15） |
| 默认分支 | `main`，保护：必须 PR + CI 通过 |
| 分支命名 | `feat/…`、`fix/…`、`chore/…` |
| 提交信息 | Conventional Commits（`feat:`、`fix:`、`chore:`、`docs:`） |
| 版本 | SemVer；tag `v0.1.0` 起 |
| 许可证 | 私有仓库可不加；若开源建议 MIT |

### 15.2 工作流

| 文件 | 触发 | 内容 |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR | setup-go、`go vet`、`go test -race`、`make check-ascii`、构建 amd64 |
| `.github/workflows/release.yml` | tag `v*` | 交叉编译 amd64/arm64，生成 checksum，创建 GitHub Release |
| `.github/workflows/docker.yml` | tag `v*` / 手动 | buildx 构建，推送 `ghcr.io/hyhdsl/neca:<tag>` 与 `:latest` |

所需权限与密钥：

- `release.yml`：`permissions: contents: write`，用内置 `GITHUB_TOKEN`，无需额外 secret；
- `docker.yml`：`permissions: packages: write` + `GITHUB_TOKEN` 登录 `ghcr.io`，**无需 PAT**；
- 若未来需要跨仓操作，再考虑 GitHub App，不要引入长期 PAT。

当前状态（2026-11-15）：三个工作流尚未创建——`ci.yml` 随 M0 落地，`release.yml` 与 `docker.yml` 随 M5 落地。

### 15.3 发布流程

```text
feature 分支 → PR（CI 全绿）→ squash merge 到 main
  → 打 tag v0.1.0 → release.yml 产出二进制 + Release
  → docker.yml 推送镜像 → 服务器下载二进制替换 /usr/local/bin/neca
```

回滚：保留上一个二进制为 `/usr/local/bin/neca.prev`，或直接从 Release 下载旧版本。

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
| M2 | pki / issue / reissue / revoke / unrevoke / platform / delete；flock；失败清理 | 临时 PKI 全流程通过；影子测试通过 | 2 人日 |
| M3 | 向导、`--json`、`--dry-run`、audit、退出码、兼容 flag 双轨 | 人工回归清单全过 | 1 人日 |
| M4 | `swanctl --load-creds` 实验与 `--reload-mode creds` | ✅ 已完成（2026-11-15），详见附录 A 实验结果 | 0.5 人日 |
| M5 | v0.1.0 发布、服务器部署、旧脚本保留两周 | 生产连续两周无回退 | 0.5 人日 |
| M6 | 阶段二 Web 面板（`cmd/necad` + `internal/web`） | 只读面板上线，与 CLI 共用 core | 3–4 人日 |

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

---

## 18. 待定事项

1. ~~GitHub 仓库 owner/名称~~：已定 `github.com/hyhdsl/neca`（私有）；
2. 运行镜像：已定 **debian-slim**（内置 openssl，约 80MB；scratch/distroless 无法运行 openssl 子进程）；
3. Go 版本锁定：镜像内实测 `go1.25.14`，`go.mod` 声明 `go 1.25`；
4. 是否把旧 bash 脚本放入 `legacy/` 一并归档；
5. 阶段二 Web 的权限方案（§12.2 的三选一）。

---

## 19. 进度与变更记录

### 19.1 当前进度（2026-11-15）

| 项 | 状态 |
|---|---|
| 仓库 | `github.com/hyhdsl/neca`（私有）已创建，`main` 已推送基线（`fbe67d6`、`16338b0`） |
| 代码 | 未开始（M0 之前） |
| `load-creds` 实验 | ✅ 已完成，`creds` 设为默认重载模式 |
| Docker 开发环境 | ✅ 可用：镜像源已配、`golang:1.25-bookworm` 已拉取 |
| DSH GitHub 集成 | ✅ `dsh-github-flow@0.7.1` 已装，`gh` 已 OAuth 登录 |
| CI/CD 工作流 | ⏳ 未创建（M0 落 CI，M5 落 release/docker） |
| 服务器 | 生产状态已还原：索引 8 行、CRL 5 条、容器未重启 |

### 19.2 变更记录

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-11-15 | 第 1 稿 | 决策 D1–D9、架构、CLI 契约、CI/CD 规划、路线图、实验步骤 |
| 2026-11-15 | 第 2 稿 | 仓库落地（hyhdsl/neca）；load-creds 实测通过并改默认；运行镜像定 debian-slim；补充本机环境实测与镜像源说明 |

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
- 现有脚本与说明：[vpn-cert-manager.sh](vpn-cert-manager.sh)、[vpn-cert-manager.md](vpn-cert-manager.md)
- 技术选型评估：[vpn-cert-manager-rewrite-assessment.md](vpn-cert-manager-rewrite-assessment.md)
