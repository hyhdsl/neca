# neca Web 面板开发文档（M6）

> 状态：**M6a 已实现（代码/测试/镜像验证），待发布；M6b 未开始**。日期：2026-11-15。
> 设计依据：[web-panel.md](web-panel.md)。本文把已确认的 Q1–Q11 落成可执行的开发任务。
> 目标：M6a 只读面板 + M6b helper 写能力；默认 lite，Web 作为可选 neca-web 面板包。

## 1. 已确认决策

| # | 决策 |
|---|---|
| Q1 | M6a 只读；M6b 增加 helper 写能力 |
| Q2 | root helper 是唯一写边界 |
| Q3 | 不使用第三方 VPN 客户端；单管理员优先 OpenSSH 本地转发；浏览器场景系统原生 IKEv2 + mTLS |
| Q4 | Web 用户不是 web-secrets 属主；root 属主、组权限、按文件挂载 |
| Q5 | Go 模板 + 原生 JS；深浅色；SQLite 延后 |
| Q6 | neca（lite）+ neca-web（可选面板包）两个镜像 |
| Q7 | Web 包使用独立 Docker 镜像 |
| Q8 | helper 使用独立 web-write profile/容器 |
| Q9 | 保持 monorepo |
| Q10 | 空闲超时默认 30m，最大 24h；绝对会话上限 24h |
| Q11 | Passkey 首版作为第二因素；仅保留接口，默认关闭 |

## 2. 范围

### 2.1 M6a（只读）

- 登录、会话、空闲超时、CSRF 基础；
- 证书列表/搜索/详情；
- p12 verify 结果；
- 下载列表与受保护下载；
- Web 配置管理命令；
- lite/neca-web 双镜像与 compose profiles；
- Passkey 接口占位；
- 不包含 issue/revoke/reissue/delete 和设置写回。

### 2.2 M6b（写能力）

- root helper（Unix socket + 白名单）；
- issue/revoke/unrevoke/reissue；
- 写操作审计、CSRF、幂等与错误映射；
- delete 暂不开放，后续单独评审；
- 所有写操作默认受 web.readOnly 和 helper 可用性双重限制。

### 2.3 明确不做

- Web 端删除证书、修改 PKI 路径；
- 多用户/RBAC；
- 第三方 VPN 客户端（Tailscale/WireGuard 等）；
- 运行时把 Web 二进制复制进 lite 容器；
- SQLite（M6 延后）。

## 3. 组件与目录

~~~text
cmd/
  neca/                 lite CLI
  necad/                Web 服务端（仅 neca-web 镜像）
  neca-web-helper/      root helper（仅 neca-web 镜像）
internal/
  core/                 现有业务 core，不复制
  web/
    server.go           HTTP server、路由、中间件
    auth.go             登录、密码校验、mTLS 信任
    session.go          签名 Cookie、空闲/绝对超时
    passkey.go          PasskeyStore/PasskeyAuthenticator 接口（无实现）
    api.go              只读 JSON API
    downloads.go        下载授权、TTL、路径安全
    views/              Go 模板
    static/             CSS、极少量 JS
  cli/
    commands_web.go     web status/readonly/idle-timeout
deploy/
  docker-compose.neca.yml
  neca.env.example
docs/
  web-panel.md
  web-panel-development.md
~~~

lite 镜像只包含 cmd/neca；neca-web 镜像包含 cmd/necad 与 cmd/neca-web-helper，不包含 lite CLI。

## 4. 配置

在 neca.json 增加非敏感 web 段：

~~~json
{
  "web": {
    "enabled": true,
    "listen": "127.0.0.1:18080",
    "readOnly": true,
    "idleTimeout": "30m",
    "adminCNs": ["webadmin-01"],
    "passkey": {
      "enabled": false,
      "rpId": "vpn.example.com",
      "rpName": "neca Admin"
    }
  }
}
~~~

约束：

