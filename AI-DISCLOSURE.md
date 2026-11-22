# AI 参与披露

> 适用范围：**neca v1.0.0**（2026-11-22）。本文说明 AI 在本项目中的实际参与范围、人类维护者的职责，以及使用者应如何理解本项目的测试与验收结果。

## 一句话结论

**本项目的代码编写、调试、测试、CI/构建脚本、部署脚本、文档和帮助站内容由 AI 编程助手完成；人类维护者主要负责提出需求、检查功能是否符合预期、审查变更并决定是否发布。** 人类的功能验收不等于逐行代码审计或安全审计。

## 实际分工

| 工作 | 主要由谁完成 |
|---|---|
| Go 代码、CLI/Web/helper 实现 | AI |
| 测试代码、测试数据、回归/影子测试 | AI |
| 调试、错误定位、修复实现 | AI |
| CI/CD、Dockerfile、Compose、发布脚本 | AI |
| 部署文档、设计文档、帮助文档、英文核心页 | AI |
| 需求定义、范围取舍、发布决策 | 人类维护者 |
| 功能验收：实际运行并检查是否符合预期 | 人类维护者 |
| 生产服务器操作、凭据/密码/私钥保管 | 人类维护者 |
| 最终合并、发布、回滚决策 | 人类维护者 |

补充说明：

- 人类维护者可以要求 AI 增添、删除或修改实现与文档，并会检查关键功能是否按预期运行。
- 人类维护者不逐行编写或审查全部实现；因此本项目不应被理解为经过了完整的人工安全审计。
- 生产环境中的真实部署、权限、备份和应急操作仍由人类负责。

## AI 工具

当前开发使用 **DeepSeek Harness / DeepSeek 模型**作为 AI 编程助手；也可能使用本地 Docker、Git、GitHub CLI 等工具执行构建、测试与发布操作。具体模型和工具版本可能随开发过程变化。

## 对使用者的含义

1. **功能验收是有限的**：测试通过表示已定义的测试和人工验收通过，不表示所有边界、攻击面或部署环境都已覆盖。
2. **AI 可能产生错误**：包括错误实现、遗漏、错误文档、过期示例或不安全默认值。使用前请自行审查、测试并承担风险。
3. **不是安全保证**：本项目处理 CA 私钥、VPN 证书和 Web 管理面；生产使用前应做独立的安全评审、权限检查和备份演练。
4. **许可证仍然有效**：AI 参与不改变 **AGPL-3.0-only** 的许可、源码公开与署名义务。
5. **披露会更新**：随着开发方式变化，本文会补充或修订；以仓库当前版本为准。

## 可核验信息

- 源码与提交历史：<https://github.com/hyhdsl/neca>
- CI 记录：<https://github.com/hyhdsl/neca/actions>
- 发布记录：<https://github.com/hyhdsl/neca/releases>
- 许可：<https://github.com/hyhdsl/neca/blob/main/LICENSE>
- 版权与 AI 披露摘要：[NOTICE](NOTICE)
- 二次开发与 AGPL 义务：[docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)

## English summary

All code authoring, debugging, testing, CI/build/deploy scripting, documentation and
help-site content in this project were produced by an AI coding agent (DeepSeek
Harness / DeepSeek model). The human maintainer defines requirements, reviews changes,
performs functional acceptance, handles production credentials and makes release
decisions. Functional acceptance is not a line-by-line or security audit. AI output may
contain errors; review and test before production use. The project remains licensed
under AGPL-3.0-only.
