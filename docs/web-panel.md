# neca Web 面板设计评估（M6）

> 状态：**设计已确认；M6a/M6b 已实现、服务器部署和 Playwright/axe 验收完成，预览包为 `v0.3.0-preview.3`**。日期：2026-11-15。
> 本文回答 Web 面板的访问控制、只读开关、证书访问、鉴权、UI 与后续 SQLite 迁移问题。实现以本文确认后的版本为准。
> 实施规格见 [web-panel-development.md](web-panel-development.md)。

## 1. 结论摘要

| 问题 | 结论 |
|---|---|
| Q1 后台只读开关 | **可以**。在 neca.json 增加 web.readOnly，并新增 neca web readonly status/on/off 子命令；服务端强制拦截写请求。只读关闭后需要独立 root helper，Web 进程本身始终非 root。 |
| Q2 权限调整 | Web 进程只读挂载、无 ca.key、无 docker.sock；写操作交给 Unix socket 的 root helper。 |
| Q3 仅 VPN/指定证书访问 | 不引入第三方 VPN 客户端。单管理员用 OpenSSH 本地转发直连回环；浏览器场景使用系统原生 IKEv2 + 反向代理 mTLS 专用管理员证书 + VPN 网段防火墙。 |
| Q4 鉴权调整 | mTLS（可选）+ 管理员密码哈希 + 签名 Cookie + CSRF + 限流；空闲超时默认 30m、可配至 24h、绝对会话上限 24h；web-secrets 由 root 属主、按文件授权/挂载；只读开关只允许 root CLI 修改；Passkey 先保留接口。 |
| Passkey | 可行但依赖 HTTPS + 稳定域名/RP ID；M6 仅保留接口、默认关闭，先作为第二因素。 |
| Q5 UI/SQLite | Go 模板 + CSS 变量，深浅色蓝白/蓝黑主题；不以截图作为像素级验收依据，使用 data-testid/DOM/Playwright。M6 不引入 SQLite，但预留 Store 接口与迁移路径。 |
| Docker 发布 | 只发布 lite 默认镜像 + 可选 neca-web 面板包；lite 不替换即可增量安装 Web；停止/移除 Web 组件即回到 lite。 |
| 仓库边界 | 建议保持 monorepo：共享 core、同版本、同 Docker base；独立仓库收益不足以抵消成本。 |

## 2. 总体架构

~~~text
浏览器
  │ HTTPS + 可选 mTLS
  ▼
Caddy/Nginx 反向代理（仅 VPN 接口或回环）
  │ 127.0.0.1:18080
  ▼
necad（非 root，只读）
  ├── 只读 core：list / verify / export / downloads
  ├── 会话、登录、CSRF、限流
  └── 可选写请求 ──Unix socket──> neca-web-helper（root）
                                      ├── 固定命令白名单
                                      ├── CA 私钥 / docker.sock
                                      └── 审计
~~~

关键边界：

- necad 永不读取 ca.key，不挂载 docker.sock；
- helper 只接受结构化操作，不接受任意 shell、路径或参数；
- 只读模式由 necad 的 HTTP 中间件强制，helper 再做一次 defense in depth；
- 反向代理是唯一对外入口，Web 不监听 0.0.0.0。

## 3. Q1：后台只读开关与 CLI

### 3.1 可行性

可以。只读不是 UI 隐藏按钮，而是**配置 + 服务端路由级强制**：

- 默认 web.readOnly = true；
- 所有非 GET/HEAD 请求在中间件层返回 403；
- UI 根据能力接口隐藏写操作；
- 即使直接构造请求也无法越权。

### 3.2 配置

在 neca.json 的非敏感 web 段保存：

~~~json
{
  "web": {
    "enabled": true,
    "listen": "127.0.0.1:18080",
    "readOnly": true,
    "idleTimeout": "30m",
    "adminCn": "webadmin-01"
  }
}
~~~

密码哈希、会话密钥、mTLS 私钥/token 不写入 neca.json。

### 3.3 新增 CLI 子命令

建议命令：

~~~bash
neca web status                # lite 可用：配置状态；已装 Web 时含运行状态
neca web readonly status       # readonly | readwrite
neca web readonly on           # 强制只读
neca web readonly off          # 需要 Web 包 + helper；lite 内拒绝
neca web idle-timeout 30m      # 空闲超时；默认 30m，最大 24h
~~~