- idleTimeout 允许 5m–24h，默认 30m；
- 绝对会话上限固定 24h，不提供无限会话；
- readOnly 默认 true；
- passkey.enabled 默认 false；M6a 不注册 Passkey 路由；
- 密码哈希、会话密钥、mTLS 私钥、数据库密码不写入 JSON；
- 配置写入使用现有原子写 + flock；新文件 0600，覆盖已有文件时保留 mode/属主/组；
- lite 镜像忽略 web 配置，不因 web 字段报错。

## 5. CLI

lite 提供管理命令，不包含 Web 服务端：

~~~bash
neca web status
neca web readonly status
neca web readonly on
neca web readonly off
neca web idle-timeout status
neca web idle-timeout 30m
~~~

行为：

- 所有写配置命令只允许 root；
- readonly off 需要 Web 组件 + helper 控制 socket；lite 默认没有，返回“Web 组件未安装”；
- full 部署把 helper 控制 socket 只读挂载给 lite CLI，用于检查 helper 可用性；
- 修改通过 SIGHUP 或配置监视生效；不能热加载时提示重启；
- 切换动作写审计，记录旧值/新值；
- 不提供 neca web serve；Web 服务由 neca-web 镜像的 necad 启动。

## 6. 权限与 web-secrets

### 6.1 目录

~~~text
/opt/ipsec/web-secrets/          root:neca-web  0710
  session.key                    root:neca-web  0640
  passkeys.json                  root:neca-web  0660（后续）
  web.db                         root:neca-web  0660（后续）
  mtls.key                       root:proxy     0600（代理使用）
  helper.sock                    root:neca-web  0660
~~~

- Web 用户不是目录属主；
- Docker 采用单文件 bind mount，不整体挂载 web-secrets；
- mTLS 私钥只给反向代理，Web 不可见；
- helper 私钥/配置只给 helper；
- 会话签名密钥可读是必要的；Web 被攻破仍可能伪造会话，这是单用户方案的固有限制；
- 通过短 idle（默认 30m）、24h 绝对上限、密钥轮换和审计降低风险。

### 6.2 容器用户

- necad 固定非 root UID/GID，例如 10001:10001；
- Web 容器 no-new-privileges，只读根文件系统（需要时仅挂载 tmpfs）；
- helper 独立 root 容器，只挂载 CA 私钥、docker.sock、helper socket；
- 禁止 Web 容器访问 docker.sock。

## 7. 认证与会话

### 7.1 登录链

~~~text
系统原生 IKEv2 + 反向代理 TLS（可选 mTLS）
  → 来源网段检查
  → 管理员密码哈希
  → 签名会话 Cookie
  → 空闲/绝对超时
  → CSRF（写操作）
  → 限流与审计
~~~

- 密码哈希：Argon2id 或 bcrypt；通过 NECA_WEB_PASSWORD_HASH 注入；
- 会话：HMAC-SHA256 签名 Cookie，包含 iat、lastSeen、exp、jti；
- Cookie：HttpOnly、Secure、SameSite=Lax；
- 每次认证请求更新 lastSeen；静态资源不续期；
- 空闲超过 idleTimeout 或自登录超过 24h，返回 401/跳转登录；
- 登出将 jti 放入内存撤销表直到过期；重启后会话自然失效；
- 会话密钥轮换后所有会话失效；
- CSRF 使用 double-submit token；写操作强制校验；
- 登录失败限流并审计。

### 7.2 mTLS

- 反向代理终止 TLS/客户端证书；
- 只信任回环代理，校验共享代理密钥或 Unix socket；
- 清除客户端伪造的 X-Client-Cert/X-Forwarded-* 头；
- adminCNs 白名单；
- 管理员证书由独立 CN 签发，不复用普通 VPN 用户证书；
- 代理不可用时 fail closed。

### 7.3 Passkey

M6a 只定义接口和 feature flag：

~~~go
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

- 不引入 WebAuthn 依赖；
- 默认不注册路由；
- 未来仅在 HTTPS + 稳定域名/RP ID 可用时启用；
- 首版作为密码之外的第二因素；
- 凭据文件后续可迁移到 SQLite。

