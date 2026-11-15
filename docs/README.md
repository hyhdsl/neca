# neca 文档索引

长期项目文档统一放在 docs/。各组件的目录级 README（legacy/、templates/、testdata/）仍保留在对应目录，方便就地阅读。

| 文档 | 内容 |
|---|---|
| [开发文档](neca-development.md) | 架构、CLI 契约、CI/CD、路线图、验收记录 |
| [Web 面板设计评估](web-panel.md) | M6 只读开关、权限、VPN/mTLS、UI、SQLite 迁移、lite + neca-web 面板包与仓库边界评估 |
| [Web 面板开发文档](web-panel-development.md) | M6 实施规格与 M6a 记录：组件、CLI、会话、web-secrets、Docker 与验收 |
| [技术选型评估](vpn-cert-manager-rewrite-assessment.md) | Python vs Go 的历史评估（Go 已选型） |
| [服务器部署](deploy.md) | Docker 部署、配置、备份/回滚与安全说明 |
| [二次开发与许可](SECONDARY-DEVELOPMENT.md) | AGPL 义务、署名与贡献政策 |
| [旧 bash 使用说明](legacy-vpn-cert-manager.md) | 上一代脚本归档说明与回退参考 |
| [LICENSE](../LICENSE) | GNU AGPL-3.0-only 全文 |
| [NOTICE](../NOTICE) | 版权、AI 披露与署名 |

README 保留为项目入口和快速索引；目录级说明：
[legacy/README.md](../legacy/README.md)、[templates/README.md](../templates/README.md)、[testdata/README.md](../testdata/README.md)。
