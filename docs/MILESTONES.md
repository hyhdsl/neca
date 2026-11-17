# neca 开发目标与里程碑

> 更新：2026-11-16 · 当前发布：v0.5.0（tag 0a3af74）；生产仍为 v0.4.1
> 当前 main：da11bc0；帮助站 neca-help：0ab67d3。
> 本文件是**里程碑、优先级与待决策事项**的唯一权威来源；当前开发入口见 [DEVELOPMENT.md](DEVELOPMENT.md)，历史实施记录见 [neca-development.md](neca-development.md)。

## 1. 当前状态

- M0–M8 已完成；v0.4.1 已发布并部署，Web 默认只读。
- CLI、只读 Web、受控 root helper、配置目录挂载、SIGHUP、helper 幂等缓存均已落地。
- 帮助站：根主页（中英切换）、中文 /zh/、英文 /en/、sitemap；已有适用场景。
- 官方名：**Neko's Efficient Certificate Administrator（neca/NECA）**；不使用图形 logo。
- 当前定位：单租户、CLI 优先、默认只读、可审计、可恢复的小型/中型 strongSwan 证书管理。

## 2. 重新确定开发目标

**产品目标**：在小规模 strongSwan IKEv2 部署中，以最小攻击面、可恢复的方式管理证书；破坏性操作默认由后台 root 控制，Web 只承担查看、受限写与请求。

**工程目标**：

1. **安全默认**：只读、无公网默认暴露、敏感配置不经 Web 修改。
2. **可恢复**：删除/停止/回滚都有后台确认、审计与恢复路径。
3. **可维护**：统一命令契约，明确后台 CLI、Web、helper 三种来源；新功能可加可撤。
4. **文档收敛**：里程碑、当前开发指南、历史设计分离；中文为主版本。

**暂不追求**：多租户 SaaS、复杂高可用、无人值守自动签发、默认公网 Web、OpenAPI 对外契约。

## 3. 用户已确认的决策

| # | 决策 |
|---|---|
| 1 | 紧急断开两者都提供：**<code>neca web lock</code>**（只读 + 使全部 Web 会话失效）与 **<code>neca web lockdown</code>**（停止 Web + helper）。名称简短、拼写相近。 |
| 2 | Web 只能把证书移入回收站；物理删除只能后台确认。回收站默认**不限时**，保存时长由用户配置；允许恢复已吊销证书；清理必须后台二次确认。 |
| 3 | 语言/显示等低风险偏好 Web 可直接修改；高风险安全设置默认后台完成；设置页可展示全部；是否允许 Web 修改高风险设置由后台开关决定。 |
| 4 | Web 导出默认关闭；命令为 **<code>neca web export on/off</code>**；仅导出 CSV 简易名单。 |
| 5 | 接受**大重构**：弃用 legacy 脚本旧命令，统一新命令标准，区分后台命令与 Web 命令。范围限定命令/控制面，不重写 PKI 核心。 |
| 6 | 里程碑顺序重新评估；本文件按大重构优先重排。 |
| 7 | OpenAPI **长期搁置**，不在当前版本规划内。 |
| 8 | 正式名确认：**Neko's Efficient Certificate Administrator（neca）**；正式简写 **NECA**；无图像 logo。 |
| 9 | 后台二次确认统一为交互式 **Y/N，默认 N**。 |
| 10 | 新命令统一使用 **<code>neca cert ...</code>** 命名空间；v0.5.0 直接移除旧命令。 |
| 11 | 高风险设置方案采用：后台开关默认关；前后台修改都要 Y/N 二次确认。 |
| 12 | Web 导出只导出 CSV 简易名单，区分 issued/revoked。 |
| 13 | 以后 Git commit 标题默认中文；正文统一中英双语。 |

## 4. 安全细化决策（2026-11-16 补充）

