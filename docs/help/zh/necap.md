# necap 认证组件（v1.7 开发中）

> 适用于 v1.7 necap。当前仍为开发版，尚未发布或部署生产；生产继续使用 v1.5.0。

necap 是独立的可选认证服务，负责密码、TOTP、Passkey、恢复码和第二因素策略。Web 只做 UI 与会话；所有认证请求经 helper 通过 Unix socket 转发。necap 不监听 TCP，不挂载 CA、私钥或 docker.sock。

## 支持范围

- 密码
- 密码 + TOTP
- 密码 + Passkey
- 密码 + 任一已注册第二因素
- 不支持 TOTP-only 或 Passkey-only 无密码登录。

每用户可以同时注册 TOTP 和 Passkey；数据库已按多用户模型设计，但 v1.7 只开放单管理员 UI。

## 前置条件

- 已有可用的 Web 面板和 web-helper。
- 已有 bcrypt 管理员密码哈希文件，默认路径 /opt/ipsec/web-secrets/password.hash。
- RP ID 与 Origin；例如 RP ID 为 neca.example.com，Origin 为 https://neca.example.com。
- 独立目录 /opt/ipsec/necap 和 /run/necap，属主为 necap 运行用户 10002。

~~~bash
install -d -m 0700 -o 10002 -g 10002 /opt/ipsec/necap
install -d -m 0700 -o 10002 -g 10002 /run/necap
~~~

如果没有密码哈希文件，先在受控终端生成：

~~~bash
neca web hash-password > /opt/ipsec/web-secrets/password.hash
chmod 600 /opt/ipsec/web-secrets/password.hash
~~~

## 初始化

先停止现有一键启动的 Web/helper，然后执行一次离线初始化：

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile necap run --rm   --user 0:0 --entrypoint /usr/local/bin/necap necap init   --config /opt/ipsec/necap/necap.json   --rp-id neca.example.com   --rp-name "neca Admin"   --origins https://neca.example.com   --uid 10002 --gid 10002
~~~

初始化会创建：

- necap.json（0600）
- necap.db（0600）
- master.key（0600）

三个文件会 chown 到 10002:10002；配置目录会收紧为 0700。不要把 master.key 与数据库放在一起备份之外的地方。

## 启用

在 /opt/ipsec/neca.env 增加：

~~~text
NECA_NECAP_ENABLED=1
NECA_NECAP_USERNAME=admin
~~~

启动 Web、写 helper 和 necap：

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml   --profile web --profile web-write --profile necap   up -d web web-helper necap
~~~

Web 登录流程：

1. 输入密码；密码由 necap 校验。
2. 已配置第二因素时进入 TOTP/Passkey/恢复码页面。
3. 第二因素成功后才签发 Web 会话。

打开“设置 → 安全”可以：

- 重新验证管理身份
- 注册/删除 TOTP
- 注册/列出/删除 Passkey
- 查看剩余恢复码并重新生成
- 修改第二因素策略

安全设置中的再认证使用密码加 TOTP 或恢复码；Passkey 用户可使用恢复码。

## CLI

在 root/helper 可用且已完成再认证后：

~~~bash
neca pass status
neca pass totp add
neca pass totp remove
neca pass passkey list
neca pass passkey remove <ID>
neca pass recovery regenerate
neca pass policy <password|totp|passkey|any>
~~~

Passkey 注册只在 Web 完成；CLI 只列出和删除。CLI 写操作会重新要求密码和现有第二因素/恢复码。当前不可用 Passkey 完成 CLI 再认证，请使用恢复码或 Web。

## 回滚

1. 从 /opt/ipsec/neca.env 删除 NECA_NECAP_ENABLED。
2. 停止 necap 容器。
3. 重启 web 与 web-helper。
4. 保留或备份 /opt/ipsec/necap 目录；回滚不需要修改 neca.json 或 Web 密码哈希。

回滚后 Web 恢复原密码哈希登录。necap 数据库、TOTP secret、Passkey 公钥和恢复码不会自动删除。

## 边界与注意

- necap 不监听 TCP，不访问 CA 私钥或 docker.sock。
- necap.db 与 master.key 必须 0600；配置目录 0700。
- master.key 丢失会导致 TOTP secret 无法解密；必须单独备份。
- 恢复码只存哈希，明文只显示一次。
- 删除当前策略依赖的唯一第二因素会被拒绝，避免管理员锁死。
- v1.7 不提供无密码登录，也不提供多用户 Web UI。
- 已验证：密码→TOTP 的真实 necap+helper+Web HTTP E2E，以及 Passkey 注册/断言的软件认证器测试。
- 真实浏览器 Passkey 流程和发布前安全评审仍待完成。
