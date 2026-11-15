# 服务器部署（Docker）

仓库 `deploy/` 目录提供 neca 的 Docker 部署文件。发布两个镜像：

| 镜像 | 内容 | 默认 |
|---|---|---|
| `neca:vX.Y.Z` | CLI + Web 配置管理；不含 `necad` | 是（lite） |
| `neca-web:vX.Y.Z` | `necad` + helper 占位；不含 lite CLI | 否（可选 Web 面板包） |

lite 容器通过 `/var/run/docker.sock` 调用宿主机 docker，对 strongSwan 执行
`swanctl --load-creds`。Web 容器不挂载 docker.sock、`ca.key` 或客户端私钥。

## 1. 构建与传输

在仓库根目录（已安装 Docker）执行：

```bash
make build
docker build --target lite -t neca:v0.3.0 .
docker build --target web -t neca-web:v0.3.0 .
docker save neca:v0.3.0 neca-web:v0.3.0 | gzip > /tmp/neca-v0.3.0.tar.gz
scp /tmp/neca-v0.3.0.tar.gz root@<SERVER>:/tmp/
```

服务器：

```bash
docker load < /tmp/neca-v0.3.0.tar.gz
```

只装 lite 时只需加载/传输 `neca:v0.3.0`；启用 Web 时两个镜像必须使用同一
版本标签。

## 2. 部署文件

```bash
install -m 0600 deploy/neca.env.example /opt/ipsec/neca.env
install -m 0644 deploy/docker-compose.neca.yml /opt/ipsec/docker-compose.neca.yml
install -m 0755 deploy/init-web-secrets.sh /opt/ipsec/init-web-secrets.sh
chmod 0600 /opt/ipsec/neca.env
```

业务设置统一放在 `/opt/ipsec/neca.json`，`neca.env` 只保留 bootstrap：

```bash
# 新部署：从模板创建后编辑真实值
install -m 0600 deploy/neca.json.example /opt/ipsec/neca.json

# 从已有环境变量迁移（neca.env 仍含完整设置时）
neca config init --from-env
neca config validate
```

真实 `SERVER_ID` / `ORG` / `CA_CN` 只写服务器 `/opt/ipsec/neca.json`，
不提交仓库。优先级：**CLI flag > 环境变量 > neca.json > 默认值**。

## 3. 安装便捷命令

```bash
install -m 0755 deploy/neca-wrapper.sh /usr/local/bin/neca
neca version
neca list
```

包装命令会在非交互环境自动加 `docker compose run -T`，并把当前 shell 的
`NECA_LANG` 传入容器。

## 4. 直接运行 lite

```bash
cd /opt/ipsec
docker compose -f docker-compose.neca.yml run --rm neca list
docker compose -f docker-compose.neca.yml run --rm neca verify
docker compose -f docker-compose.neca.yml run --rm neca issue TEST01 --dry-run
```

写操作建议先 `--dry-run`，再在维护窗口执行。审计日志在
`NECA_AUDIT_LOG`（默认 `/opt/ipsec/pki/audit.log`）。

## 5. 可选 Web 面板（M6）

### 5.1 启用配置

编辑 `/opt/ipsec/neca.json`：

```json
{
  "web": {
    "enabled": true,
    "listen": "127.0.0.1:18080",
    "readOnly": true,
    "idleTimeout": "30m",
    "adminCNs": [],
    "passkey": { "enabled": false, "rpId": "", "rpName": "neca Admin" }
  }
}
```

允许的空闲超时为 5m–24h，绝对会话上限固定 24h。默认只读；M6a 只提供只读
面板，M6b 在 root helper 启动后才允许 `neca web readonly off`。

### 5.2 初始化 web-secrets

在服务器以 root 执行：

```bash
NECA_IMAGE=neca:v0.3.0 /opt/ipsec/init-web-secrets.sh
```

脚本创建：

```text
/opt/ipsec/web-secrets/             root:neca-web 0710
  session.key                       root:neca-web 0640
  password.hash                     root:neca-web 0640
  web-audit.log                     root:neca-web 0660
```

镜像内 necad 固定使用 UID/GID 10001；脚本会创建/校验宿主机 `neca-web`
组 GID=10001。密码只从终端读取，不写入 shell 历史或 `neca.env`。

### 5.3 文件权限

Web 进程是非 root，必须能读取配置、客户端包和下载 zip：

```bash
chgrp neca-web /opt/ipsec/neca.json
chmod 0640 /opt/ipsec/neca.json

chgrp -R neca-web /opt/ipsec/clients /opt/ipsec/downloads
chmod -R g+rX /opt/ipsec/clients /opt/ipsec/downloads
find /opt/ipsec/clients /opt/ipsec/downloads -type f -exec chmod g+r {} +

chmod 0644 /opt/ipsec/pki/ca/index.txt
chmod -R a+rX /opt/ipsec/pki/meta
```

只要 `clients/`、`downloads/` 的组为 `neca-web`，CLI 后续生成的 p12、zip
与 sidecar 会自动继承该组并设置 0640；不需要每次签发后手工重跑。

