# neca 开发指南（当前）

> 更新：2026-11-16 · 基线：v0.4.1（tag 6060c2b）  
> 当前权威里程碑：[MILESTONES.md](MILESTONES.md)。历史设计/验收记录：[neca-development.md](neca-development.md)、[web-panel.md](web-panel.md)、[web-panel-development.md](web-panel-development.md)。  
> 中文文档为主版本；英文 README/帮助可能滞后。

## 1. 项目定位

**Neko's Efficient Certificate Administrator（neca，部分场合写作 NECA）** 是面向 strongSwan IKEv2 的轻量证书管理工具：CLI 是主入口，Web 默认只读，写操作经 root helper 白名单。

## 2. 当前目标

- v0.4.x 保持可运行、默认只读；在大重构完成前不继续堆叠新功能。
- 重构范围是**命令/控制面**，不是 PKI 数据格式或 strongSwan 配置格式。
- 重构后统一命令标准，明确 backend CLI、Web、helper 三种来源，旧 legacy 命令进入迁移/废弃路径。
- 破坏性操作必须有后台确认、审计与恢复路径。

## 3. 代码结构

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
| <code>legacy/</code> | 旧 bash 脚本只读归档；不作为兼容层继续扩展 |

## 4. 命令/控制面重构（M9）

### 4.1 三种来源

| 来源 | 允许做什么 | 审计 origin |
|---|---|---|
| backend CLI | SSH/root 管理：配置、紧急停止、回收站物理清理、备份 | <code>cli</code> |
| Web | 页面/API 的只读或受限写；只能经 helper 白名单 | <code>web</code> |
| helper | 结构化 root 操作；不接受任意 shell/路径 | <code>helper</code> |

Web 不执行任意 CLI；后台命令与 Web 动作使用同一业务契约，但入口、权限、确认级别不同。

### 4.2 命令元数据

每个命令至少声明：

- 名称/别名/分组；
- 来源：<code>backend</code> / <code>web</code> / <code>shared</code>；
- 是否写操作、是否需要确认；
- 是否支持 <code>--json</code> / <code>--dry-run</code>；
- 审计事件与错误码；
- 废弃状态与替代命令。

### 4.3 新命令标准（建议）

- <code>neca cert issue|list|verify|reissue|revoke|unrevoke|delete|export|platform</code>
- <code>neca pki ...</code>、<code>neca config ...</code>、<code>neca web ...</code>
- <code>neca trash list|restore|purge</code>
- 紧急控制：<code>neca web lock</code>、<code>neca web lockdown</code>
- 导出开关：<code>neca web export on/off</code>

### 4.4 legacy 迁移

<code>legacy/vpn-cert-manager.sh</code> 保留为只读归档，不再新增功能。旧脚本参数到新命令的迁移表：

| 旧脚本 | 新命令 |
|---|---|
| <code>--issue CN</code> | <code>neca cert issue CN</code> |
| <code>--revoke CN</code> | <code>neca cert revoke CN</code> |
| <code>--unrevoke CN</code> | <code>neca cert unrevoke CN</code> |
| <code>--reissue CN</code> | <code>neca cert reissue CN</code> |
| <code>--add-platform</code> / <code>--set-platforms</code> | <code>neca cert platform add/set</code> |
| <code>--delete CN --yes</code> | <code>neca cert delete CN</code>（后续改为移入回收站，后台 purge） |
| <code>--export-issued</code> / <code>--export-revoked</code> | <code>neca cert export issued|revoked</code> |
| <code>--list</code> | <code>neca cert list</code> |
| <code>--sync</code> | <code>neca pki index sync</code> |
| <code>--verify</code> | <code>neca cert verify</code> |

旧命令是否保留一个发布周期的警告期，或 v0.5.0 直接移除，需用户决策（见 MILESTONES）。

### 4.5 扩展/移除

CLI 已有注册表（<code>internal/cli/command.go</code>），当前新增命令成本低；Web 路由集中在 <code>Server.Handler</code>，页面/API handler 集中在 <code>pages.go</code>/<code>api.go</code>，缺少 feature flag 时移除功能较费力。

重构方向：

1. 先抽命令定义/来源元数据，不改变业务行为；
2. 再按功能拆 Web 路由组和 helper 操作；
3. 加开关与回退路径；
4. 最后移除旧命令/别名，保持测试与影子对比。

不做一次性重写 core/PKI。

## 5. 安全不变量

- Web 默认 <code>readOnly=true</code>；Web 不得接触 <code>ca.key</code> 或 <code>docker.sock</code>。
- 写操作只经 helper 白名单；Web 端仍需双重检查。
- 低风险偏好（语言/显示）可 Web 修改；高风险设置默认后台-only，Web 只读展示。
- 高风险设置是否允许 Web 修改，由后台开关决定；开启时需重新认证、CSRF、审计与确认。
- 删除先移入回收站；物理 purge 只能后台二次确认。
- 紧急命令只允许后台 root；<code>lock</code> 保留只读服务，<code>lockdown</code> 停止 Web+helper。
- 生产默认 OpenSSH 本地转发；公网访问需要单独的 TLS/mTLS 或 Tunnel/Access 设计。

## 6. 配置与部署

- 业务配置：<code>/opt/ipsec/config/neca.json</code>；Web 挂载配置目录以感知 SIGHUP。
- 只读/空闲超时可通过 SIGHUP 重载；listen/adminCNs/passkey 仍需重启。
- helper 幂等缓存：<code>/run/neca/web-helper-cache.json</code>（0600，24h TTL）。
- 部署、权限、备份与回滚：[deploy.md](deploy.md)。

## 7. 构建与测试

~~~bash
docker compose run -T --rm dev make all
docker compose run -T --rm dev make test
docker compose run -T --rm dev make shadow
docker compose run -T --rm dev make check-docs
~~~

重构期间额外要求：

- 命令迁移表逐条测试；
- 旧命令/新命令同时存在时，确保不会双写；
- Web/helper 入口分别做权限和审计测试；
- 破坏性功能必须有故障注入与恢复演练。

## 8. 文档地图

- 里程碑与决策：[MILESTONES.md](MILESTONES.md)
- 当前开发指南：本文件
- 历史架构/CLI/验收：[neca-development.md](neca-development.md)
- Web 设计历史：[web-panel.md](web-panel.md)、[web-panel-development.md](web-panel-development.md)
- 部署与回滚：[deploy.md](deploy.md)
- 用户帮助：[help/README.md](help/README.md)
- AI 披露：[AI-DISCLOSURE.md](../AI-DISCLOSURE.md)

## 9. 当前路线

M9 命令/边界重构（v0.5.0 breaking）→ M10 安全运维（lock/lockdown、导出开关、备份恢复）→ M11 回收站/设置 → M12 可维护性与可观测性 → M13 公网访问 → M14 OpenAPI 长期搁置。详见 [MILESTONES.md](MILESTONES.md)。
