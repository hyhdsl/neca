# neca 开发指南（当前）

> 更新：2026-11-16 · 基线：v0.4.1
> 这是当前开发入口。里程碑看 [MILESTONES.md](MILESTONES.md)；历史设计/验收记录看 [neca-development.md](neca-development.md)；Web 设计历史看 [web-panel.md](web-panel.md) 与 [web-panel-development.md](web-panel-development.md)。

## 1. 项目定位

**Neko's Efficient Certificate Administrator（neca，部分场合写作 NECA）** 是面向 strongSwan IKEv2 的轻量证书管理工具：CLI 是主入口，Web 默认只读，写操作经 root helper 白名单。

## 2. 代码结构

| 目录 | 职责 |
|---|---|
| <code>cmd/neca</code> | CLI 薄入口 |
| <code>cmd/necad</code> | Web 服务入口 |
| <code>cmd/neca-web-helper</code> | root helper（Unix socket） |
| <code>internal/cli</code> | 参数解析、命令注册表、输出 |
| <code>internal/core</code> | 证书/PKI/配置/审计等业务核心 |
| <code>internal/web</code> | 模板、会话、HTTP handler、只读 API |
| <code>internal/helper</code> | helper 协议、白名单执行、幂等缓存 |
| <code>internal/i18n</code> | zh-CN/en-US CLI 文本 |
| <code>internal/web/locales</code> | Web 页面翻译 |
| <code>deploy/</code> | compose、初始化脚本、环境模板 |
| <code>docs/</code> | 当前指南、历史设计、部署与帮助源文件 |

## 3. 添加/移除功能

### CLI 子命令

CLI 已有注册表（<code>internal/cli/command.go</code>），内置命令在 <code>internal/cli/cli.go</code> 通过 <code>MustRegister</code> 挂载。新增命令通常只需：

1. 在对应 <code>commands_*.go</code> 实现构造函数和 <code>Run</code>；
2. 在 <code>cli.go</code> 注册；
3. 补命令级测试与帮助文本；
4. 若涉及写操作，确认 dry-run、JSON、审计和错误映射。

移除命令则删除注册和实现，并检查帮助、测试和文档引用。CLI 侧扩展成本低。

### Web 功能

当前 <code>Server.Handler</code> 集中注册路由，页面/API handler 主要集中在 <code>pages.go</code>/<code>api.go</code>。新增功能需要同时改：

- 路由与认证/CSRF/只读 guard；
- 模板与静态资源；
- helper 请求（如涉及写）；
- 审计与测试。

结论：CLI 可快速增删；Web 尚缺 feature flag/模块边界，移除功能比新增更费力。建议增量抽取“可选路由组 + 配置开关”，不做一次性大重构。紧急断开、导出开关、回收站都应先定义开关和回退行为。

## 4. 安全不变量

- Web 默认 <code>readOnly=true</code>；Web 不得接触 <code>ca.key</code> 或 <code>docker.sock</code>。
- 写操作只经 helper 白名单；Web 端仍需双重检查。
- 安全配置（readOnly、adminCNs、passkey、helper、导出开关）只允许后台 CLI 修改。
- 会话 Cookie 签名、HttpOnly、SameSite；会话密钥至少 32 字节。
- 删除/紧急停止必须有后台确认、审计与恢复路径；不要用 Web 直接物理删除。
- 生产默认 OpenSSH 本地转发；公网访问需要单独的 TLS/mTLS 或 Tunnel/Access 设计。

## 5. 配置与部署

- 业务配置：<code>/opt/ipsec/config/neca.json</code>；Web 挂载配置目录以感知 SIGHUP。
- 只读/空闲超时可通过 SIGHUP 重载；listen/adminCNs/passkey 仍需重启。
- helper 幂等缓存：<code>/run/neca/web-helper-cache.json</code>（0600，24h TTL）。
- 部署、权限、备份与回滚：[deploy.md](deploy.md)。

## 6. 构建与测试

~~~bash
docker compose run -T --rm dev make all
docker compose run -T --rm dev make test
docker compose run -T --rm dev make shadow
docker compose run -T --rm dev make check-docs
~~~

提交前至少确认受影响的 Go 包、文档检查；涉及删除/紧急停止时必须有故障注入和恢复测试。

## 7. 文档地图

- 里程碑与决策：[MILESTONES.md](MILESTONES.md)
- 当前开发指南：本文件
- 历史架构/CLI/验收：[neca-development.md](neca-development.md)（历史/深度参考）
- Web 设计历史：[web-panel.md](web-panel.md)、[web-panel-development.md](web-panel-development.md)
- 部署与回滚：[deploy.md](deploy.md)
- 用户帮助：[help/README.md](help/README.md)
- AI 披露：[AI-DISCLOSURE.md](../AI-DISCLOSURE.md)

## 8. 当前路线

按 [MILESTONES.md](MILESTONES.md)：M9 安全与可恢复性（导出开关、紧急断开、备份恢复）→ M10 受控 Web 管理（回收站、设置偏好）→ M11 可维护性 → M12 公网访问 → M13 API/OpenAPI（按需）。