行为：

- lite 只带 status/readonly 管理命令，不带 necad/helper；Web 服务端由 neca-web 包的 necad 进程启动，不通过 lite CLI 的 serve 子命令；
- lite 中 readonly off 返回“Web 组件未安装”，避免在无 helper 时产生错误的可写配置；
- full 中 lite CLI 通过挂载的 helper 控制 socket 检查能力；socket 缺失或版本不匹配时同样拒绝；
- on/off 由 root CLI 执行，原子写 neca.json；新文件 0600，启用 Web 时为 `root:neca-web 0640`；
- 运行中的 necad 通过 SIGHUP 或配置文件监视重新加载；无法热加载时提示重启；
- off 前检查 helper 可用、socket 权限正确、CA 私钥存在；
- off 失败时保持 readOnly=true，不能出现“配置已关闭但写能力未就绪”的半状态；
- 切换动作写审计日志，记录操作者、时间、旧值/新值。

### 3.4 off 的写能力

Web 进程始终非 root，因此 readOnly=false 不意味 Web 直接获得 CA 私钥。写请求通过 helper：

- helper 是独立 root 进程，监听 /run/neca/web-helper.sock；
- socket 0660，necad 通过专用组访问；
- 只允许预定义操作：issue / revoke / unrevoke / reissue；delete 默认不开放；
- 参数经过 schema 校验（CN 正则、天数范围、平台枚举）；
- 每次操作串行执行，复用 CLI 的 flock；
- 结果返回结构化 JSON，不返回私钥/p12 密码；
- helper 自身也检查 web.readOnly，防止绕过。

建议分两阶段：

| 阶段 | 内容 | 估算 |
|---|---|---|
| M6a | 只读面板、登录、下载、readonly on/off、无 helper 时 off 被拒绝 | 5 人日 |
| M6b | root helper + issue/revoke/unrevoke/reissue 写操作 | +3 人日 |

如果 Q1 要求立即具备 Web 写操作，M6 估算调整为 8 人日，并且必须先完成 helper 的安全评审。

## 4. Q2：权限与进程边界

- necad 以专用非 root 用户运行；
- 只读挂载 neca.json、pki/ca/index.txt、pki/meta、clients、downloads、CA 公钥；
- 不挂载 pki/ca.key、不挂载 /var/run/docker.sock；
- 不把 /opt/ipsec 整体挂成可写；
- Web 只监听回环，反向代理负责外部 TLS；
- helper 是唯一 root 边界，最小权限、固定白名单、独立审计；
- 任何目录参数都不来自 URL，下载只使用 sidecar 中已登记的 ID。

## 5. Q3：VPN/指定证书访问

### 5.1 方案比较

| 方案 | 安全性 | 便利性 | 结论 |
|---|---|---|---|
| 只限制 Web 到 VPN 网段 + 登录 | 中 | 高 | 任何 VPN 用户都能访问登录页，不满足“指定证书” |
| 按证书固定 VPN 客户端 IP，只放行该 IP | 中高 | 低 | 依赖地址池/静态映射，易漂移，不建议作为唯一措施 |
| 系统原生 IKEv2 + 反向代理 mTLS + 专用管理员证书 + CN 白名单 | 高 | 中 | 满足“只有该证书能访问 Web”；推荐浏览器场景，不引入第三方 VPN |
| OpenSSH 本地端口转发 | 很高 | 中高 | 单管理员最简、暴露面最小；系统原生，优先推荐 |
| 用同一张 VPN 客户端证书兼做浏览器 mTLS | 中高 | 低 | 技术上可行，但浏览器导入、移动端、私钥暴露和角色混淆问题多，不推荐 |

### 5.2 推荐

**单管理员、日常运维：**

- Web 绑定 127.0.0.1:18080；
- 通过系统原生 OpenSSH 本地端口转发访问：ssh -L 18080:127.0.0.1:18080 ...；
- 不使用 Tailscale/WireGuard 等第三方 VPN 客户端；
- 不向公网或 VPN 业务网段开放 Web 端口。

**必须从浏览器直接访问：**

