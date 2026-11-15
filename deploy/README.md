# 服务器部署（Docker）

本目录提供 neca CLI 容器的生产部署文件。镜像内包含 openssl 与静态 docker CLI；
容器通过 `/var/run/docker.sock` 调用宿主机 docker，对 strongSwan 容器执行
`swanctl --load-creds`。

## 1. 构建与传输

在仓库根目录（已安装 Docker）执行：

```bash
make build
docker build --build-arg TARGETARCH=amd64 -t neca:v0.1.0 .
docker save neca:v0.1.0 | gzip > /tmp/neca-v0.1.0.tar.gz
scp /tmp/neca-0.1.0.tar.gz root@<SERVER>:/tmp/
```

服务器：

```bash
docker load < /tmp/neca-0.1.0.tar.gz
```

## 2. 部署文件

```bash
install -m 0600 deploy/neca.env.example /opt/ipsec/neca.env
# 编辑真实值；必须与 /opt/ipsec 现有路径一致
install -m 0644 deploy/docker-compose.neca.yml /opt/ipsec/docker-compose.neca.yml
chmod 0600 /opt/ipsec/neca.env
```

真实 `SERVER_ID` / `ORG` / `CA_CN` 只写服务器 env 文件，不提交仓库。

## 3. 运行

```bash
cd /opt/ipsec
docker compose -f docker-compose.neca.yml run --rm neca list
docker compose -f docker-compose.neca.yml run --rm neca verify
docker compose -f docker-compose.neca.yml run --rm neca issue TEST01 --dry-run
```

写操作建议先 `--dry-run`，再在维护窗口执行。审计日志在
`NECA_AUDIT_LOG`（默认 `/opt/ipsec/pki/audit.log`）。

## 4. 备份与回滚

升级前至少备份：

```bash
tar -C /opt/ipsec -czf /root/ipsec-backup-$(date +%Y%m%d-%H%M%S).tar.gz pki clients swanctl
```

回滚：恢复备份，并 `docker run --rm -v /opt/ipsec:/opt/ipsec ...` 使用旧镜像
版本；strongSwan 容器本身不需要重建。

## 5. 安全

- 挂载 `docker.sock` 等价于宿主 root 权限，仅 root 可执行/读取配置；
- `/opt/ipsec/pki`、`clients`、`neca.env` 含 CA 私钥或 p12，权限保持 0600/0700；
- 不要把镜像、env 文件或 `/opt/ipsec` 目录整体上传到公共 registry；
- 容器默认 `network_mode: none`，只通过 docker socket 工作。