1. **会话失效**：采用持久 <code>web.sessionEpoch</code>（默认 0）。<code>neca web lock</code> 在一次配置写入中设置 <code>readOnly=true</code> 并递增 epoch，随后 SIGHUP；旧 Cookie 的 epoch 不匹配，全部失效。不依赖内存撤销表，不轮换 session.key，不重启 Web。SIGHUP 失败则命令报错并建议改用 <code>lockdown</code>。
2. **Lockdown 精确控制**：只停止明确命名的 Web/helper 服务或容器；例如 compose 的 <code>web web-helper</code>，或 systemd 的 <code>neca-web.service</code> / <code>neca-web-helper.service</code>。禁止“停掉除 X 之外所有服务”的反向逻辑；不触碰 SSH、strongSwan 或 VPN。
3. **回收站安全边界**：只保证隔离与防误删；不承诺 SSD、快照、备份介质上的安全擦除。恢复已吊销证书不会自动取消吊销；purge 需要后台二次确认。文档必须明确这一点。
4. **高风险设置**：后台开关 <code>web.allowHighRiskSettings</code> 默认 false；即使开启，前后台修改都必须 Y/N 二次确认、重新认证、CSRF、审计。建议只允许 <code>readOnly</code> / <code>idleTimeout</code> / 导出开关；<code>adminCNs</code>、passkey、helper 永远后台-only。
5. **旧命令与回滚**：v0.5.0 直接移除旧命令，不设过渡期；legacy 脚本只读归档，只提供迁移表。回滚使用 v0.4.1 镜像 tar、升级前配置/PKI 备份和旧 compose；验证索引/CRL 哈希后恢复流量。
6. **Web 导出**：只提供 CSV 简易名单，区分 issued/revoked；建议列：CN、状态、序列号、到期时间、平台；不导出 p12/私钥/zip；默认关闭，<code>neca web export on/off</code>。

### 4.7 确认与提交规范

- 后台二次确认统一交互式 <b>Y/N，默认 N</b>；不再使用输入 <code>PURGE &lt;CN&gt;</code> 的二次确认方式。
- 高风险设置无论后台还是 Web，都必须 Y/N 二次确认；Web 还需重新认证、CSRF、审计。
- 自动化允许显式 <code>--yes</code>/<code>-y</code> 跳过 Y/N；后台命令必须记录该信号与审计事件，不能隐式跳过。
- Y/N 独立为 <code>Confirmer</code> 接口/函数（默认 N），可按键功能添加或移除；不需要确认的功能不注入空壳。
- 以后 commit：标题默认中文，正文中文 + English 两段；不重写历史 commit。

## 5. 十项建议评估（更新版）

| # | 建议 | 结论与优先级 | 重写/开发难度 | 关键安全点 |
|---|---|---|---|---|
| 1 | 帮助场景 + 删除字母 n | 已完成；纯文字品牌 | 低 | 不声称商标注册 |
| 2 | 后台强制断开 | 已完成决策：lock/lockdown；M10 | 中 | 只停明确 Web/helper 服务；后台 root、审计、确认 |
| 3 | 删除证书回收站 | M11；Web 仅移入，后台 purge | 高 | 私钥仍在；恢复已吊销证书需保留吊销状态；清理二次确认；禁止 Web 物理删除 |
| 4 | Web 设置菜单 | M11；低风险偏好即时生效，高风险默认只读 | 中 | 高风险 Web 修改必须由后台开关显式开启；不能绕过 helper 边界 |
| 5 | Web 导出后台开关 | M10；默认关闭；CSV 名单 | 低–中 | CN/状态敏感；审计/限速；关闭时 UI/API 同步拒绝 |
| 6 | 大重构/命令统一 | **M9，最高优先级**；命令/控制面重构 | 高 | 破坏性变更；旧命令无过渡期；需迁移表、回滚与测试；legacy 脚本归档 |
| 7 | 里程碑独立文件 | 本文件完成 | 低 | 只保留一个权威来源 |
| 8 | 开发文档整合 | 当前指南 [DEVELOPMENT.md](DEVELOPMENT.md) 完成；旧文档历史化 | 低–中 | 历史证据保留，当前状态单一来源 |
| 9 | OpenAPI | 长期搁置，不排期 | 暂不做 | API 继续标实验性 |
| 10 | 正式全名 | 本轮同步 | 低 | 首现全名、正式简写 NECA；无图形 logo |

