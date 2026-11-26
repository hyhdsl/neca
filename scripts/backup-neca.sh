#!/usr/bin/env bash
# 本脚本备份 neca 运行状态和密钥材料。
# neca backup helper. Back up the state needed for rollback.
# Does not stop services and does not print secrets.
set -euo pipefail

BASE="${NECA_BASE:-/opt/ipsec}"
OUT="${1:-/root/neca-backup-$(date -u +%Y%m%dT%H%M%SZ).tar.gz}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "[ERR] 请以 root 执行" >&2
  exit 1
fi
if [[ ! -d "$BASE" ]]; then
  echo "[ERR] 目录不存在: $BASE" >&2
  exit 1
fi
for name in pki clients swanctl config web-secrets; do
  [[ -e "$BASE/$name" ]] || echo "[WARN] 跳过不存在的目录: $BASE/$name" >&2
done

mkdir -p "$(dirname "$OUT")"
umask 077
tar -C "$BASE" -czf "$OUT" pki clients swanctl config web-secrets
chmod 0600 "$OUT"
sha256sum "$OUT" > "${OUT}.sha256"
chmod 0600 "${OUT}.sha256"
echo "[OK]   备份: $OUT"
echo "[OK]   校验: ${OUT}.sha256"
echo "[INFO] 备份含 CA 私钥与会话密钥，请加密保存并限制访问。"
