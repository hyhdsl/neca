# vpn-cert-manager 2.0 技术选型评估：Python vs Go

> **决策更新（2026-11-15）**：项目最终选定 **Go**，本文的 Python 推荐已被覆盖。决策记录、仓库规划与开发计划见 [neca-development.md](neca-development.md)。

> 评估对象：`legacy/vpn-cert-manager.sh`（506 行 bash，当前部署于 `<SERVER_IP>:/opt/ipsec/scripts/`）
> 评估日期：2026-11-15
> 评估范围：阶段一（控制台 CLI/向导）→ 阶段二（Web 面板）→ 阶段三（多用户平台）的技术栈选择
> 关联文档：`legacy/vpn-cert-manager.md`（现有脚本使用与二次开发说明）、`vpn/ipsec-vpn-deployment.md`（服务端部署，另一工作区）

---

## 0. 一页结论

| 决策 | 结论 |
|---|---|
| 是否重写 | **是**。bash 无法承载阶段二的常驻服务、阶段三的多用户与权限模型，越晚迁移越贵 |
| 阶段一语言 | **Python 3.11，纯标准库**（零第三方依赖、零 venv、单文件可部署） |
| 阶段二 Web | **FastAPI + uvicorn + SQLite**，复用同一 core，不重写业务逻辑 |
| 阶段三 多用户 | 沿用 Python core，增加 RBAC、审计与归属模型；若届时出现 §9.3 的四个信号，再评估 Go |
| 备选方案 | Go（单静态二进制、低内存、强类型）。若你更看重「一次到位不做二次选型」，现在直接选 Go 也成立，代价见 §9.4 |
| 明确不选 | 继续 bash（承载不了阶段二/三）、C（构建链缺失、重写量为 bash 的 5–8 倍、且需在持有 CA 私钥的代码里承担内存安全风险） |

**一句话**：这个项目 90% 的工作是「编排 openssl/docker + 生成文本包 + 维护证书生命周期状态」，属于胶水型业务逻辑，Python 的表达力与现有代码同构；Go 的优势在部署形态与长期强类型约束，属于后期收益。按你给出的阶段顺序，Python 的收益兑现得更早，且不会封死转 Go 的路。

---

## 1. 背景与目标形态

### 1.1 现状

现有工具是一个 506 行的 bash 脚本，管理 strongSwan IKEv2 的用户证书：签发、吊销、恢复吊销、重签发、补平台包、导出 CSV、列表、索引同步、p12 自检。它以 root 身份在服务器上运行，直接操作 `/opt/ipsec/pki`（含 CA 私钥）与 `/opt/ipsec/clients`，并通过 `docker restart ipsec-vpn` 让新 CRL 生效。

### 1.2 目标形态（按你的规划）

| 阶段 | 形态 | 关键要求 |
|---|---|---|
| 阶段一 | 控制台命令 `vpnctl <参数>` 或交互向导 | 与现有行为等价、可在 SSH 下作为 root 使用、保留自动化能力 |
| 阶段二 | Web 面板（常驻服务） | 证书列表、签发/吊销/重签、导出下载、操作审计、与 CLI 共用业务逻辑 |
| 阶段三 | 多用户平台 | 账号体系、证书归属、权限分级（管理员/普通用户）、操作留痕、并发安全 |

### 1.3 本文要回答的问题

1. 现在重写是否可行、是否值得；
2. 若可行，阶段一用 Python 还是 Go；
3. 选型如何不影响阶段二/三（避免二次重写）。

---

## 2. 现状实测（证据）

以下数据均来自 2026-11-15 对服务器的只读检查，以及本机环境探测。

