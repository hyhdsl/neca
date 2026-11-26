#!/usr/bin/env bash
# 本脚本恢复 neca 备份并重启 Web 服务。
# neca restore helper. Requires an explicit --yes and an archive created by
# backup-neca.sh. It stops only the explicit web/helper compose services.
set -euo pipefail

if [[ "${1:-}" != "--yes" || -z "${2:-}" ]]; then
  echo "用法: restore-neca.sh --yes <backup.tar.gz>" >&2
  echo "只恢复 /opt/ipsec 下的 pki/clients/swanctl/config/web-secrets。" >&2
  exit 1
fi
if [[ "$(id -u)" -ne 0 ]]; then
  echo "[ERR] 请以 root 执行" >&2
  exit 1
fi
ARCHIVE="$2"
BASE="${NECA_BASE:-/opt/ipsec}"
COMPOSE="${NECA_COMPOSE_FILE:-/opt/ipsec/docker-compose.neca.yml}"
DOCKER="${NECA_DOCKER_BIN:-docker}"

[[ -f "$ARCHIVE" ]] || { echo "[ERR] 备份不存在: $ARCHIVE" >&2; exit 1; }
if [[ -f "${ARCHIVE}.sha256" ]]; then
  (cd "$(dirname "$ARCHIVE")" && sha256sum -c "$(basename "$ARCHIVE").sha256")
fi

echo "[INFO] 停止明确命名的 Web/helper 服务（不触碰 strongSwan/VPN）"
"$DOCKER" compose -f "$COMPOSE" --profile web --profile web-write stop web web-helper || true

echo "[INFO] 恢复 $ARCHIVE 到 $BASE"
tar -C "$BASE" -xzf "$ARCHIVE"

echo "[OK]   文件已恢复；请先以只读模式启动 Web，验证后再开放写能力。"
echo "[INFO] 验证索引/CRL、登录与证书列表后再恢复流量。"