## 6. 新开发进程

### M9：命令与边界重构（最高优先级）

详细增删改、接口与扩展点见 [REFACTOR-PLAN.md](REFACTOR-PLAN.md)。

**目标**：在加新安全功能前，先统一命令/控制面，避免旧接口和新接口并存。

- [x] **M9.1 命令盘点与迁移表**：[M9.1-COMMAND-INVENTORY.md](M9.1-COMMAND-INVENTORY.md) 已完成并确认：downloads/package 归属、Y/N 范围、CSV 文件名、确认函数接口。
- [x] **M9.2 命令元数据/确认/审计**：Command 元数据、内置命令标注、独立 Confirmer、全局 `--yes`、审计 origin 已落地；受影响包测试通过。Web 高风险确认留到 M11。
- [x] **M9.3 Web/helper 模块化**：Web 路由拆为 public/auth/cert/download/API 组；helper 操作改为注册表；测试通过。
- [x] **M9.4 命令迁移**：`cert`/`pki`/`trash` 命名空间；旧顶层命令与 `compatArgs` 移除；最小回收站（trash/restore/purge）与 help/README 更新。
- [x] **M9.5 v0.5.0**：完整 `make all` 通过；帮助站已发布；迁移表/回滚说明完成；打 tag 并发布。
  - 基础类型/Confirmer 已落地并有单测；命令注解、审计接线、--yes 解析仍待完成。
- 建立命令元数据：名称、别名、来源（backend/web/helper）、是否写操作、确认级别、JSON、审计、废弃状态。
- 新命令标准：
  - <code>neca cert issue|list|verify|reissue|revoke|unrevoke|trash|export|platform</code>（不再提供直接物理 delete；物理 purge 走 <code>neca trash purge</code>）
  - <code>neca pki ...</code>、<code>neca config ...</code>、<code>neca web ...</code>、<code>neca trash ...</code>
- 后台命令与 Web 命令分离：Web 只能调用 helper 白名单，不执行任意 CLI；审计记录 origin=cli/web/helper。
- **旧命令 v0.5.0 直接移除，不设过渡期**；提供迁移表。legacy 脚本作为只读归档，不再作为兼容层。
- 回滚：保留 v0.4.1 镜像 tar、升级前配置/PKI 备份和旧 compose；回滚步骤写入发布说明与运维文档。
- 版本策略：M9 完成后发布 **v0.5.0**（breaking），提供从 v0.4.x 的迁移表与回滚说明。
- 出口：新增/移除一个命令只改命令定义、路由/helper 用例和测试；不改变 PKI 文件格式。

### M10：安全运维（当前阶段）

#### M10.1 配置与会话代际（已完成）

- ✅ 已增加 <code>web.sessionEpoch</code>；签名 Cookie 携带 epoch；SIGHUP 重载。
- 已完成：配置校验、示例配置、Web 状态展示、旧 Cookie 失效测试。
- 配置 schema 向后兼容，SIGHUP 原子重载；缺失默认 0。
- 单测：旧 Cookie 失效、重启后仍失效。

#### M10.2 <code>neca web lock</code>（已完成）

- ✅ 一次配置写入：<code>readOnly=true</code> + <code>sessionEpoch++</code>，随后 SIGHUP。
- ✅ 旧会话全部失效，服务保留；SIGHUP 失败时报错并建议 <code>lockdown</code>。
- ✅ Y/N 默认 N；<code>--yes</code> 仅后台自动化。

#### M10.3 <code>neca web lockdown</code>（已完成）

- ✅ 只停止明确命名的 <code>web</code> + <code>web-helper</code>；禁止反向停止。
- ✅ 不触碰 SSH、strongSwan 或 VPN；Y/N 默认 N。

#### M10.4 Web 导出开关（已完成）

- ✅ <code>neca web export on/off</code>；默认关闭。
- ✅ 仅 CSV：<code>issued.csv</code> / <code>revoked.csv</code>。
- ✅ 关闭时 Web/API 路由 404；SIGHUP 重载开关。

#### M10.5 备份、恢复与回滚演练（已完成）