| 项目 | 实测值 | 对选型的影响 |
|---|---|---|
| 服务器系统 | Debian GNU/Linux 12 (bookworm)，x86_64，glibc 2.36 | 两种语言的官方包都齐全 |
| Python | **3.11.2**，`venv` 可用，pip 23.0.1；`/usr/lib/python3.11/EXTERNALLY-MANAGED` **存在（PEP 668 生效）** | 阶段一可用纯标准库；装第三方包必须用 venv，不能直接 pip install |
| Go | **未安装** | 需在本地装工具链交叉编译，或在服务器装 Go |
| systemd | 运行中（PID 1） | 阶段二可做成 systemd 服务，开机自启 |
| 内存 | 总 1676 MB，**可用 419 MB**（已跑 6 个容器） | Python Web 可行但不宽裕；Go 更省内存 |
| 磁盘 | `/` 40G，可用 21G | 充裕 |
| 外网 | pypi 与 proxy.golang.org 均返回 HTTP 200 | 依赖拉取无障碍 |
| bash / openssl | bash 5.2.15 / OpenSSL 3.0.15 | 现状可运行 |
| 容器与版本 | `ipsec-vpn` 运行中，swanctl **5.9.8**，健康检查正常 | 契约稳定，重写不改变契约 |
| CRL 挂载 | `/opt/ipsec/swanctl` **bind mount** 到容器 `/etc/swanctl`，宿主与容器内 CRL 哈希一致 | 「写 CRL + 重启容器」的现行链路成立 |
| CA / CRL 有效期 | CA notAfter **2036-11-11**；CRL nextUpdate **2036-11-11** | 无近期到期压力 |
| 现存数据 | 3 张有效证书（SZX/HCH/HYH）+ 5 条已吊销历史；CRL 5 条 | 天然的迁移验收样本 |
| 脚本规模 | 506 行 / 29.4 KB / 50 个函数 / 23 处 openssl 调用 / 4 处 docker 调用 | 规模适中，可整体重写，不需要逐行翻译 |
| 本地环境 | Windows；git 2.56；Docker Desktop 已安装（CLI 在 resources/bin，未进 PATH）；WSL **无任何发行版**；**无 gcc、无 Go、无真实 Python**（仅有 Windows Store 占位） | Python 可以在服务器上直接开发；Go 必须先装本地工具链 |

补充：脚本的本地副本与服务器副本 SHA256 完全一致（`1e73bac3…`），本地工作区就是线上版本，可直接作为重写基线。

---

## 3. 重写必须保留的契约（硬约束）

这些是下游脚本、人工操作和服务器配置已经依赖的事实，新实现必须逐条兼容，否则迁移本身就是一次事故。

| 契约 | 内容 | 处理方式 |
|---|---|---|
| 命令行 | `--issue/--revoke/--unrevoke/--reissue/--add-platform/--set-platforms/--delete/--export-issued/--export-revoked/--list/--sync/--verify`，以及 `--platforms/--days/--out/--yes/-y/-h` | 全部保留为兼容别名，新子命令并存 |
| 输出前缀 | `[OK]  `、`[INFO] `、`[WARN] `、`[ERR]  ` 四类 | 原样保留（已被下游识别），新增 `--json` 作为结构化通道 |
| 交互 | 无参数进入向导；菜单 0–10 | 保留菜单编号语义 |
| 退出码 | 目前仅 0/1 | 明确定义 0/1/2/3（见 §11.3） |
| 环境变量 | `PKI_DIR`、`CA_CERT`、`CA_KEY`、`CLIENTS_DIR`、`CRL_DIR`、`CONTAINER`、`DOCKER_BIN`、`SERVER_ID`、`ORG`、`DEFAULT_DAYS`、`KEY_BITS` 等 | 全部保留，且优先级不变（环境变量 > 默认值） |
| 目录布局 | `pki/`、`pki/ca/`、`pki/meta/`、`pki/archive/`、`clients/<CN>/<平台>/`、`exports/` | 原样保留，不迁移、不重命名 |
| `index.txt` | 6 字段、TAB 分隔、openssl ca 数据库格式 | **继续作为 CRL 的唯一事实源**，格式不变 |
| `meta/<CN>.env` | `CN/PLATFORMS/CREATED/EXPIRES/SERIAL` | 保留，新增键向后兼容（旧 bash 用 sed 按前缀读取，加键无害） |
| p12 规范 | 3DES + SHA1、仅客户端证书、12 位字母数字密码 | 作为 neca 默认 `compat` profile 保持不变；新增 `modern`（AES-256/SHA256）等可选 profile，默认仍走兼容算法，MIUI 兼容性是硬需求 |
| 证书扩展 | `clientAuth`、SAN 含 `SERVER_ID` 与 `CN`、SKI/AKI | 不变 |

