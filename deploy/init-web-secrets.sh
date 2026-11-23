#!/usr/bin/env bash
# init-web-secrets.sh —— 初始化 neca-web 的 web-secrets（root 执行）。
#
# 生成：
#   /opt/ipsec/web-secrets/            root:neca-web 0710
#   /opt/ipsec/web-secrets/session.key root:neca-web 0640
#   /opt/ipsec/web-secrets/password.hash root:neca-web 0640
#   /opt/ipsec/web-secrets/web-audit.log root:neca-web 0660
#
# 不写 neca.json；仅修正 Web 只读所需的 CA 索引权限，不修改证书数据。密码从终端读取，不进入 shell 历史。
# 镜像内 necad 固定 UID/GID 10001；宿主组必须使用同一 GID。
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "[ERR]  必须以 root 执行" >&2
  exit 1
fi

ROOT="${NECA_ROOT:-/opt/ipsec}"
SECRETS="${ROOT}/web-secrets"
GROUP="neca-web"
GID="10001"
IMAGE="${NECA_IMAGE:-ghcr.io/hyhdsl/neca:v1.0.0}"

if getent group "${GROUP}" >/dev/null 2>&1; then
  actual="$(getent group "${GROUP}" | cut -d: -f3)"
  if [[ "${actual}" != "${GID}" ]]; then
    echo "[ERR]  组 ${GROUP} 的 GID=${actual}，与镜像要求的 ${GID} 不一致" >&2
    exit 1
  fi
else
  groupadd --system --gid "${GID}" "${GROUP}"
fi

# Web 只读挂载 CA 索引；setgid 让 CLI 原子重写 index.txt 时新文件继续继承
# neca-web 组，否则 Web 会报 permission denied。
CA_DIR="${ROOT}/pki/ca"
if [[ -d "${CA_DIR}" ]]; then
  chgrp "${GROUP}" "${CA_DIR}"
  chmod 2755 "${CA_DIR}"
  if [[ -e "${CA_DIR}/index.txt" ]]; then
    chgrp "${GROUP}" "${CA_DIR}/index.txt"
    chmod 0640 "${CA_DIR}/index.txt"
  fi
fi

install -d -m 0710 -o root -g "${GROUP}" "${SECRETS}"

if [[ ! -s "${SECRETS}/session.key" ]]; then
  openssl rand -base64 32 > "${SECRETS}/session.key"
fi
chown root:"${GROUP}" "${SECRETS}/session.key"
chmod 0640 "${SECRETS}/session.key"

if [[ ! -s "${SECRETS}/password.hash" ]]; then
  if [[ ! -t 0 ]]; then
    echo "[ERR]  非交互环境无法读取密码；请先交互运行本脚本" >&2
    exit 1
  fi
  printf '管理员密码: ' >&2
  IFS= read -r -s password
  printf '\n' >&2
  if [[ ${#password} -lt 8 ]]; then
    echo "[ERR]  密码至少 8 个字符" >&2
    exit 1
  fi
  tmp="$(mktemp "${SECRETS}/.password.XXXXXX")"
  trap 'rm -f "${tmp}"' EXIT
  printf '%s' "${password}" | docker run --rm -i --entrypoint /usr/local/bin/neca "${IMAGE}" web hash-password > "${tmp}"
  unset password
  chown root:"${GROUP}" "${tmp}"
  chmod 0640 "${tmp}"
  mv "${tmp}" "${SECRETS}/password.hash"
  trap - EXIT
fi
chown root:"${GROUP}" "${SECRETS}/password.hash"
chmod 0640 "${SECRETS}/password.hash"

if [[ ! -e "${SECRETS}/web-audit.log" ]]; then
  install -m 0660 -o root -g "${GROUP}" /dev/null "${SECRETS}/web-audit.log"
fi
chown root:"${GROUP}" "${SECRETS}/web-audit.log"
chmod 0660 "${SECRETS}/web-audit.log"
echo "[OK]   web-secrets 已初始化: ${SECRETS}"