1. 使用系统原生 IKEv2 客户端连接；反向代理只绑定 VPN 接口，防火墙只允许 VPN 网段；
2. 反向代理启用 mTLS，校验证书链和 CN；
3. 单独签发管理员客户端证书（不要复用普通 VPN 用户证书），CN 写入白名单；
4. 代理把已验证的 CN 通过受信任方式传给 necad（或由 necad 自己验证客户端证书）；
5. 再叠加管理员登录密码，避免“证书即全部权限”；
6. 管理员证书吊销后，代理立即拒绝；证书私钥只存在管理员设备，不进服务器仓库。

### 5.3 能不能“签一张证书，只让这张证书连 VPN 才能访问 Web”？

- “只让这张证书连 VPN”本身需要 VPN 侧的用户/证书授权与地址分配；当前 strongSwan 配置不能仅凭 Web 侧实现。
- “只让这张证书访问 Web”更适合用 **mTLS CN 白名单** 实现，而不是依赖 VPN 连接状态。
- 最佳组合：**VPN 网段限制 + 反向代理 mTLS 专用管理员证书 + 登录密码**。
- 单管理员更推荐 **OpenSSH 本地转发**（系统原生），比浏览器证书更容易维护；不引入第三方 VPN 客户端。

## 6. Q4：鉴权与会话调整

### 6.1 认证链

~~~text
TLS/mTLS（反向代理，可选）
  → 来源 IP/网段检查
  → 管理员登录（密码哈希）
  → 签名会话 Cookie（HttpOnly + Secure + SameSite=Lax）
  → 空闲超时 / 绝对会话上限
  → CSRF token（写操作强制）
  → 限流 / 登录失败审计
~~~

- 密码只以 Argon2id 或 bcrypt 哈希形式通过 NECA_WEB_PASSWORD_HASH 注入；
- 会话密钥放 0600 文件，支持轮换；不使用默认值；
- 代理信任：necad 只接受来自回环的连接；若读取 X-Forwarded-* 或客户端证书头，必须验证代理共享密钥并清除客户端伪造头；
- 登录失败限流，避免暴力破解；
- 只读/读写切换只能由 root CLI 完成，Web 不提供“关闭只读”的 API；
- 下载必须登录，下载 ID 不透明，TTL 30 分钟，过期 410/404。

### 6.2 空闲超时与会话上限

- neca.json 增加 web.idleTimeout，默认 30m，允许 5m–24h；超过 24h 拒绝；
- 另设绝对会话上限 24h（从登录时刻起），空闲超时和绝对上限任一到达即失效；
- CLI：neca web idle-timeout 30m / status；只允许 root CLI 修改，Web 不提供自行修改入口；
- 服务端按认证请求更新 lastSeen；静态资源请求不单独续期，避免后台轮询延长会话；
- 过期后页面跳转登录，API 返回 401；会话 Cookie 的 Max-Age 不超过绝对上限；
- 修改后通过 SIGHUP/配置监视生效；无法热加载时提示重启。

### 6.3 Passkey/WebAuthn（先评估，保留接口）

**可行，但有前置条件，M6 先不实现登录，只保留接口和配置开关。**

前置条件：

- WebAuthn 需要 HTTPS 安全上下文；
- 需要稳定的 RP ID（通常是管理面板域名）；仅用 IP 地址访问通常不可用；
- 如果只通过 OpenSSH 本地转发访问 http://127.0.0.1:18080，localhost 可能被视为安全上下文，但 passkey 绑定 localhost，不适合多设备/远程管理员；生产仍建议域名 + 反向代理 TLS；
- 浏览器/设备需支持 WebAuthn；硬件密钥、平台 passkey 和同步 passkey 的可用性不同；
- 必须设计凭据丢失、恢复、撤销和审计流程。

建议策略：

- 默认 web.passkey.enabled=false；
- 第一版只作为**第二因素**：密码 + passkey，密码仍是恢复路径；不建议一开始做无密码登录；
- 注册必须在已登录会话中进行，要求 user verification；挑战短时有效；
- 凭据存储先走 PasskeyStore 接口，M6 可用 0600 JSON 文件实现，后续可迁移到 SQLite；
- 记录 credential ID、public key、sign count、created/last used、label；不记录私钥；
- 只允许已注册凭据；克隆检测、RP ID/Origin 校验、挑战一次性；
- 审计登录成功/失败、注册、删除、恢复。