> 结论：**重写的是实现，不是行为**。新实现必须能在同一份 `index.txt`、同一套目录、同一组环境变量下，与旧脚本产出可 diff 的结果。

---

## 4. 候选方案与排除

### 4.1 继续用 bash（排除）

阶段一可以，但阶段二/三不行：

- 没有可用的 HTTP 服务、会话、并发模型；
- 结构化数据全靠 awk/sed 拼字符串，多用户权限与审计会变成不可维护的文本游戏；
- 已出现真实痛点：无文件锁（并发写坏索引）、无异常路径回滚（签发失败残留密钥）、密码文件 12/13 字节不一致。

结论：bash 作为**过渡期的生产工具**保留（重写完成前继续用），不作为 2.0 的基础。

### 4.2 Python（推荐）

见 §5。

### 4.3 Go（备选）

见 §6。

### 4.4 C（不建议）

上一轮已实测：本机无 gcc、无 WSL 发行版；服务器有 gcc 12.2 但**没有 libssl-dev / pkg-config**。即使补齐构建链，也要用 2500–4000 行重新实现 PKI、plist、中文宽度表、CSV 与菜单，而 90% 的既有工作只是调用 openssl 与 docker。更关键的是：这段代码持有 CA 私钥，C 的失败模式（缓冲区溢出、UAF）比 bash 的失败模式（文件写错）严重得多。**除非出现「必须在无 openssl CLI 的机器上跑静态二进制」的硬约束，否则不选 C。**

---

## 5. Python 详评

### 5.1 优势

| # | 优势 | 证据与影响 |
|---|---|---|
| 1 | 与现有代码同构 | 脚本里已内嵌两段 Python（iOS mobileconfig 的 plistlib 生成、--list 的中文宽度对齐）。迁移时这两段直接升格为模块，行为天然一致 |
| 2 | 标准库覆盖全部非加密需求 | argparse、pathlib、subprocess、plistlib、csv、unicodedata、sqlite3、json、logging、fcntl（flock）——**阶段一零第三方依赖** |
| 3 | 模板处理最省事 | f-string 与三引号替代 sed 转义加 heredoc 的嵌套地狱，PowerShell 模板可原样内联 |
| 4 | 服务器已装 3.11.2 | 部署等于拷贝目录加 python3 -m vpnman，无需编译 |
| 5 | 中文与编码友好 | 源码 UTF-8、unicodedata.east_asian_width 现成可用（Go 需引入 x/text/width） |
| 6 | 阶段二生态成熟 | FastAPI 自动生成 OpenAPI 文档，前端联调成本最低；SQLite 内置，无需额外服务 |
| 7 | 迭代最快 | 单人加 AI 辅助维护，改一句跑一次，最适合边用边改的运维工具 |

### 5.2 劣势与对策

| # | 劣势 | 影响 | 对策 |
|---|---|---|---|
| 1 | 需要解释器与 venv（Web 阶段） | 依赖漂移；PEP 668 禁止系统 pip | 阶段一纯标准库；阶段二 python3 -m venv 并锁定 requirements.txt |
| 2 | 内存占用高于 Go | uvicorn 单进程约 60–100 MB，服务器仅 419 MB 可用 | 单 worker、不上重依赖；面板不做长连接推送 |
| 3 | 无静态类型 | 多用户阶段约束弱 | 引入 mypy --strict 到测试门禁；核心数据结构用 dataclass |
| 4 | GIL 并发上限 | 本场景是低频管理操作，非瓶颈 | 无需处理 |
| 5 | 打包不如 Go 直观 | 无法拷一个文件就走 | 用 zipapp 或 pip install --target 目录整体部署；阶段一单文件已足够 |

