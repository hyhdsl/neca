# v1.0.0-rc 稳定化与兼容契约

> 状态：v1.0.0-rc 候选；生产仍为 v0.4.1。
> 目标：在 M12 完成后冻结契约、完成升级/回滚验证，再决定 v1.0.0。

## 1. 稳定范围

以下视为 v1.0 的稳定契约：

- CLI 命名空间与命令：<code>cert</code>、<code>pki</code>、<code>trash</code>、<code>config</code>、<code>web</code>、<code>doctor</code>、<code>version</code>、<code>help</code>、<code>wizard</code>。
- 配置文件 schema v1 的已有字段；新增字段必须可选并有安全默认值。
- 退出码与 <code>--json</code> 信封的既有错误码。
- Web 默认只读、回环监听、helper 白名单边界。
- CSV issued/revoked 的字段与默认关闭语义。
- helper 协议 <code>v1</code> 的既有操作；新增操作必须可选。

## 2. 不纳入 v1.0 稳定契约

- <code>/api/v1</code> 仍为实验性，可变更。
- Go 内部包、内部文件布局、日志格式细节。
- 默认容器名、部署路径、具体 compose 文件名。
- 未来 M13 公网访问方案与 M14 OpenAPI。

## 3. 候选版本流程

1. <code>v0.6.0</code>：M10 + M11 检查点。
2. <code>v0.7.0</code>：M12 检查点。
3. <code>v1.0.0-rc.1</code>：契约冻结 + 升级/回滚演练。
4. 观察 1–2 周；只接受修复、文档和安全补丁。
5. 通过发布门槛后 <code>v1.0.0</code>。
6. M13/M14 不阻塞 v1.0。

## 4. 冻结规则

- 新功能进入下一版本；RC 期间不新增命令/字段/路由。
- 修复必须包含回归测试；安全修复可例外但需记录。
- 任何 breaking change 必须重新走 RC。
- API 实验性变更不视为 breaking，但必须更新文档。
- 生产升级必须先备份 config/PKI/web-secrets，并完成回滚演练。

## 5. 发布门槛

- <code>make all</code> 与 CI 全绿，包含 race、shadow、docs、locale。
- amd64/arm64 构建与 version 冒烟。
- v0.4.1 → 候选的升级演练。
- 备份/恢复/回滚完成，目标 10 分钟内。
- 安全复核无未解决高危项。
- 依赖/供应链检查。
- 文档、迁移表与 [发布清单](RELEASE-CHECKLIST.md) 完成。
- 生产未自动升级；部署需单独确认。

## 6. 本次生产升级/回滚演练

- 生产快照：2026-11-16 22:00:53（已确认）。
- 候选：<code>v1.0.0-rc.4</code>（签发高亮、平台勾选框与设置下拉框）；升级前生产：<code>v0.4.1</code>。
- 演练步骤：备份 config/PKI/web-secrets/clients/meta/downloads → 部署候选 → 验证 healthz/登录/证书列表/下载 → 回滚到 v0.4.1 → 验证索引/CRL/strongSwan 健康。
- 只操作明确命名的 Web/helper；不重启 strongSwan。
- 演练结果：2026-11-16 完成升级 → 验证 → 回滚 → 再升级；健康检查、doctor、CLI/Web/helper 与 strongSwan 隔离均通过。
- 最终生产状态：v1.0.0-rc.4；回滚目标 v0.4.1 已验证。
- 演练结果写入发布检查清单与任务记录。

## 7. RC 观察期

- 观察期：**1 周**，2026-11-16 至 2026-11-23（自候选部署验证完成起算）。
- 期间只接受修复、文档、安全补丁；不新增功能。
- 观察期结束后再决定是否发布 v1.0.0 正式版。

## 8. 清理决策

- 删除本地不再使用的旧容器与测试镜像。
- v0.3 及更早的项目镜像可按需清理；保留当前稳定版、候选版与开发镜像。

## 9. 回滚

- 保留 v0.4.1 与当前候选镜像/配置/PKI 备份。
- 只停止明确命名的 Web/helper 服务。
- 恢复后验证索引/CRL、登录、证书列表与 strongSwan 健康。
- 不承诺 SSD/快照/备份上的安全擦除。