- ✅ 新增 <code>scripts/backup-neca.sh</code>：备份 pki/clients/swanctl/config/web-secrets，0600 + sha256。
- ✅ 新增 <code>scripts/restore-neca.sh --yes</code>：仅停止明确命名的 web/helper，恢复后提示先只读启动。
- ✅ 回滚顺序：停 Web/helper → 恢复备份 → 校验索引/CRL → 只读启动 → 验证登录/证书/下载 → 再决定写能力。
- ✅ 备份脚本烟测通过；恢复脚本仅做语法检查，未在真实 PKI 上执行。

**出口**：两种紧急命令可审计、可确认；导出默认关闭；恢复演练通过；回滚可在 10 分钟内完成。

### M11：受控 Web 管理

#### M11.1 Web 移入回收站（已完成）

- ✅ Web 证书详情增加“移入回收站”动作；通过 helper 白名单执行。
- ✅ 新增 helper <code>trash</code> 操作、Web/API 路由与通知文案。
- Web 仍不提供物理 purge；恢复/清理只在后台 CLI。

#### M11.2 设置页与保留期（已完成）

- ✅ Web 设置页 <code>/settings</code>：侧栏二级菜单“通用/安全/关于”；语言/主题位于“通用”，高风险设置位于“安全”。
- ✅ 修复设置页 <code>template not found</code>：模板注册表缺少 <code>settings</code>，与只读模式无关。
- ✅ 后台开关 <code>web.allowHighRiskSettings</code> 默认关闭；helper 二次校验。
- ✅ 高风险修改：重新输入密码、CSRF、Y/N 确认、审计；字段白名单仅 readOnly/idleTimeout/导出。
- ✅ 回收站保留期 <code>trash.retention</code>（默认 0=不限时）；<code>trash purge --expired</code> 清理过期条目。
- ✅ 设置页展示保留期；恢复已吊销证书保留吊销状态；单条 purge 一次 Y/N（默认 N，<code>--yes</code> 可跳过）且不输入 CN；不承诺安全擦除。
- **M11 出口**：Web 移入回收站、设置页、高风险编辑、保留期与后台清理已完成；物理 purge 仍只能后台。
- 出口：删除/恢复/清理有测试；私钥访问、保留期和恢复规则明确。

### M12：可维护性与可观测性（进行中）

#### M12.1 审计轮转（已完成）

- ✅ <code>audit.maxBytes</code>（默认 10 MiB）与 <code>audit.keep</code>（默认 5）。
- ✅ CLI/helper 审计日志与 Web 审计日志共用轮转策略。
- ✅ 轮转保留权限 0600；不承诺安全擦除。

#### M12.2 诊断（已完成）

- ✅ 新增 <code>neca doctor</code>：检查配置、CA 证书/私钥、索引、CRL、Web 配置、Docker、审计日志与 helper 状态。
- ✅ 支持 <code>--json</code>；不输出密码、私钥或 Cookie。

#### M12.3 发布检查清单（已完成）

- ✅ 新增 [RELEASE-CHECKLIST.md](RELEASE-CHECKLIST.md)：发布前、升级演练、发布、发布后、回滚与签字。

#### M12.4 功能开关（已完成）

- ✅ 新增 <code>web.features</code>（如 <code>trash:false</code>、<code>settings:false</code>），默认全部开启。
- ✅ 关闭时对应 Web 路由返回 404，导航/按钮隐藏；CLI 不受影响。
- ✅ 现有 <code>web.exportEnabled</code>、<code>web.allowHighRiskSettings</code>、<code>trash.retention</code> 保留。
- 功能开关在 Web 启动时注册路由；变更后需重启 Web，SIGHUP 只重载运行参数。

**M12 出口**：审计轮转、诊断、发布/回滚清单与通用 feature flag 已完成；默认行为不变。

### v1.0.0-rc 稳定化（当前）