---

## 6. Go 详评

### 6.1 优势

| # | 优势 | 证据与影响 |
|---|---|---|
| 1 | 单静态二进制 | CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build 在 Windows 上直接产出 Linux 可执行文件，服务器零运行时依赖 |
| 2 | 内存低 | 常驻约 15–30 MB，契合 419 MB 余量 |
| 3 | 强类型与编译期检查 | 多用户、多权限分支时，重构安全性明显更好 |
| 4 | 标准库完整 | net/http、crypto/x509、database/sql、encoding/xml、encoding/base64、text/template |
| 5 | 并发模型好 | 面板若要做任务队列、并发签发，天然合适 |
| 6 | 发布管理清晰 | 用 ldflags 注入版本，回滚等于保留上一个二进制 |
| 7 | 绕开本机 C 工具链问题 | 交叉编译不需要 WSL 发行版，这比 C 路线强得多 |

### 6.2 劣势与对策

| # | 劣势 | 影响 | 对策 |
|---|---|---|---|
| 1 | 迁移成本高 | 文本模板、CSV、中文宽度表、plist 全部要重写，估计阶段一多 40% 工时 | 接受；或先做 Python 版再按需重写 |
| 2 | 样板代码多 | 每次外部命令调用都要显式处理 error 与退出码 | 抽象 runner 包，统一捕获 stderr |
| 3 | p12 的 3DES 加 SHA1 无标准库支持 | 要么继续调用 openssl，要么引入第三方 go-pkcs12 | 阶段一继续走 openssl 子进程（与现在一致） |
| 4 | 服务器无 Go | 必须维护本地构建链 | 一次性成本，约 0.5 人日 |
| 5 | 无运行时热修改 | 改模板要重新编译发布 | 模板用 embed 外置，减少编译次数 |

---

## 7. 十二维正面对比

| 维度 | Python | Go |
|---|---|---|
| 阶段一 CLI 开发量 | 小（约 800–1200 行） | 中（约 1500–2200 行） |
| 模板与文本处理 | 强 | 中 |
| 部署 | 拷贝源码目录，需解释器 | 单个静态二进制 |
| 运行时依赖 | python3（已装） | 无 |
| 内存（Web 常驻） | 60–100 MB | 15–30 MB |
| 启动速度 | 约 40 ms | 约 5 ms |
| 类型安全 | 弱（可选 mypy） | 强 |
| 并发 | 受 GIL 限制 | 原生 |
| Web 生态 | FastAPI（最省事） | net/http 加 chi（稳健） |
| 数据库 | sqlite3 内置 / SQLAlchemy | database/sql 加驱动 |
| 测试 | unittest 与 pytest | testing 内建 |
| 学习曲线 | 低（贴近 bash 思维） | 中（需理解类型与并发） |

---

## 8. 加权评分

权重按你给出的阶段顺序设定：阶段一占 45%，阶段二占 30%，阶段三占 25%。分数为 1–5 的主观判断，用于暴露取舍，不代表精确测量。