## 8. HTTP 与 API

只读 API：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /healthz | 存活检查，仅回环 |
| GET | /login | 登录页 |
| POST | /login | 登录 |
| POST | /logout | 登出 |
| GET | /api/v1/certs | 列表/搜索 |
| GET | /api/v1/certs/{cn} | 详情 |
| GET | /api/v1/certs/{cn}/verify | p12 verify |
| GET | /api/v1/downloads | 下载列表 |
| GET | /api/v1/downloads/{id} | 下载 |
| GET | /api/v1/capabilities | readOnly/功能开关 |

- 写 API 只在 M6b 增加；
- 下载 ID 由 sidecar 元数据解析，禁止任意路径；
- 过期返回 410/404；
- 所有响应设置安全头：CSP、X-Content-Type-Options、Referrer-Policy、Cache-Control；
- 下载响应使用 Content-Disposition，文件名不含路径。

## 9. UI

- Go html/template + embed；
- 页面：登录、概览、证书列表、证书详情、下载、错误/空状态；
- 蓝色主色 + 白/浅灰（浅色）与蓝 + 近黑（深色）；
- CSS 变量 + prefers-color-scheme + 手动切换（localStorage）；
- 响应式：桌面侧栏 + 内容，移动端单列；
- 不使用外部 CDN；CSP 默认 self；
- 关键控件 data-testid + ARIA；
- 不以截图作为像素/颜色验收依据；Playwright DOM/计算样式/axe 为准。

## 10. 读只读模式

- 中间件检查 readOnly 与请求方法；
- POST/PUT/PATCH/DELETE 在 M6a 一律 403；
- capabilities 返回 readOnly 与可用能力；
- UI 隐藏写控件，但不作为安全边界；
- helper 自身也检查 readOnly；
- readonly off 失败不产生半状态。

## 11. Helper（M6b）

- Unix socket /run/neca/web-helper.sock，0660，组 neca-web；
- 只允许 issue/revoke/unrevoke/reissue；
- 请求结构：op、CN、days、platforms、p12Profile、requestId；
- 校验 CN 正则、天数范围、平台枚举；
- 不接受任意路径、shell 或额外参数；
- 串行执行，复用 PKI flock；
- 结果结构化 JSON；不返回私钥/p12 密码；
- helper 检查 readOnly、请求 ID 幂等、审计；
- delete 不开放；
- Web 只发送白名单请求，无法直接读 CA 私钥。

## 12. Docker 发布

- lite 镜像：neca:vX，默认；只含 CLI 与 web 管理命令；
- neca-web 镜像：neca-web:vX，可选；含 necad/helper，不包含 lite CLI；
- 两镜像共享 base 层；version 标签一致；
- compose profiles：
  - 默认：lite；
  - --profile web：只读 Web；
  - --profile web-write：helper；
- lite + web 是 full 部署；不发布 full 镜像；
- helper 控制 socket 只读挂载到 lite CLI；
- lite→full 只拉 neca-web；full→lite 停用/移除 Web 组件；
- web-secrets 按第 6 节挂载；
- Web/helper 默认不启动；
- Dockerfile target lite/web；CI 构建两镜像。

## 13. 代码任务

### M6a

1. config：WebConfig、idleTimeout 校验、env/JSON 优先级；
2. CLI：web status/readonly/idle-timeout；
3. internal/web server：路由、中间件、安全头；
4. session/auth：密码哈希、Cookie、idle、CSRF、限流；
5. 只读 API + 模板页面；
6. 下载授权与 TTL；
7. lite/neca-web 镜像与 compose profiles；
8. Passkey 接口与 feature flag（无路由）；
9. 单元、httptest、Playwright、Docker 测试。

### M6b

1. helper 协议与 Unix socket；
2. helper 白名单、审计、幂等；
3. 写 API + CSRF + readOnly 双检查；
4. Web 写页面（确认、错误、审计）；
5. helper 与 Web 集成/回滚测试。

## 14. 测试与验收