不要把 `/opt/ipsec/pki` 整体挂载给 Web；compose 只挂载
`pki/ca.crt`、`pki/ca/index.txt`、`pki/meta`、`clients`、`downloads`、
`swanctl/x509crl/ca.crl.pem`（公开 CRL，用于计数）和 `web-secrets` 中的单个文件。

### 5.4 启动与访问

```bash
mkdir -p /run/neca && chmod 0755 /run/neca
cd /opt/ipsec
docker compose -f docker-compose.neca.yml --profile web up -d web
docker compose -f docker-compose.neca.yml --profile web logs -f web
```

Web 默认监听 `127.0.0.1:18080`，compose 使用 host network 但不发布端口。
本次生产环境 18080 已被 other service 占用，`web.listen` 使用 18080；下面以生产端口
为例。若使用默认端口，将命令中的 18080 换成 18080。推荐通过系统原生 OpenSSH
本地转发访问：

```bash
ssh -L 18080:127.0.0.1:18080 root@<SERVER>
# 然后浏览器打开 http://127.0.0.1:18080
```

如果必须在浏览器直接访问，使用系统原生 IKEv2 连接 VPN，由反向代理终止
TLS 并校验证书；不要使用 Tailscale/WireGuard 等第三方 VPN 客户端。

### 5.5 启用 root helper（M6b 写能力）

默认 `readOnly=true`，Web 仍只能读。需要签发/吊销/重签时，启动独立的 root
helper 容器，再由 root 执行 `readonly off`：

```bash
cd /opt/ipsec
docker compose -f docker-compose.neca.yml --profile web --profile web-write up -d web web-helper
neca web status                 # helper 应显示“可用”
neca web readonly off           # 检查 socket/CA key 后原子写配置
docker kill --signal=HUP neca-web
```

`web-helper` 是 M6b 的**唯一 root 写边界**：挂载 PKI 与 docker.sock，只接受
`issue/revoke/unrevoke/reissue` 白名单，不接受任意路径/shell；Web 容器通过
`/run/neca/web-helper.sock`（0660，root:neca-web）调用它。

helper 的 requestId 幂等缓存默认持久化到 `/run/neca/web-helper-cache.json`
（0600，TTL 24h）；compose 已设置 `NECA_WEB_HELPER_CACHE_FILE`。该文件损坏时
helper 会忽略并从空缓存继续，不会阻塞写操作。

关闭写能力：

```bash
neca web readonly on
docker kill --signal=HUP neca-web
```

### 5.6 反向代理 mTLS（可选）

配置了 `web.adminCNs` 时，必须同时设置 `NECA_WEB_PROXY_SECRET`（写入
`/opt/ipsec/neca.env`，0600），代理转发：

```text
X-Neca-Proxy-Secret: <共享密钥>
X-Neca-Client-CN:   <已验证的管理员证书 CN>
```

necad 只接受来自回环代理的请求；客户端伪造这两个头不会生效。TLS 终止后
在 `/opt/ipsec/neca.env` 设置 `NECA_WEB_SECURE_COOKIES=1` 并重启 web 容器；
OpenSSH 本地转发 HTTP 时保持未设置/0。

### 5.7 状态、只读与重启

```bash
neca web status
neca web readonly status
neca web idle-timeout status
```

修改 `web.readOnly` / `web.idleTimeout` 后发送 SIGHUP，necad 会重新读取配置并原子更新运行时值：

```bash
docker kill --signal=HUP neca-web
```

`web.listen`、`web.adminCNs`、`passkey` 变更仍需要重启 web 容器。

## 6. 备份与回滚

升级前至少备份：

```bash
tar -C /opt/ipsec -czf /root/ipsec-backup-$(date +%Y%m%d-%H%M%S).tar.gz \
  pki clients swanctl neca.json web-secrets
```

回滚：恢复备份，并切回旧镜像标签。停止 Web 不影响 lite：

```bash
docker compose -f docker-compose.neca.yml --profile web stop web
```

lite 与 neca-web 使用同一版本；升级时同时更新两个镜像标签。

## 7. 安全

- 挂载 `docker.sock` 等价于宿主 root 权限，仅 root 可执行/读取 lite 配置；
- Web 容器以 UID/GID 10001 非 root 运行，只读根文件系统、`no-new-privileges`、
  `cap_drop: ALL`，不挂载 `ca.key`、客户端私钥或 docker.sock；
- `web-secrets` 目录 0710，session key/password hash 0640，audit log 0660；
- 不要让 Web 容器拥有整个 `web-secrets` 目录；compose 使用单文件挂载；
- 默认不发布 Web 端口；优先 OpenSSH 本地转发；
- 不要把镜像、env 文件或 `/opt/ipsec` 目录整体上传到公共 registry；
- neca 容器不修改宿主 `/etc/ssl`、`/etc/hosts`、iptables 或 Docker daemon 配置；
  其它容器出现 TLS unknown authority 时，应检查该容器自身的 CA bundle。