| 维度 | 权重 | Python | Go | 加权分（Py/Go） | 说明 |
|---|---|---|---|---|---|
| 从 bash 迁移成本 | 15 | 5 | 3 | 75 / 45 | Python 与现有逻辑几乎一一对应；Go 需重写模板与文本处理 |
| 文本/模板处理 | 12 | 5 | 3.5 | 60 / 42 | PS1、mobileconfig、CSV、中文对齐表：Python 标准库覆盖 |
| 部署与运行时依赖 | 12 | 3.5 | 5 | 42 / 60 | Go 单二进制零依赖；Python 需解释器，Web 阶段需 venv |
| 迭代速度 | 10 | 5 | 4 | 50 / 40 | 单人加 AI 维护，Python 改完即跑 |
| 错误处理与类型安全 | 8 | 3.5 | 5 | 28 / 40 | Go 编译期保证；Python 需 mypy 与约定 |
| Web 框架与生态 | 10 | 5 | 4 | 50 / 40 | FastAPI 开发效率最高；Go net/http 稳健但样板多 |
| 内存与资源占用 | 5 | 3 | 5 | 15 / 25 | 服务器可用内存仅 419MB |
| 长期可维护性 | 10 | 4 | 5 | 40 / 50 | 多用户平台后期，强类型更稳 |
| 并发能力 | 5 | 3.5 | 5 | 17.5 / 25 | 本场景为低频管理操作，非瓶颈 |
| 本地开发环境就绪 | 5 | 3 | 3.5 | 15 / 17.5 | 两者本机都没有；Go 可交叉编译，Python 可直接在服务器写 |
| 测试与可验证性 | 4 | 4 | 4.5 | 16 / 18 | Go testing 内建；Python unittest 够用 |
| PKI/DB/认证生态 | 4 | 4.5 | 4 | 18 / 16 | Python cryptography/FastAPI/SQLAlchemy 成熟；Go 标准库 crypto/x509 亦完整 |
| **合计** | **100** | — | — | **426.5 / 418.5** | 折算百分制：**Python 85.3 分，Go 83.7 分** |

> 两者只差约 1.6 分，说明**不存在碾压性优势**，真正的分水岭是阶段优先级与部署形态，而不是语言本身的强弱。

---

## 9. 结论与决策规则

### 9.1 推荐

**阶段一用 Python 3.11（纯标准库）重写，core 与 CLI 分离；阶段二在 core 之上加 FastAPI；阶段三继续在 core 上扩展权限与审计。**

理由按权重排序：

1. 阶段一的收益立刻兑现——迁移成本最低、模板处理最顺、服务器已就绪；
2. 现有脚本已经内嵌 Python，迁移不会引入新语言加新范式的双重风险；
3. 阶段二的 FastAPI 与阶段三的 SQLite 足以支撑私有 VPN 证书管理这一规模（个位数到几十个用户）；
4. 部署劣势可以用单目录加 venv 加 systemd 消化，不构成阻塞。

### 9.2 为什么不继续用 bash 撑到阶段二

阶段二的本质是常驻服务、并发访问与权限模型，这三件事 bash 都不擅长。拖到那时再重写，业务逻辑会膨胀到 1500 行以上，迁移成本翻倍。**现在重写的边际成本最低**：行为已由旧脚本定义清楚，测试用例可以直接从旧脚本生成。

### 9.3 什么情况下翻盘到 Go

出现以下任意两条，就应重新评估 Go：

1. **需要分发到多台没有 Python 的机器**（单二进制价值陡增）；
2. **面板长期常驻且内存吃紧**（例如服务器再加两三个容器，可用内存跌破 200 MB）；
3. **并发或吞吐成为真实瓶颈**（例如对外提供证书自助签发，QPS 不再是每天几次）；
4. **维护者变成多人**，需要编译期约束与统一代码风格。

注意：即使翻盘，迁移的也是**编排层**。index.txt 格式、目录布局、CLI 契约、证书规范都是语言无关的，Python 版可以作为 Go 版的参考实现与测试预言（oracle），不存在推倒重来。

### 9.4 如果你更看重一次到位

可以现在直接选 Go。代价是：

- 阶段一多约 30–50% 工时（模板、CSV、中文宽度、plist 都要重写）；
- 需要先在本机安装 Go 工具链（一次性，约 0.5 人日）；
- 日常改模板的循环从改完即跑变成编译、scp、再跑。

换来的收益是：CLI 与面板用同一套强类型代码走到最后，发布形态干净（单二进制），内存更省。**这是一个合理的取舍，不是错误答案**——只是在你的阶段规划下，性价比略低。

---

## 10. 目标架构（core 与前端分离）

重写最容易犯的错，是把 bash 的单体结构原样翻译成 Python 单文件，然后在加 Web 时再拆一次。正确做法是**第一天就分离**：

