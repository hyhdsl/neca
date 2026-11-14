# syntax=docker/dockerfile:1
#
# neca 运行镜像（开发文档 §18 待定事项 2：已定为 debian-slim）。
#
# 为什么不是 scratch/distroless：neca 通过 openssl 子进程完成证书与 p12 操作，
# 运行镜像必须自带 openssl 可执行文件，因此选择约 80MB 的 debian:bookworm-slim。
#
# M0 版本只做最小可用构建：先 make build 产出 dist/neca-linux-amd64，
# 再 build 本镜像。M5 发布时会补充 buildx 多架构、非 root 用户与版本标签。
FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      openssl \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# 二进制由 make build 交叉编译产生（CGO_ENABLED=0，静态链接）。
COPY dist/neca-linux-amd64 /usr/local/bin/neca

ENTRYPOINT ["/usr/local/bin/neca"]