- 只读模式写请求 403；
- 未登录不能访问证书/下载；
- 空闲 30m 失效、24h 绝对失效、配置上限生效；
- 路径遍历、伪造 mTLS 头、CSRF、暴力登录测试通过；
- 会话密钥权限 0640、目录 0710、Web 非 root；
- lite 镜像不含 necad/helper；neca-web 默认不暴露端口；
- lite→neca-web→lite 切换不迁移 PKI、不丢证书；
- 版本不匹配快速失败；
- Passkey 关闭时无路由/依赖；
- go test -race、httptest、Playwright、axe、容器灰度通过。

## 15. 发布

- 版本建议 v0.3.0（M6a）；
- 同 tag 构建 neca 与 neca-web；
- amd64/arm64、SBOM、checksums；
- release notes 明确默认 lite、Web 可选；
- 保留上一版本镜像与 compose 回滚；
- 文档更新 README、docs 索引、部署说明。

## 16. 实施前置条件

这些不影响代码开工，但部署/验收前需要确定：

- Web 管理域名与 TLS 证书（mTLS/Passkey 需要稳定域名和 RP ID）；
- 管理员证书 CN 与签发流程；
- 反向代理选型（Caddy 或 Nginx）；
- 服务器原生 IKEv2/OpenSSH 的访问路径；
- web-secrets 的固定 UID/GID 与挂载路径。

## 17. 与评估文档的一致性

- 默认 lite，Web 可选；不发布 full 镜像；
- 非 root Web + root helper；
- 不使用第三方 VPN；
- Web 用户不拥有 web-secrets；
- 空闲超时、Passkey 接口、Go 模板、SQLite 延后；
- 保持 monorepo。

未发现与已确认决策冲突的项。M6a 已按本文实施，M6b 待开始。

## 18. M6a 实施记录（2026-11-15）

已落地：

- core：`WebConfig`、idleTimeout 5m–24h 校验、只读配置加载 `LoadConfigReadOnly`、原子 `UpdateWebConfig/UpdateWebReadOnly/UpdateWebIdleTimeout`；
- CLI：`neca web status|readonly|idle-timeout|hash-password`；`readonly off` 在 M6a 明确拒绝；
- Web：`cmd/necad` + `internal/web`，bcrypt 登录、HMAC 签名 Cookie、空闲/绝对超时、CSRF、登录限流、代理共享密钥 + 管理员 CN 校验、只读中间件、证书/详情/verify/下载 API 与 Go 模板页面；
- Web 只从 `clients/<CN>/<CN>.crt` 读取证书副本，不挂载 `pki` 根目录、`ca.key`、客户端私钥或 docker.sock；
- Docker：`Dockerfile` 拆为 `base`/`web`/`lite`，`make build` 同时产出 necad/helper；`deploy/docker-compose.neca.yml` 增加 `web` profile；
- 部署：`deploy/init-web-secrets.sh` 初始化 web-secrets；M6a 不启动 helper；
- 验证：`go vet ./...`、`go test -race ./...`、lite/neca-web 镜像内容检查、necad 容器 `/healthz` 健康检查通过。

尚未完成：

- Playwright DOM/计算样式/axe 浏览器验收；
- 生产服务器灰度与反向代理/mTLS 实机验证；
- SIGHUP 热重载（当前修改配置后需重启 necad）；
- M6b helper、写 API 与写页面；
- Passkey 实际注册/登录（仅接口与配置开关）。

部署权限要点：

- `/opt/ipsec/neca.json` 在启用 Web 时为 `root:neca-web 0640`；
- `clients/` 与 `downloads/` 需要对 `neca-web` 组可读（`g+rX`）；目录组为 `neca-web` 时，CLI 新生成的 p12/zip/sidecar 会自动设为 0640；
- `web-secrets/` 为 `root:neca-web 0710`，session key/password hash 0640，audit log 0660；
- Web 容器使用固定 UID/GID 10001、只读根文件系统、`no-new-privileges`、无 docker.sock。

未发现与 Q1–Q11 的冲突。