```text
/opt/ipsec/vpnman/
  pyproject.toml
  src/vpnman/
    __init__.py          版本号
    cli.py               argparse 与命令分发（薄）
    wizard.py            交互向导（薄）
    core/
      config.py          环境变量与默认值，唯一来源
      proc.py            外部命令统一封装（超时、stderr、错误类型）
      pki.py             签发、重签、归档（openssl 编排）
      index.py           index.txt 读写，flock 与原子替换
      meta.py            meta/<CN>.env 读写
      crl.py             gencrl 与校验
      engine.py          docker restart 与就绪探测
      bundle/            windows.py、android.py、ios.py
      audit.py           追加式审计日志
      models.py          dataclass：Cert、Meta、IssueRequest
    web/                 阶段二：app.py（FastAPI），只依赖 core
  tests/
    fixtures/            临时 PKI 生成器
    test_*.py
```

关键约束：

- `core/` **不 import 任何 CLI 或 Web 模块**；CLI 与 Web 都只是 core 的调用方；
- `core/` 的每个函数返回结构化结果（dataclass 或 dict），**不直接打印**；打印与 JSON 输出由前端决定；
- 所有外部命令集中在 `core/proc.py`：统一超时、统一 stderr 捕获、统一错误类型；
- 路径与开关只允许在 `core/config.py` 出现一次。

这套结构在阶段二可以直接把 `web/app.py` 挂上去，不需要动 core 一行。

---

## 11. CLI 契约设计

### 11.1 命令形态

新形态用子命令，同时保留旧 flag 作为别名（迁移期双轨）：

| 新子命令 | 兼容旧写法 | 说明 |
|---|---|---|
| `vpnctl issue CN --platforms a,b --days N` | `--issue CN --platforms …` | 签发 |
| `vpnctl revoke CN` | `--revoke CN` | 吊销 |
| `vpnctl unrevoke CN` | `--unrevoke CN` | 恢复 |
| `vpnctl reissue CN` | `--reissue CN` | 重签发 |
| `vpnctl platform add CN --platforms ios` | `--add-platform CN` | 补平台包 |
| `vpnctl platform set CN --platforms …` | `--set-platforms CN` | 改标注 |
| `vpnctl delete CN [--yes]` | `--delete CN --yes` | 删除（**拒绝删除 R 行**，见 §12.4） |
| `vpnctl list [--json]` | `--list` | 列表 |
| `vpnctl verify [CN…]` | `--verify` | p12 自检 |
| `vpnctl export issued 或 revoked [--out F]` | `--export-issued` / `--export-revoked` | CSV 导出 |
| `vpnctl index sync` | `--sync` | 补索引 |
| `vpnctl`（无参数） | 无参数 | 进向导 |

### 11.2 新增通用开关

| 开关 | 作用 |
|---|---|
| `--json` | 机器可读输出（阶段二前端直接消费）；人类可读的 `[OK]` 等关闭 |
| `--dry-run` | 只打印将执行的动作，不落盘、不调用 docker |
| `--no-reload` | 只生成 CRL，不重启容器（批量运维时合并重启） |
| `--quiet` / `-q` | 仅输出错误 |

### 11.3 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功（可含 `[WARN]`） |
| 1 | 用法错误（缺参数、CN 非法、未知子命令） |
| 2 | 运行期失败（openssl / docker / 文件系统） |
| 3 | 前置条件不满足（证书已存在、容器未找到、缺 CA） |

---

## 12. 数据模型演进

### 12.1 阶段一：不改变存储

`index.txt` 仍是唯一事实源，`meta/*.env` 仍是元数据，目录结构不动。新增能力只加不删：

- meta 增加 `SCHEMA=2`、`OWNER=`、`DEVICE=`（旧 bash 读取时忽略，向后兼容）；
- 新增 `/opt/ipsec/pki/audit.log`：时间、操作者、动作、CN、序列号、结果，只追加；
- 所有写操作先 `flock -x /opt/ipsec/pki/.lock`，再 mktemp 加原子替换。

