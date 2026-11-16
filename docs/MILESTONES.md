# neca 开发目标与里程碑

> 更新：2026-11-16 · 当前基线：v0.4.1（tag 6060c2b）
> 当前 main：e53d165；帮助站 neca-help：14a7c1a。
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
| 1 | 紧急断开提供两种命令，名称简短且拼写相近。建议 **<code>neca web lock</code>**（只读 + 使全部 Web 会话失效，服务保留）与 **<code>neca web lockdown</code>**（停止 Web + helper 容器）。 |
| 2 | Web 只能把证书移入回收站；物理删除只能后台确认。回收站默认**不限时**，保存时长由用户配置；允许恢复已吊销证书；清理必须后台二次确认。 |
| 3 | 语言/显示等低风险偏好 Web 可直接修改；高风险安全设置默认后台完成；设置页可展示全部；是否允许 Web 修改高风险设置由后台开关决定。 |
| 4 | Web 导出默认关闭；命令为 **<code>neca web export on/off</code>**。 |
| 5 | 接受**大重构**：弃用 legacy 脚本保留的旧命令，统一新命令标准，区分后台命令与 Web 命令。范围限定命令/控制面，不重写 PKI 核心。 |
| 6 | 里程碑顺序重新评估；本文件按大重构优先重排。 |
| 7 | OpenAPI **长期搁置**，不在当前版本规划内。 |
| 8 | 正式名确认：**Neko's Efficient Certificate Administrator（neca）**；正式简写 **NECA**；无图像 logo。 |

## 4. 十项建议评估（更新版）

| # | 建议 | 结论与优先级 | 重写/开发难度 | 关键安全点 |
|---|---|---|---|---|
| 1 | 帮助场景 + 删除字母 n | 已完成；纯文字品牌 | 低 | 不声称商标注册 |
| 2 | 后台强制断开 | 已完成决策：<code>lock</code> / <code>lockdown</code>；M10 | 中 | 只停 Web/helper，绝不能误停 SSH/strongSwan；后台 root、审计、确认 |
| 3 | 删除证书回收站 | M11；Web 仅移入，后台 purge | 高 | 私钥仍在；恢复已吊销证书需保留吊销状态；清理二次确认；禁止 Web 物理删除 |
| 4 | Web 设置菜单 | M11；低风险偏好即时生效，高风险默认只读 | 中 | 高风险 Web 修改必须由后台开关显式开启；不能绕过 helper 边界 |
| 5 | Web 导出后台开关 | M10；默认关闭；<code>neca web export on/off</code> | 低–中 | CN/状态敏感；审计/限速；关闭时 UI/API 同步拒绝 |
| 6 | 大重构/命令统一 | **M9，最高优先级**；命令/控制面重构 | 高 | 破坏性变更；需迁移表、兼容期/警告、测试与回滚；legacy 脚本归档不再扩展 |
| 7 | 里程碑独立文件 | 本文件完成 | 低 | 只保留一个权威来源 |
| 8 | 开发文档整合 | 当前指南 [DEVELOPMENT.md](DEVELOPMENT.md) 完成；旧文档历史化 | 低–中 | 历史证据保留，当前状态单一来源 |
| 9 | OpenAPI | 长期搁置（M14 之外，不排期） | 暂不做 | API 继续标实验性 |
| 10 | 正式全名 | 本轮同步 | 低 | 首现全名、正式简写 NECA；无图形 logo |

## 5. 新开发进程

### M9：命令与边界重构（最高优先级）

**目标**：在加新安全功能前，先统一命令/控制面，避免旧接口和新接口并存。

- 盘点 legacy 脚本命令、当前 CLI 命令、Web 路由、helper 操作。
- 建立命令元数据：名称、别名、**来源**（backend/web/helper）、是否写操作、确认级别、JSON、审计、废弃状态。
- 新命令标准建议：
  - <code>neca cert issue|list|verify|reissue|revoke|unrevoke|delete|export|platform</code>
  - <code>neca pki ...</code>、<code>neca config ...</code>、<code>neca web ...</code>、<code>neca trash ...</code>
