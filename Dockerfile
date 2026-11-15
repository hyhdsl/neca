# syntax=docker/dockerfile:1
#
# neca 运行镜像（开发文档 §9/§15）。
#
# 发布两个镜像：
#   --target lite  -> neca:vX（默认，CLI + Web 配置管理，不含 necad）
#   --target web   -> neca-web:vX（necad + helper 占位，不包含 lite CLI）
#
# 两个 target 共用 base 层（openssl、ca-certificates、docker CLI），
# lite 是默认 target，因此普通的 docker build . 不会意外发布 Web 组件。
FROM docker:27-cli AS dockercli

FROM debian:bookworm-slim AS base

ARG TARGETARCH=amd64
LABEL org.opencontainers.image.title="neca" \
      org.opencontainers.image.description="strongSwan IKEv2 client certificate manager" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.source="https://github.com/hyhdsl/neca"

RUN apt-get update \
 && apt-get install -y --no-install-recommends openssl ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# 只复制静态 docker CLI；helper（M6b）需要它，web 容器不挂载 docker.sock。
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker

WORKDIR /opt/ipsec

# Web 面板包：非 root necad；helper 在 M6a 只编译占位，不开放 socket。
FROM base AS web
RUN groupadd --system --gid 10001 neca-web \
 && useradd --system --uid 10001 --gid neca-web --no-create-home --shell /usr/sbin/nologin neca-web \
 && mkdir -p /opt/ipsec/pki/ca /opt/ipsec/pki/meta /opt/ipsec/clients /opt/ipsec/downloads /opt/ipsec/swanctl/x509crl /opt/ipsec/web-secrets
COPY dist/necad-linux-${TARGETARCH} /usr/local/bin/necad
COPY dist/neca-web-helper-linux-${TARGETARCH} /usr/local/bin/neca-web-helper
RUN chmod 0755 /usr/local/bin/necad /usr/local/bin/neca-web-helper \
 && chown root:root /usr/local/bin/necad /usr/local/bin/neca-web-helper
USER 10001:10001
HEALTHCHECK --interval=30s --timeout=4s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/necad", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/necad"]

# 默认 lite：只含 CLI 与 Web 配置管理命令。
FROM base AS lite
COPY dist/neca-linux-${TARGETARCH} /usr/local/bin/neca
RUN chmod 0755 /usr/local/bin/neca
ENTRYPOINT ["/usr/local/bin/neca"]
CMD ["help"]