### 12.2 阶段二：只加不改

新增 SQLite（`/opt/ipsec/pki/vpnman.db`）存**用户、会话、操作审计、导出记录**。证书状态仍以 `index.txt` 为准，DB 不参与 CRL 生成。

### 12.3 阶段三：多用户

DB 增加 `users`、`roles`、`cert_owner`，引入申请-审批-签发流程。**仍然不在 DB 里存证书状态**，避免 CRL 与 DB 双写不一致。

### 12.4 必须在重写时修掉的旧问题

| 问题 | 现状 | 处理 |
|---|---|---|
| 对已吊销记录执行 `--delete` 会删掉 R 行，CRL 丢条目 | 文档已警告，脚本不拦 | 新实现：检测到该 CN 存在 R 行则**拒绝删除索引行**，只允许删文件，或要求显式 `--purge-revoked` |
| 无文件锁 | 并发会互相覆盖索引 | 全局 flock |
| 失败残留 | 签发失败留 key 与 csr | try/finally 清理未完成产物 |
| 密码文件换行不一致 | 实测 12 与 13 字节并存 | 写入统一去换行；读取统一 strip；verify 同时报长度与格式 |
| `cert_cn` 解析脆弱 | RFC2253 加 tr 逗号 | 改用 `openssl x509 -subject -nameopt sep_multiline` 解析 |

---

## 13. 迁移路线图

| 阶段 | 内容 | 完成标准 | 粗估 |
|---|---|---|---|
| 0 | git init；旧脚本冻结为 `legacy/`；建立 tests/fixtures 临时 PKI | 基线可复现，旧脚本可随时回退 | 0.5 人日 |
| 1 | 只读命令迁移：list、verify、export、index sync | 与旧脚本输出逐字节一致（列表允许空白差异） | 1.5 人日 |
| 2 | 写命令迁移：issue → reissue → revoke/unrevoke → platform → delete | 每个命令在临时 PKI 上通过 §14 用例 | 2.5 人日 |
| 3 | 加固：flock、事务式清理、--json、--dry-run、审计、退出码 | 并发压测不破坏索引；失败路径无残留 | 2 人日 |
| 4 | 切换到新 CLI：/usr/local/bin/vpnctl，旧脚本保留一个月 | 生产连续使用两周无回退 | 0.5 人日 |
| 5 | 阶段二 Web MVP（FastAPI，先只读后面板） | 面板与 CLI 共用 core，审计可查 | 3–4 人日 |

> 阶段 1 的顺序很重要：**先只读、后写入**。只读命令可以在生产目录上直接对比旧脚本输出，零风险地验证解析逻辑（证书字段、索引、元数据、CSV、表格对齐）。

---

## 14. 测试策略（影子测试）

1. **临时 PKI**：tests/fixtures 用 mktemp 生成独立目录树（PKI_DIR、CLIENTS_DIR 等全部用环境变量指向它），绝不触碰 /opt/ipsec。
2. **用例集**：直接采用 `legacy/vpn-cert-manager.md` §14 的回归清单（签发、加平台、导出、吊销、恢复、重签、删除），外加并发、缺 CA、容器缺失等故障用例。
3. **影子对比**：同一份 fixture 分别用旧 bash 与新 CLI 跑同一命令，比对：
   - `--list` 的表体（忽略列宽填充）；
   - 导出 CSV（去掉时间戳列后逐行 diff）；
   - `index.txt` 的状态、序列号、CN 三元组集合；
   - `openssl crl -text` 中的序列号集合；
   - `clients/` 目录树与文件大小。
4. **门禁**：tests/run.sh 全绿，外加 mypy --strict（Python）或 go vet（Go），才允许部署。

---

## 15. 部署与权限

### 15.1 部署形态