- 后台命令与 Web 命令分离：Web 只能调用 helper 白名单，不执行任意 CLI；审计记录 origin=cli/web/helper。
- 旧命令/别名：标记 deprecated，输出警告；迁移表完成后移除。legacy 脚本作为只读归档，不再作为兼容层。
- 版本策略：M9 完成后发布 **v0.5.0**（breaking），提供从 v0.4.x 的迁移表与回滚说明。
- 出口：新增/移除一个命令只改命令定义、路由/helper 用例和测试；不改变 PKI 文件格式。

### M10：安全运维（M9 后）

- <code>neca web lock</code>：强制 readOnly + 使全部 Web 会话失效；服务保留。
- <code>neca web lockdown</code>：停止 Web + helper；不触碰 strongSwan/SSH。
- <code>neca web export on/off</code>：默认关闭；Web/API 守卫；审计。
- 备份/恢复与回滚演练：PKI、config、web-secrets、clients、meta、downloads。
- 出口：两种紧急命令可审计、可确认；导出默认关闭；恢复演练通过。

### M11：受控 Web 管理

- Web 设置页：低风险偏好（语言/显示）可直接改；高风险安全设置默认只读展示。
- 后台开关决定是否允许 Web 修改高风险设置；开启时需重新认证、CSRF、审计与确认。
- 证书回收站：Web 仅移入；后台 <code>neca trash list|restore|purge</code>；默认不限时；清理二次确认。
- 出口：删除/恢复/清理有测试；私钥访问、保留期和恢复规则明确。

### M12：可维护性与可观测性

- 完成 M9 的后续模块化；feature flag；日志/审计保留与轮转；发布/回滚检查清单。
- 出口：不改变外部行为；新增功能可通过开关回退。

### M13：公网访问安全（后置）

- TLS + mTLS 或 Tunnel/Access；默认仍 OpenSSH；单独威胁建模。

### M14：API/OpenAPI（长期搁置）

- 不排期；只有外部自动化需求明确后再评估。当前 API 继续标实验性。

## 6. 安全与未考虑事项

- **紧急命令语义**：<code>lock</code> 不等于吊销 VPN 会话；只能保证 Web 会话失效；<code>lockdown</code> 会中断 Web/helper，不影响 SSH/strongSwan，但需确认不会误停。
- **会话失效**：当前签名 Cookie + 内存撤销表，重启会丢失；<code>lock</code> 需要持久 session epoch 或密钥轮换，且要考虑 Web 挂载/重启。
- **回收站**：私钥/证书仍在；不承诺 SSD/快照/备份上的安全删除；恢复已吊销证书不能自动取消吊销；清理需二次确认。
- **高风险设置**：后台开关一旦允许 Web 修改，就降低了 root/helper 边界；必须限制白名单、重新认证、审计、默认关闭。
- **导出**：默认关闭；开启后仍需审计/限速，避免批量枚举。
- **大重构**：最大风险是命令兼容性与误删；先做 inventory + deprecation shim，不一次性删除；legacy 脚本保留归档但不扩功能。
- **文档**：中文主版本；英文版本另行统一更新。
- **测试**：每个新功能都要有“关闭/回退”路径；破坏性操作需要故障注入和恢复测试。

## 7. 仍需用户决策

1. 紧急命令名是否采用 **<code>lock</code> / <code>lockdown</code>**？是否都要求 <code>--confirm</code>？
2. 大重构的新命令命名是否采用 <code>neca cert ...</code> 命名空间？旧命令保留一个版本的警告期，还是 v0.5.0 直接移除？
3. 高风险设置的后台开关名与白名单：是否只允许 <code>readOnly</code> / <code>idleTimeout</code> / 导出开关，而 <code>adminCNs</code>/passkey/helper 永远后台-only？
4. 回收站命令名（<code>neca trash list|restore|purge</code>）与保留期配置键；清理二次确认采用输入 CN 还是 <code>--confirm</code>？
5. 导出格式：CSV 全量、单证书，还是两者？
6. 大重构完成后版本号用 **v0.5.0** 是否接受？是否需要同时保留旧命令至少一个版本？
7. 公网访问继续后置；OpenAPI 长期搁置；是否确认？