接口草案：

~~~go
type PasskeyCredential struct {
    ID          []byte
    PublicKey   []byte
    SignCount   uint32
    Label       string
    CreatedAt   time.Time
    LastUsedAt  time.Time
}

type PasskeyStore interface {
    Put(ctx context.Context, c PasskeyCredential) error
    Get(ctx context.Context, id []byte) (PasskeyCredential, error)
    List(ctx context.Context) ([]PasskeyCredential, error)
    Delete(ctx context.Context, id []byte) error
}

type PasskeyAuthenticator interface {
    BeginRegistration(ctx context.Context, adminID string) (any, error)
    FinishRegistration(ctx context.Context, adminID string, response []byte) error
    BeginLogin(ctx context.Context) (any, error)
    FinishLogin(ctx context.Context, response []byte) (string, error)
}
~~~

M6a 只定义接口与 feature flag，不引入 WebAuthn 依赖；M6b/后续版本再决定是否实现。候选库需单独做许可证与安全评审，不提前锁定。

### 6.4 审计

记录：

- 登录成功/失败、来源 IP、证书 CN（如启用 mTLS）；
- readonly on/off；
- 下载创建/访问/过期；
- 写操作（helper）：操作者、CN、序列号、结果；
- 绝不记录密码、私钥、p12 密码、会话密钥。

### 6.5 web-secrets 目录所有权

**建议：Web 服务用户不是 web-secrets 目录的属主。**

两种方案：

| 方案 | 做法 | 风险 |
|---|---|---|
| 是：Web 用户为属主 | 目录 0700、文件 0600，全部由 Web 用户拥有 | 简单，但 Web 应用被攻破后，该用户可以读取目录内所有密钥、数据库密码和 passkey 凭据 |
| 否：root 为属主，Web 用户按文件/组访问 | 目录 root:neca-web 0710；只挂载/授权 Web 真正需要的文件 | 稍复杂，但能把 Web 不需要的密钥隔离在目录外 |

推荐细则：

- 目录：root:neca-web，0710（Web 组可进入已知路径，但不能列出目录）；
- 会话签名密钥：root:neca-web，0640，只读挂载给 necad；
- Passkey 凭据文件：root:neca-web，0660（M6 默认关闭，后续再启用写权限）；
- 未来 web.db：root:neca-web，0660；数据库账号使用最小权限；
- mTLS 私钥：root:proxy，0600，只给反向代理，不挂载给 Web；
- helper 控制 socket：root:neca-web，0660，由 helper 持有；
- 密码哈希走环境变量或单独的 0640 文件，不放入可写 JSON；
- Docker 使用**单文件 bind mount**，不要为了省事把整个 web-secrets 目录挂进 Web 容器。

必须承认的固有限制：

- 会话签名密钥必须能被 necad 读取，否则无法验证 Cookie；Web 被攻破时攻击者仍能读到它并伪造会话；
- Passkey 凭据文件若需要 Web 注册/删除，Web 用户必须可读写；被攻破时可篡改凭据；
- 因此 root 属主不是“防住 Web 被攻破”，而是**把 Web 不需要的密钥从它的可读范围移除**；
- 会话密钥应支持轮换，Web 失效时间短（默认 30m），异常时立即轮换并重启 Web；
- helper 是独立 root 边界，即使 Web 被攻破，也只能调用白名单操作，不能读取 CA 私钥。

结论：**采用“否”方案**——root 属主、组权限、按文件挂载、最小权限；不接受 Web 用户拥有整个 web-secrets 目录。



## 7. Q5：UI、配色与可测试性

### 7.1 DeepSeek 图像识别的边界

DeepSeek 可以辅助阅读截图、识别大致区域和明显布局问题，但**不应假设它能稳定做到像素级位置判断、颜色精确比对或所有状态的视觉回归**。截图会被缩放、抗锯齿、字体渲染和暗色主题影响。

因此：

- 页面使用语义化 HTML、ARIA 标签、稳定 data-testid；
- 验收用 DOM 快照、Playwright 选择器、计算样式和 axe 可访问性检查；
- 截图只用于人工评审和差异展示，不作为唯一断言；
- UI 位置通过布局系统与响应式断点固定，不依赖模型“看懂”。

