# neca

基于 Go 的 strongSwan IKEv2 用户证书管理工具（CLI，后续含 Web 面板）。

前身是 `vpn-cert-manager.sh`（506 行 bash），负责 `/opt/ipsec` 下的证书签发、吊销、重签发、平台包生成与 CRL 维护。

- 仓库：https://github.com/hyhdsl/neca （私有）
- 技术路线：Go 1.25，单静态二进制，`CGO_ENABLED=0`
- 运行形态：服务器上以 root 执行，`ca.key` 保持 `0600 root:root`

## 文档

| 文档 | 内容 |
|---|---|
| [neca-development.md](neca-development.md) | 开发文档：架构、CLI 契约、CI/CD、路线图、实验记录 |
| [vpn-cert-manager-rewrite-assessment.md](vpn-cert-manager-rewrite-assessment.md) | 技术选型评估（Python vs Go） |
| [vpn-cert-manager.md](vpn-cert-manager.md) | 现有 bash 脚本使用与二次开发说明 |

## 现状

- 设计阶段，代码尚未开始（里程碑 M0 之前）。
- 2026-11-15 完成 `swanctl --load-creds` 实测：新 CRL 可热替换，吊销不再需要重启容器。
- 基线脚本、文档已入库；`main` 为保护分支。

## 里程碑

| 里程碑 | 内容 |
|---|---|
| M0 | 仓库骨架、Makefile、Dockerfile.dev、CI |
| M1 | 只读命令（list / verify / export / index sync） |
| M2 | 写命令（issue / reissue / revoke / platform / delete） |
| M3 | 向导、JSON、dry-run、审计、退出码 |
| M4 | 热加载模式（已完成实验） |
| M5 | v0.1.0 发布与服务器切换 |
| M6 | Web 面板 |