| 方案 | 阶段一 | 阶段二 |
|---|---|---|
| Python | 拷贝 src 到 /opt/ipsec/vpnman，/usr/local/bin/vpnctl 为两行 wrapper | 增加 venv 与 vpnman-web.service（systemd，Restart=always，User=vpnman） |
| Go | 本地交叉编译，scp 到 /usr/local/bin/vpnctl | 同左，另加 systemd unit；回滚等于换回旧二进制 |

### 15.2 权限（阶段二必须处理）

现状：脚本以 root 运行，ca.key 为 0600 root。阶段二如果让 Web 服务直接以 root 跑，是明显的降级。建议：

1. 新建 vpnman 系统用户；ca.key 改为 0640 root:vpnman（**这是一次有意的安全放宽，需要你确认**）；
2. index.txt、meta/、clients/、exports/ 归属 vpnman:vpnman；
3. 重启容器通过受限 sudoers：vpnman ALL=(root) NOPASSWD: /usr/bin/docker restart ipsec-vpn，不授予通用 docker 权限；
4. 面板只监听 127.0.0.1，对外通过反向代理加认证，或仅允许 VPN 内网访问；
5. 审计日志只追加，必要时用 chattr +a。

---

## 16. 风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|
| 重写改坏生产 PKI | 中 | 高 | 先只读迁移；影子测试；迁移期旧脚本保底；动手前整目录备份 |
| 删 R 行导致 CRL 丢吊销 | 中 | 高 | 新实现硬性拦截（§12.4） |
| 并发写坏索引 | 中 | 中 | flock 加原子替换加并发用例 |
| PEP 668 导致装依赖失败 | 高 | 低 | 阶段一零依赖；阶段二 venv 加 requirements.txt 锁定 |
| 内存不足（419 MB 可用） | 中 | 中 | 单 worker；不上 Redis 与重型 ORM；必要时用 Go 重写服务层 |
| 本机无 Python 或 Go | 确定 | 低 | Python 在服务器上开发；Go 需先装工具链 |
| 吊销必须重启容器、断所有会话 | 高 | 中 | 作为独立课题：实验 swanctl --load-creds 能否刷新 CRL；若可，改为免重启 |
| Web 面板成为 CA 私钥的暴露面 | 中 | 高 | 最小权限加仅内网加审计；面板不另存私钥明文 |

---

## 17. 待你确认的决策点

1. **软件名**：建议 `vpnctl`（命令）与 `vpnman`（包名），也可沿用 vpn-cert-manager；
2. **是否保留旧 flag 双轨**：建议保留一个版本周期，方便现有习惯与脚本；
3. **p12 是否继续走 openssl 子进程**：建议继续（3DES 加 SHA1 是 MIUI 兼容性硬需求，重实现风险高）；
4. **ca.key 是否放宽到 0640 root:vpnman**：阶段二必须做决定；
5. **是否做 CRL 免重启实验**：建议在维护窗口验证一次 swanctl --load-creds，收益（不断线吊销）很高。

---

## 18. 附录：环境快照（2026-11-15）

| 项 | 值 |
|---|---|
| 服务器 | Debian 12 bookworm，x86_64，glibc 2.36，systemd，内存 1676 MB（可用 419 MB），磁盘可用 21 GB |
| 依赖 | bash 5.2.15、OpenSSL 3.0.15、python3 3.11.2（PEP 668）、pip 23.0.1、sqlite 3.40.1、docker、column、curl |
| 容器 | ipsec-vpn（strongSwan / swanctl 5.9.8）、existing-service、other service、existing-service、existing-service 等共 6 个 |
| 挂载 | /opt/ipsec/swanctl 到 /etc/swanctl（rw），charon-logging.conf（ro） |
| 数据 | pki/ 下 3 张有效证书加 5 条已吊销索引；clients/ 下 3 个客户包；CRL 5 条 |
| 有效期 | CA 到 2036-11-11；CRL nextUpdate 2036-11-11 |
| 本机 | Windows 11，git 2.56，Docker Desktop（CLI 不在 PATH），WSL 无发行版，无 gcc、无 Go、无真实 Python |