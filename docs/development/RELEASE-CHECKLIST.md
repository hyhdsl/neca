# neca 发布与回滚检查清单

> 适用于 v0.5.0 之后的版本；生产发布前逐项确认。不要在本文件记录主机、端口、哈希或凭据。

## 1. 发布前

- [ ] 版本号、tag 与变更范围确认
- [ ] CI 全绿：gofmt、vet、race tests、check-ascii、check-locales、check-docs
- [ ] 交叉编译 amd64/arm64
- [ ] 影子对比通过
- [ ] 安全复核：helper 白名单、高风险设置、回收站、导出、审计
- [ ] 依赖/供应链检查
- [ ] 配置 schema 向后兼容；示例配置已更新
- [ ] 迁移表覆盖所有 breaking 命令
- [ ] 文档、帮助站与 README 同步
- [ ] 备份当前 config/PKI/web-secrets/clients/meta/downloads
- [ ] 回滚镜像/旧 compose 已保留

## 2. 升级演练

- [ ] 在非生产环境完成旧版 → 新版
- [ ] 登录、证书列表、p12 验证、下载、签发/吊销/回收站流程
- [ ] lock/lockdown 只影响明确 Web/helper 服务
- [ ] 审计日志可读且轮转正常
- [ ] 回滚到旧版并验证索引/CRL
- [ ] 回滚耗时记录；目标 10 分钟内

## 3. 发布

- [ ] 打 annotated tag
- [ ] 构建并上传 amd64/arm64 二进制与 checksums
- [ ] 发布 GitHub Release
- [ ] 推送 GHCR 多架构镜像（lite/web）
- [ ] 发布帮助站
- [ ] 记录发布说明中的 breaking changes

## 4. 发布后

- [ ] 生产健康检查 /healthz
- [ ] Web 登录与只读行为
- [ ] 关键命令冒烟
- [ ] 审计日志写入
- [ ] 监控 24–48 小时
- [ ] 更新状态文档与已知问题

## 5. 回滚

- [ ] 停止明确命名的 Web/helper
- [ ] 恢复升级前 config/PKI 备份
- [ ] 加载旧镜像与旧 compose
- [ ] 验证索引/CRL 哈希与 strongSwan 健康
- [ ] 只读启动并验证登录/证书
- [ ] 记录事件与根因

## 6. 本次 RC 演练记录

| 项 | 内容 |
|---|---|
| 生产快照 | 2026-11-16 22:00:53 |
| 升级前版本 | v0.4.1 |
| 候选版本 | v1.0.0-rc.9 |
| 备份 | 已完成（config、PKI、web-secrets、clients、meta、downloads） |
| 升级验证 | 通过：healthz、login 200、CLI/Web/helper 健康、doctor 全 OK |
| 回滚验证 | 通过：v0.4.1 Web/helper 健康、CLI v0.4.1、strongSwan 健康 |
| 最终状态 | 生产 v1.0.0-rc.8；RC.9 已发布、待部署确认 |
| 观察期 | 1 周：2026-11-16 至 2026-11-23 |
| 结论 | 升级/回滚演练通过；待观察期结束决定 v1.0.0 |

## 7. 签字

| 角色 | 姓名 | 日期 | 结论 |
|---|---|---|---|
| 负责人 |  |  |  |
| 安全复核 |  |  |  |
| 发布执行 |  |  |  |
