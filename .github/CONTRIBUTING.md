# 分支、PR 与发布标准

> 适用于 neca 维护者。目标是保持 main 可发布、历史线性、发布可追溯；不引入长期 develop 分支。

## 长期分支

- `main`：唯一长期开发分支，始终是下一个版本的集成目标。
- `release/x.y`：仅当已发布稳定线需要紧急补丁、而 main 已经进入下一版本开发时，从该稳定 tag 临时创建；补丁发布后删除。
- 不保留长期 `develop`、`release/*` 或 `hotfix/*` 分支。

## 短生命周期分支

从 `main` 创建（旧稳定线补丁从对应 `release/x.y` 创建）：

| 前缀 | 用途 | 示例 |
|---|---|---|
| `feat/` | 新功能、新能力 | `feat/cert-key-import` |
| `fix/` | 缺陷修复 | `fix/trash-route` |
| `docs/` | 仅文档 | `docs/branch-standard` |
| `chore/` | 构建、依赖、CI、维护 | `chore/ci-release-preflight` |
| `security/` | 安全修复；必要时使用受限说明 | `security/audit-path` |
| `hotfix/` | 旧稳定线紧急修复，必须基于 release/x.y | `hotfix/1.3.2-crash` |

命名使用小写字母、数字和短横线；不使用个人姓名、日期或宽泛的 `update`。一个分支只做一个逻辑变更。

## PR 规则

- 所有变更经 Pull Request 合并；禁止直接 push main。
- 默认 base 为 `main`；旧稳定线补丁 base 为对应 `release/x.y`。
- 合并方式：squash merge；保持线性历史；合并后删除 head 分支。
- 必需检查：`build-and-test`；启用 conversation resolution。
- main 已启用管理员约束、禁止强推/删除；release/x.y 创建时必须采用同等保护。
- PR 标题采用 `type(scope): 中文 / English`，与提交规范一致。
- PR 描述必须写明变更范围、验证证据、风险与回滚方式；涉及证书、权限、部署配置或生产行为的变更必须单独说明。
- 不在 PR、日志、测试数据或文档中提交生产主机、端口、凭据、内部哈希、密码路径或真实证书材料。

## 版本与 tag

- 稳定版：`vMAJOR.MINOR.PATCH`。
- 预览版：`vMAJOR.MINOR.PATCH-preview.N`。
- tag 使用 annotated tag；发布后不可移动、不可复用、不可强推。
- 稳定 tag 只能从 `main` 或对应的 `release/x.y` 创建；tag 所在提交必须已有成功的 `build-and-test`。
- 发布顺序：合并 PR → 确认 main/ release 分支 CI 成功 → 创建 tag → 等待 release/docker 工作流 → 验证资产、镜像与部署。
- 旧稳定线补丁：从稳定 tag 建 `release/x.y` → 修复 PR → CI → patch tag → 发布 → 删除 release 分支；如修复同样适用于 main，另行 cherry-pick/PR。

## 工作流边界

- `ci.yml`：PR 与功能分支门禁，含单元测试、race、漏洞、文档、交叉编译和 Docker/Compose 冒烟。
- `release.yml`：tag 发布前校验 tag 格式、来源分支、CI 结果，再生成 Release 资产。
- `docker.yml`：同样先校验 tag 与 CI，再推送 GHCR 双架构镜像；不再支持无 tag 的手工推送。