- 契约与冻结规则：[STABILITY.md](STABILITY.md)。
- 候选流程：v0.6.0（M10/M11）→ v0.7.0（M12）→ v1.0.0-rc.1 → v1.0.0-rc.2（设置页修复）→ v1.0.0。
- 出口：升级/回滚演练通过、安全复核无高危、CI 全绿、文档完成。
- 用户已确认：生产快照 `2026-11-16 22:00:53` 后执行升级/回滚演练；RC 观察 **1 周**；清理旧镜像。
- 生产演练已完成：v0.4.1 → v1.0.0-rc.1 → 回滚 v0.4.1 → 再升级 RC；最终生产为 RC，strongSwan 未重启。
- 观察期：2026-11-16 至 2026-11-23；期间只接受修复/文档/安全补丁。

### M13：公网访问安全（下一步，后置）

- TLS + mTLS 或 Tunnel/Access；默认仍 OpenSSH；单独威胁建模。

### M14：API/OpenAPI（长期搁置）

- 不排期；只有外部自动化需求明确后再评估。当前 API 继续标实验性。

## 7. 安全与未考虑事项

- **紧急命令语义**：<code>lock</code> 不等于吊销 VPN 会话；只能保证 Web 会话失效；<code>lockdown</code> 会中断 Web/helper，不影响 SSH/strongSwan，但需确认不会误停。
- **会话失效**：使用持久 sessionEpoch，避免内存撤销表重启丢失；epoch 必须包含在签名 Cookie 中，并由 SIGHUP 重载。
- **回收站**：私钥/证书仍在；不承诺 SSD/快照/备份上的安全删除；恢复已吊销证书不能自动取消吊销；清理需二次确认。
- **高风险设置**：后台开关一旦允许 Web 修改，就降低了 root/helper 边界；必须限制白名单、重新认证、审计、默认关闭。
- **导出**：默认关闭；仅 CSV 简易名单；开启后仍需审计/限速，避免批量枚举。
- **大重构**：最大风险是命令兼容性与误删；旧命令无过渡期，需要迁移表、回滚镜像和测试。
- **文档**：中文主版本；英文版本另行统一更新。
- **测试**：每个新功能都要有“关闭/回退”路径；破坏性操作需要故障注入和恢复测试。

## 7.1 已解决的冲突

- purge：不再输入 <code>PURGE &lt;CN&gt;</code>，保留一次 Y/N（默认 N）；自动化可用显式 <code>--yes</code> 跳过并审计。
- Y/N 与自动化：通过显式 <code>--yes</code> 解决；不接受隐式跳过。
- delete 命名：取消直接物理 delete，使用 <code>neca cert trash</code> 与 <code>neca trash purge</code>。
- 仍有安全注意：交互式确认只能降低误操作，不能替代备份与回滚。

## 8. 已确认默认与后续功能选择

以下已确认并作为实现基线：

1. 紧急命令 **<code>lock</code> / <code>lockdown</code>**，交互式 Y/N（默认 N）。
2. 新命令 **<code>neca cert ...</code>**；v0.5.0 直接移除旧命令；<code>neca cert trash</code> 移入回收站，<code>neca trash purge</code> 后台清理。
3. 高风险开关 <code>web.allowHighRiskSettings</code> 默认关闭；白名单 readOnly / idleTimeout / 导出；adminCNs、passkey、helper 永远后台-only；前后台 Y/N。
4. 回收站 <code>neca trash list|restore|purge</code>；<code>trash.retention</code> 默认 0=不限时；purge 一次 Y/N，<code>--yes</code> 可自动化跳过；不输入 CN。
5. 导出仅 CSV，区分 issued/revoked；建议列 CN、状态、序列号、到期时间、平台。
6. 大重构版本 **v0.5.0**；旧命令不保留；回滚使用 v0.4.1 镜像/配置备份。
7. 公网访问后置；OpenAPI 长期搁置。

后续新增功能优先级（用户已确认按此顺序）：

1. B. 一键备份/恢复（高价值）
2. A. 证书到期/状态概览（只读）
3. E. 配置 schema 版本/迁移
4. C. 审计日志导出/轮转
5. F. 证书导入/链验证（只读）
6. D. CLI shell completion（最后）

详见 [REFACTOR-PLAN.md](REFACTOR-PLAN.md) 第 6 节。