### 7.2 视觉方向

- 主色：蓝色 #2563EB（浅色）/ #60A5FA（深色强调）；
- 浅色：背景 #F8FAFC、卡片 #FFFFFF、文字 #0F172A、边框 #CBD5E1；
- 深色：背景 #0B1220、卡片 #111827、文字 #E5E7EB、边框 #334155；
- 状态色：成功 #16A34A、警告 #D97706、危险 #DC2626；深色主题对应提高亮度；
- 对比度满足 WCAG AA；不能只靠颜色表达状态，必须辅以文字/图标；
- 深浅色：CSS 变量 + prefers-color-scheme，提供显式切换并保存到 localStorage；
- 图表/表格避免过度装饰，优先可读性和键盘操作。

### 7.3 页面与组件

- 概览：证书总数、有效/吊销、CRL 条目、最近到期；
- 证书列表：搜索、状态筛选、平台、到期；
- 详情：subject/serial/到期/平台/verify 结果/下载；
- 登录、错误、空状态、加载状态、无权限状态；
- 移动端单列布局，桌面端侧栏 + 内容区。

## 8. SQLite 延后与迁移设计

M6 不引入 SQLite：

- 会话使用签名 Cookie，不依赖数据库；
- 下载元数据继续使用现有 sidecar 文件；
- 设置继续使用 neca.json；
- 审计继续追加文件。

为未来迁移预留：

- 定义 WebStore 接口（会话、下载 token、操作任务），M6 使用无状态/文件实现；
- 未来 SQLite 只保存会话、任务、角色、下载 token 等**动态状态**，不替代 index.txt、meta 或 CRL；
- 数据库使用独立文件（如 /opt/ipsec/web.db），不放进 PKI 目录；
- 使用 schema_migrations 表，迁移脚本随版本发布；
- 推荐 pure-Go modernc.org/sqlite；WAL + busy_timeout；
- 通过配置开关启用，不改变 CLI 的 JSON 配置事实源；
- 迁移前提供导出/回滚说明，避免把证书事实迁入数据库。

## 9. API 草案（只读）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /healthz | 存活检查，仅回环 |
| GET | /login | 登录页 |
| POST | /login | 登录，限流 + CSRF |
| POST | /logout | 退出 |
| GET | /api/v1/certs | 列表/搜索/过滤 |
| GET | /api/v1/certs/{cn} | 详情 |
| GET | /api/v1/certs/{cn}/verify | p12 校验结果 |
| GET | /api/v1/downloads | 下载列表 |
| GET | /api/v1/downloads/{id} | 下载，TTL 校验 |
| GET | /api/v1/capabilities | readOnly 与可用操作 |

写 API 在 M6b 增加，且全部要求 CSRF、会话、readOnly=false 和 helper 可用。

## 10. 验收标准

- 只读模式下所有写方法返回 403，UI 无写控件；
- readonly on/off 只允许 root CLI，切换成功/失败均不产生半状态；
- Web 进程非 root，无法读取 ca.key，无法访问 docker.sock；
- 未登录不能访问证书详情/下载；下载过期返回 410/404；
- 路径遍历、伪造 CN 头、CSRF、暴力登录测试通过；
- mTLS/反向代理/TLS 配置文档可复现；
- 空闲 30m 后会话失效，配置上限 24h 生效；API 返回 401；
- Passkey feature flag 默认关闭，未实现前不注册登录路由；只验证接口/配置可编译；
- 深浅色主题对比度通过自动化检查；
- 移动端与桌面端关键页面通过 Playwright 截图与 DOM 断言；
- go test -race、httptest、容器灰度验收通过。

## 11. Docker 发布形态：lite + Web 面板包

### 11.1 结论

可以。推荐**只发布 lite CLI 镜像 + 可选的 neca-web 面板包**，不发布 full 镜像：

- **lite**：唯一默认镜像，只包含 neca CLI 与 Web 配置管理子命令，不包含 Web 服务端；
- **neca-web**：可选面板包（独立 Docker 镜像），只增加 necad、helper 与 Web 模板/静态资源；
- 部署 full = lite 容器 + neca-web 容器；不替换 lite 镜像；
- lite → full：只拉取 neca-web 并启用 compose profile，不迁移 PKI；
- full → lite：停止/移除 neca-web 容器即可，lite 容器和镜像原样保留。

