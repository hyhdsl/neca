# neca M9 命令/边界重构计划

> 状态：M9.1–M9.4 已完成；下一步 v0.5.0 发布与迁移验证。M9.4 已迁移命名空间、移除旧命令并落地最小回收站。权威里程碑见 [MILESTONES.md](MILESTONES.md)，盘点表见 [M9.1-COMMAND-INVENTORY.md](M9.1-COMMAND-INVENTORY.md)，当前开发指南见 [DEVELOPMENT.md](DEVELOPMENT.md)。
> 范围：命令/控制面与扩展接口；**不重写** PKI 数据格式、strongSwan 配置格式或核心证书算法。

## 1. 目标

1. 统一命令命名空间和新命令标准。
2. 区分 **backend CLI / Web / helper** 三种来源与权限。
3. 为确认、审计、开关、回收站、导出、设置提供可复用接口。
4. 让新增功能只需增加模块/路由/命令与测试；让移除功能能干净关闭。
5. v0.5.0 breaking：旧命令直接移除，不设过渡期；提供迁移表和回滚。

## 2. 保留

- <code>internal/core</code> 的 PKI/证书/CRL 行为和文件格式。
- <code>internal/core/ports</code> 的 IndexStore / 配置来源接口（可扩展字段，不破坏现有实现）。
- helper Unix socket、root 白名单、requestId 缓存、配置目录挂载与 SIGHUP。
- Web 默认只读、非 root、无 <code>ca.key</code>/<code>docker.sock</code>。
- 中文主文档、AGPL、AI 披露和默认 OpenSSH 访问方式。

## 3. 删除/废弃

- <code>legacy/vpn-cert-manager.sh</code> 的旧参数习惯：不再作为兼容层；脚本只读归档。
- 顶层旧命令/别名（<code>issue</code>、<code>revoke</code>、<code>delete</code>、<code>export</code> 等）：v0.5.0 直接移除。
- 直接物理删除：改为 <code>neca cert trash</code> 移入回收站，物理清理仅 <code>neca trash purge</code>。
- 旧 <code>--yes</code> 语义：统一为确认策略的一部分；不保留旧命令的隐式兼容。
- Web 中分散的写路由/通用只读 guard 的重复逻辑：收敛到统一路由组和策略。
- 文档中的历史里程碑/重复状态：保留历史证据，但不再作为当前权威。

## 4. 修改

### CLI

- <code>internal/cli/command.go</code> 的 Command 增加元数据：
  - <code>Group</code>、<code>Surface</code>（backend/web/shared）、<code>Write</code>、<code>Confirmation</code>、<code>AuditEvent</code>、<code>Deprecated</code>。
- 注册表支持按 surface 查询/生成帮助；不把 Web-only 命令暴露给 CLI。
- 输出错误码与 JSON 保持稳定；<code>--yes</code> 仅作为显式跳过 Y/N 的信号。

### Web

- <code>Server.Handler</code> 拆为路由组：certs、downloads、web-control、settings、trash、export、auth。
- 每个可移除功能由 feature flag 控制，关闭时：
  - 不注册路由或返回 404/403；
  - 导航隐藏；
  - API 不返回可用状态。
- 高风险设置修改走 helper 白名单；低风险偏好保留在 Web/Cookie。

### helper

- 操作从字符串 switch 迁移到注册表：每个操作声明输入、权限、确认、审计、幂等能力。
- 新增回收站、导出读取、紧急控制/设置（如果跨边界）所需操作时，必须显式注册，不接受任意命令。

### 配置

新增字段（均向后兼容，旧配置缺省安全值）：

- <code>web.sessionEpoch</code>（lock）
- <code>web.allowHighRiskSettings</code>（默认 false）
- <code>web.exportEnabled</code>（默认 false）
- <code>trash.retention</code>（默认 0=不限时）
- 可选：<code>web.features</code> 开关集合

## 5. 新增接口（保留扩展点）

