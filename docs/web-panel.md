# neca Web 面板设计评估（M6）

> 状态：**评估稿，未实现**。日期：2026-11-15。
> 本文回答 Web 面板的访问控制、只读开关、证书访问、鉴权、UI 与后续 SQLite 迁移问题。实现以本文确认后的版本为准。

## 1. 结论摘要

| 问题 | 结论 |
|---|---|
| Q1 后台只读开关 | **可以**。在 neca.json 增加 web.readOnly，并新增 neca web readonly status/on/off 子命令；服务端强制拦截写请求。只读关闭后需要独立 root helper，Web 进程本身始终非 root。 |
| Q2 权限调整 | Web 进程只读挂载、无 ca.key、无 docker.sock；写操作交给 Unix socket 的 root helper。 |
| Q3 仅 VPN/指定证书访问 | 仅“连上 VPN”不足以限制到某一张证书。推荐 SSH 隧道/Tailscale 直连回环；如必须浏览器访问，使用反向代理 mTLS + 专用管理员证书 + VPN 网段防火墙。 |
| Q4 鉴权调整 | mTLS（可选）+ 管理员密码哈希 + 签名 Cookie + CSRF + 限流；只读开关只允许 root CLI 修改，不能从 Web 自己解锁。 |
| Q5 UI/SQLite | Go 模板 + CSS 变量，深浅色蓝白/蓝黑主题；不以截图作为像素级验收依据，使用 data-testid/DOM/Playwright。M6 不引入 SQLite，但预留 Store 接口与迁移路径。 |

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
    "sessionTtl": "8h",
    "adminCn": "webadmin-01"
  }
}
~~~

密码哈希、会话密钥、mTLS 私钥/token 不写入 neca.json。

### 3.3 新增 CLI 子命令

建议命令：

~~~bash
neca web serve                 # 启动 Web 服务
neca web status                # 运行状态与当前模式
neca web readonly status       # readonly | readwrite
neca web readonly on           # 强制只读
neca web readonly off          # 需要 helper 已启用
~~~

行为：

- on/off 由 root CLI 执行，原子写 neca.json，文件保持 0600；
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
| 反向代理 mTLS + 专用管理员证书 + CN 白名单 | 高 | 中 | 满足“只有该证书能访问 Web”；推荐浏览器场景 |
| SSH 隧道 / Tailscale / WireGuard 管理网 | 很高 | 中高 | 单管理员最简、暴露面最小；优先推荐 |
| 用同一张 VPN 客户端证书兼做浏览器 mTLS | 中高 | 低 | 技术上可行，但浏览器导入、移动端、私钥暴露和角色混淆问题多，不推荐 |

### 5.2 推荐

**单管理员、日常运维：**

- Web 绑定 127.0.0.1:18080；
- 通过 SSH 本地端口转发访问：ssh -L 18080:127.0.0.1:18080 ...；
- 或使用 Tailscale/WireGuard 管理网；
- 不向公网或 VPN 业务网段开放 Web 端口。

**必须从浏览器直接访问：**

1. 反向代理只绑定 VPN 接口，防火墙只允许 VPN 网段；
2. 反向代理启用 mTLS，校验证书链和 CN；
3. 单独签发管理员客户端证书（不要复用普通 VPN 用户证书），CN 写入白名单；
4. 代理把已验证的 CN 通过受信任方式传给 necad（或由 necad 自己验证客户端证书）；
5. 再叠加管理员登录密码，避免“证书即全部权限”；
6. 管理员证书吊销后，代理立即拒绝；证书私钥只存在管理员设备，不进服务器仓库。

### 5.3 能不能“签一张证书，只让这张证书连 VPN 才能访问 Web”？

- “只让这张证书连 VPN”本身需要 VPN 侧的用户/证书授权与地址分配；当前 strongSwan 配置不能仅凭 Web 侧实现。
- “只让这张证书访问 Web”更适合用 **mTLS CN 白名单** 实现，而不是依赖 VPN 连接状态。
- 最佳组合：**VPN 网段限制 + 反向代理 mTLS 专用管理员证书 + 登录密码**。
- 单管理员更推荐 **SSH 隧道/Tailscale**，比浏览器证书更容易维护。

## 6. Q4：鉴权与会话调整

### 6.1 认证链

~~~text
TLS/mTLS（反向代理，可选）
  → 来源 IP/网段检查
  → 管理员登录（密码哈希）
  → 签名会话 Cookie（HttpOnly + Secure + SameSite=Lax）
  → CSRF token（写操作强制）
  → 限流 / 登录失败审计
~~~

- 密码只以 Argon2id 或 bcrypt 哈希形式通过 NECA_WEB_PASSWORD_HASH 注入；
- 会话密钥放 0600 文件，支持轮换；不使用默认值；
- 代理信任：necad 只接受来自回环的连接；若读取 X-Forwarded-* 或客户端证书头，必须验证代理共享密钥并清除客户端伪造头；
- 登录失败限流，避免暴力破解；
- 只读/读写切换只能由 root CLI 完成，Web 不提供“关闭只读”的 API；
- 下载必须登录，下载 ID 不透明，TTL 30 分钟，过期 410/404。

### 6.2 审计

记录：

- 登录成功/失败、来源 IP、证书 CN（如启用 mTLS）；
- readonly on/off；
- 下载创建/访问/过期；
- 写操作（helper）：操作者、CN、序列号、结果；
- 绝不记录密码、私钥、p12 密码、会话密钥。

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
- 深浅色主题对比度通过自动化检查；
- 移动端与桌面端关键页面通过 Playwright 截图与 DOM 断言；
- go test -race、httptest、容器灰度验收通过。

## 11. 待确认

1. Q1：接受只读开关 + helper 写能力分期（M6a/M6b），还是要求 M6 一次完成写操作？
2. Q2：接受 root helper 作为唯一写边界？
3. Q3：访问方式选 SSH 隧道/Tailscale，还是反向代理 mTLS？若选 mTLS，管理员证书的 CN 与签发流程需定。
4. Q4：密码哈希注入与会话密钥文件的具体路径/权限。
5. Q5：先做只读页面与深浅色，SQLite 保持延后；确认是否接受 Go 模板而非 SPA。