这比“lite/full 两个完整镜像”更符合“选组件安装”：lite 始终是基础，Web 是可选附加组件。

### 11.2 镜像与标签

| 标签 | 内容 | 默认 |
|---|---|---|
| ghcr.io/hyhdsl/neca:v0.3.0 | lite：CLI + web 配置管理，无 Web 服务端 | 是 |
| ghcr.io/hyhdsl/neca-web:v0.3.0 | Web 面板包：necad + helper + Web 模板/静态资源 | 否 |
| ghcr.io/hyhdsl/neca:latest | 最新 lite | — |
| ghcr.io/hyhdsl/neca-web:latest | 最新 Web 面板包 | — |

- 不再发布 full 镜像；
- neca-web 与 lite 共享 base 层（openssl、ca-certificates、docker CLI），但不包含 lite CLI；
- neca-web 只包含 necad/helper 与 Web 资源；写 helper 使用单独 entrypoint；
- 两个镜像必须使用相同 vX.Y.Z 版本。

### 11.3 Dockerfile 规划

~~~dockerfile
FROM debian:bookworm-slim AS base
# openssl、ca-certificates、docker CLI

FROM base AS lite
COPY neca-linux-amd64 /usr/local/bin/neca
ENTRYPOINT ["neca"]

FROM base AS web
COPY necad-linux-amd64 /usr/local/bin/necad
COPY neca-web-helper-linux-amd64 /usr/local/bin/neca-web-helper
ENTRYPOINT ["/usr/local/bin/necad"]
~~~

- neca-web 通过 FROM base 与 lite 共享基础层，拉取时只增加 Web 层；
- lite 镜像没有 necad/helper，Web 代码不进入默认镜像；
- neca-web 不包含 lite CLI；Web 服务只由 necad 启动；
- helper 与 necad 在同一镜像中使用不同 command/entrypoint，运行权限不同。

### 11.4 Compose 组件选择

~~~yaml
services:
  neca:
    image: ${NECA_IMAGE:-ghcr.io/hyhdsl/neca:v0.3.0}
  web:
    image: ${NECA_WEB_IMAGE:-ghcr.io/hyhdsl/neca-web:v0.3.0}
    profiles: ["web"]
  web-helper:
    image: ${NECA_WEB_IMAGE:-ghcr.io/hyhdsl/neca-web:v0.3.0}
    profiles: ["web-write"]
~~~

- 默认：只启动 neca（lite）；
- full 只读：NECA_WEB_IMAGE 指向同版本 neca-web，启用 --profile web；
- full 可写：再启用 --profile web-write；helper 是唯一挂载 CA 私钥/docker.sock 的容器；
- Web/helper 不进入默认 profile，装了 neca-web 也不会自动暴露；
- lite 的 CLI 包含 neca web readonly/status 管理子命令，因此即使未装 Web 也能预置配置；Web 服务由 neca-web 包的 necad 启动；
- full compose 把 helper 控制 socket 只读挂载到 lite CLI；lite 默认没有该 socket，因此 readonly off 被拒绝；安装 Web/helper 后可校验并关闭只读；
- necad 在 readOnly=false 但 helper 不可用时必须 fail closed（拒绝写端点或直接启动失败），不能只改配置就开放写；
- necad 启动时检查自身版本与 lite/NECA 的版本标签，不匹配必须快速失败。

### 11.5 迁移路径

| 方向 | 操作 | 数据影响 |
|---|---|---|
| lite → full | 拉取 neca-web；设置 NECA_WEB_IMAGE；初始化 web secrets；启动 web profile | 不替换 lite，不迁移 PKI |
| full → lite（停用） | docker compose --profile web --profile web-write stop web web-helper | lite 原样保留；Web 不可用 |
| full → lite（移除） | 停用组件；删除 web/helper 容器、web secrets 与可选的 web 专属数据 | 保留 PKI、index、meta、clients、downloads |
| full 升级 | 同时更新 neca 与 neca-web 到同一新版本；重启 web/helper | PKI 不变；必要时做 Web 数据迁移 |
| 回滚 | 切回旧 neca-web 标签；需要时恢复 web secrets | 版本兼容期内无数据迁移 |