| 接口/类型 | 职责 | 为什么保留 |
|---|---|---|
| <code>CommandSpec</code> | 命令名、分组、surface、写操作、确认、审计 | 统一 CLI/文档/测试 |
| <code>Surface</code> | backend / web / helper | 防止 Web 越权调用后台命令 |
| <code>Confirmer</code> / <code>ConfirmationPolicy</code> | 独立 Y/N 函数/接口，默认 N、<code>--yes</code>、Web 确认 | 按功能增删确认；CLI 与 Web 各自实现 |
| <code>AuditOrigin</code> | cli/web/helper + 操作者 | 追责与回放 |
| <code>Feature</code> / <code>RouteGroup</code> | 可开关的 Web 功能 | 新功能可加可撤 |
| <code>HelperOperation</code> | 输入/权限/幂等/审计 | helper 白名单统一 |
| <code>SessionEpochStore</code> | 持久会话代际 | lock 不依赖内存撤销 |
| <code>ServiceController</code> | 只停止明确命名的服务 | lockdown 精确控制 |
| <code>TrashStore</code> | 移入/恢复/清理/保留期 | 回收站与后台确认 |
| <code>Exporter</code> | issued/revoked CSV | 导出开关与列定义 |
| <code>SettingsService</code> | 低风险偏好/高风险策略 | 设置页与后台一致 |

接口原则：

- 小接口、按消费方定义；不引入插件框架。
- 先做无行为变化的抽取，再加功能。
- 每个接口有最小单测与一个集成路径。
- 新增功能必须能通过 feature flag 或配置关闭。

## 6. 建议新增功能（按优先级）

1. **证书到期/状态概览**（只读，低风险）：Web 首页显示即将到期、已吊销、缺少平台包；不改变写路径。
2. **一键备份/恢复**（后台，高价值）：PKI/config/web-secrets/clients/meta/downloads；带校验和与恢复演练。
3. **审计日志导出/轮转**（后台/只读）：CSV/JSONL + 保留期；不含私钥/密码。
4. **CLI shell completion**（低风险）：bash/zsh/fish；命令元数据自动生成。
5. **配置 schema 版本与迁移**（低风险）：v0.5 前后兼容；为回滚提供明确路径。
6. **证书导入/链验证**（只读/受限写）：导入已有证书做 verify，不生成私钥；权限需另定。

不建议现在做：多租户/RBAC、默认公网 Web、Passkey 实际登录、OpenAPI 稳定契约。

## 7. 实施顺序

1. **M9.1 盘点与冻结**：旧命令/参数/路由/helper 操作清单；迁移表；不再新增旧风格功能。
2. **M9.2 元数据/确认/审计**：✅ 已完成 Command 元数据/Confirmer/--yes/AuditOrigin；Web 高风险 Y/N 留到 M11。
3. **M9.3 Web/helper 模块化**：✅ 路由组与 helper 操作注册表已完成；feature flag 留给 M9.4/M10。
4. **M9.4 命令迁移**：✅ <code>neca cert ...</code>、<code>neca pki ...</code>、<code>neca trash ...</code>；旧命令移除。
5. **M9.5 v0.5.0**：完整测试、迁移文档、回滚演练；发布。
6. **M10/M11**：在 M9 接口上实现 lock/lockdown、CSV 导出、回收站、设置。

## 8. 测试与回滚

- 每条迁移表命令至少一条新旧等价测试（旧命令仅在迁移测试中使用）。
- Web/helper/CLI 三来源分别做权限、审计、CSRF、Y/N 测试。
- 破坏性操作：故障注入 + 恢复演练 + 二次确认测试。
- 回滚：保留 v0.4.1 镜像 tar、升级前 config/PKI 备份、旧 compose；只停止明确命名的 Web/helper；验证索引/CRL 哈希后恢复流量。
- 不承诺 SSD/快照/备份上的安全擦除。

## 9. 已确认

- <code>--yes</code>/<code>-y</code> 是后台自动化跳过 Y/N 的唯一入口；Web 使用 UI Y/N。
- CLI 确认逻辑必须独立为 <code>Confirmer</code> 接口/函数，默认 N；无需确认的功能不注入空壳。
- CSV 使用 <code>issued.csv</code> / <code>revoked.csv</code>，字段 CN/状态/序列号/到期时间/平台。
- 新功能顺序：B 备份恢复 → A 到期概览 → E config schema → C 审计导出 → F 证书导入 → D CLI completion。
- 下一步：v0.5.0 发布与迁移验证；M9.4 已完成。
