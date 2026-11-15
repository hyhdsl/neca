# syntax=docker/dockerfile:1
#
# neca 运行镜像（开发文档 §9/§15：debian-slim + openssl + docker CLI）。
#
# 为什么不是 scratch/distroless：neca 通过 openssl 子进程完成证书与 p12 操作，
# 运行镜像必须自带 openssl。镜像内的 docker CLI 只用于通过挂载的
# /var/run/docker.sock 执行 swanctl --load-creds；不包含 docker daemon。
#
# 构建前先 make build 生成 dist/neca-linux-amd64 与 dist/neca-linux-arm64。
FROM docker:27-cli AS dockercli

FROM debian:bookworm-slim

ARG TARGETARCH=amd64
LABEL org.opencontainers.image.title="neca" \
      org.opencontainers.image.description="strongSwan IKEv2 client certificate manager" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.source="https://github.com/hyhdsl/neca"

RUN apt-get update \
 && apt-get install -y --no-install-recommends openssl ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# 只复制静态 docker CLI，不把 daemon/containerd 装进镜像。
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
COPY dist/neca-linux-${TARGETARCH} /usr/local/bin/neca
RUN chmod 0755 /usr/local/bin/neca /usr/local/bin/docker

WORKDIR /opt/ipsec
ENTRYPOINT ["/usr/local/bin/neca"]
CMD ["help"]