关键点：

- lite 镜像不包含 Web 运行时，移除 Web 后无需更换 lite 镜像；
- full→lite 不会清空 /opt/ipsec，也不会自动删除证书/下载；
- web secrets 与 PKI 分开存放（如 /opt/ipsec/web-secrets，0600），lite 不读取；
- 未来 SQLite 的 web.db 放在 Web 专属路径，lite 不读取、不删除；
- lite 配置中的 web 段可以被忽略；安装 Web 后复用同一配置。

### 11.6 发布流水线调整

当前 docker.yml 只构建一个镜像，M6 需要：

1. 构建并推送 lite 镜像 neca:vX；
2. 以 lite 为基构建并推送 neca-web:vX（同一 buildx 流程）；
3. 两个镜像都推送 amd64/arm64；
4. 生成各自 tags、digest、SBOM 与版本标签；
5. release notes 明确“默认 lite；Web 是可选组件”；
6. CI 增加 lite、web 只读、web+helper 写模式三组冒烟测试；
7. 如需要，发布 neca-web 的离线镜像 tar 作为镜像包，而不是可变的运行时 tar 安装包。

### 11.7 测试矩阵

| 场景 | 必须验证 |
|---|---|
| lite CLI | 与 v0.2.0 全量测试一致；无 necad/helper |
| lite + web 只读 | 登录、列表、下载、readonly；Web 非 root，无 ca.key/docker.sock |
| lite + web + helper | helper 白名单、CSRF、审计；只读开关拒绝写 |
| lite→full | 只拉 neca-web，lite 不重启/不换镜像；无 PKI 迁移 |
| full→lite | 停用/移除 Web 后 lite 仍可完整管理证书 |
| 版本不匹配 | lite 与 neca-web 版本不一致时启动快速失败 |
| 回滚 | 旧 neca-web、旧 secrets、旧配置可用 |
| 安全 | 默认无 Web 端口；Web 包不自动暴露；helper 最小权限 |

### 11.8 结论与限制

- 这种方案最贴合“默认 lite、按组件安装 Web”的要求；
- lite→full 是真正的增量：只拉 neca-web 新增层，不替换 lite；
- full→lite 也是真正的移除：停止/删除 Web 组件即可，lite 镜像仍在；
- 代价是镜像数量仍为两个、compose 需要 profiles、Web 与 lite 必须同版本；
- 不建议把 Web 二进制以 tar 形式复制进运行中的 lite 容器：容器重建后丢失，且不可审计；应发布 neca-web 镜像或离线镜像 tar。

## 12. 仓库边界：Web 是否独立仓库

### 12.1 建议

**M6/M6b 阶段继续放在 neca 主仓库，不拆分 Web 仓库。**

Web 面板不是独立产品，而是 neca core 的一个可选展示层：

- 直接复用 internal/core 的 list/verify/export/downloads；
- 需要与 lite 共用配置 schema、版本号和 Docker base 层；
- lite/neca-web 镜像必须同版本、同 commit；
- 当前代码量和团队规模不足以抵消跨仓库成本。

独立的 Web 仓库不会自动提高线上安全性；真正的安全边界是 **独立容器、非 root、无 ca.key/docker.sock、helper 白名单**，这些在 monorepo 中同样可以实现。

### 12.2 对比

| 维度 | monorepo（推荐） | 独立仓库 |
|---|---|---|
| 复用 core | 直接 import internal/core，原子修改 | 需要公开 core API、独立 core 模块或走 CLI/JSON |
| 版本一致性 | 一个 vX tag 构建 neca + neca-web，天然同版本 | 两套 tag 与兼容矩阵，容易漂移 |
| Docker 构建 | 同一 Dockerfile/base，lite/web 层共享 | Web 需 FROM lite 或复制 base，跨仓库发布 |
| CI/发布 | 一套 CI、一个 release、一个 SBOM 流程 | 两套 CI、跨仓库触发、权限与密钥管理 |
| 安全边界 | 镜像/容器/权限隔离 | 仓库隔离不是运行时隔离 |
| AGPL | 一份 LICENSE/NOTICE/源码 | 两个仓库都要维护许可和源码义务 |
| 开发效率 | Web/CLI 改动可同一 PR 验证 | 跨仓库改动需要协调 PR/版本 |
| 发布节奏 | CLI 与 Web 绑定发布 | Web 可独立迭代，但需兼容旧 lite |
| Issue/文档 | 一个入口，问题可一起排查 | 用户容易报错仓库；文档分裂 |
| 贡献者边界 | 当前单维护者足够 | 多团队/多产品时更有意义 |

### 12.3 为什么不建议现在拆

1. **Go internal 包不能跨仓库 import**
   internal/core 在本模块外不可引用；拆分需要先把 core 提取成公开模块，或让 Web 通过 CLI/JSON 调用。前者是 API 重构，后者损失类型安全与性能。

2. **lite/neca-web 必须共享精确版本**
   lite→full 要求两个镜像同 tag；同一仓库 tag 同时构建两个镜像最简单。

3. **Web 仍然很小**
   M6 只读、M6b helper 写操作；目录分离（cmd/necad、internal/web）已足够，不需要仓库级隔离。

4. **独立仓库不会减少攻击面**
   lite 镜像不包含 Web 二进制、Web 容器非 root、helper 单独容器，这些是镜像/部署问题，不是仓库问题。

5. **当前 PR 政策**
   开发期暂不接受外部 PR；独立仓库带来的协作收益暂时不成立。

### 12.4 以后可以考虑拆分的情形

满足以下一项或多项时再评估：

- Web 需要独立团队、独立发布节奏或独立 SLA；
- Web 改用不同语言/框架，无法继续复用 Go core；
- core 已稳定并提取为公开 Go module（例如 neca-core）；
- Web 需要单独的公开 API/插件生态；
- 外部 PR 开放后，Web 贡献者数量显著超过 CLI；
- 安全审计要求把 Web 供应链从 CLI 镜像中完全分离。

### 12.5 如果将来拆分

不建议复制 internal/core。可选路径：

1. 提取 neca-core 公开模块，Web 仓库依赖语义化版本；
2. Web 只通过稳定的 CLI/JSON 契约调用，不直接依赖 core；
3. 独立仓库的 Web 镜像 FROM lite base，保持基础层与版本标签一致；
4. 建立兼容矩阵和跨仓库集成测试；
5. 两侧都保留 AGPL、NOTICE、来源说明。

### 12.6 对当前发布方案的影响

保持 monorepo 后：

- 同一 tag 构建 neca:lite 与 neca-web:addon；
- lite 镜像与 neca-web 共享 base 层；
- CI 同时测试 CLI 与 Web；
- 文档继续放在 docs/；
- 不需要 submodule/replace/跨仓库 token。

结论：**当前不拆分；等 Web 成为独立产品或 core 公开 API 稳定后，再按 12.4 的条件评估。**

## 13. 决策状态

## 13. 决策状态

截至 2026-11-15：

1. ✅ Q1：M6a 只读 + M6b 写能力分期（已确认）。
2. ✅ Q2：root helper 作为唯一写边界（已确认）。
3. ✅ Q3：单管理员优先 OpenSSH 本地转发；浏览器场景系统原生 IKEv2 + mTLS；不使用第三方 VPN 客户端（已确认）。
4. ✅ Q4：Web 用户不拥有 web-secrets 目录；采用 root 属主 + 组权限 + 按文件挂载（2026-11-15 已确认）。
5. ✅ Q5：Go 模板 + 原生 JS；深浅色、蓝白/蓝黑；SQLite 延后（已确认）。
6. ✅ Q6：neca（lite）+ neca-web（可选面板包）两个镜像标签。
7. ✅ Q7：Web 包使用独立 Docker 镜像。
8. ✅ Q8：helper 使用独立 web-write profile/容器。
9. ✅ Q9：保持 monorepo。
10. ✅ Q10：空闲超时默认 30m、最大 24h、绝对会话上限 24h。
11. ✅ Q11：Passkey 首版作为第二因素，仅在 HTTPS + 稳定域名/RP ID 可用时启用；仅保留接口。

**Q1–Q11 均已确认；无剩余设计问题。实施规格见 [web-panel-development.md](web-panel-development.md)。**

M6a 只读 + 会话超时 + Passkey 接口占位 + lite/neca-web 发布约 5 人日；M6b helper 写能力再加约 3 人日。
