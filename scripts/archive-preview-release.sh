#!/usr/bin/env bash
# archive-preview-release.sh —— 把旧预览包转为 source-only 记录。
#
# 用法：
#   scripts/archive-preview-release.sh <tag> [--yes]
#
# 仅处理 prerelease；删除该 release 的全部二进制/容器资产，保留 GitHub
# 自动生成的 Source code (zip/tar.gz) 下载。默认只列出将删除的资产。
set -euo pipefail

tag="${1:-}"
confirm="${2:-}"
if [[ -z "${tag}" ]]; then
  echo "用法: $0 <tag> [--yes]" >&2
  exit 1
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "[ERR] 需要 gh CLI" >&2
  exit 1
fi
if [[ "$(gh release view "${tag}" --json isPrerelease --jq .isPrerelease 2>/dev/null)" != "true" ]]; then
  echo "[ERR] ${tag} 不是 prerelease，拒绝处理" >&2
  exit 1
fi

assets="$(gh release view "${tag}" --json assets --jq '.assets[].name')"
if [[ -z "${assets}" ]]; then
  echo "[OK]   ${tag} 已经是 source-only（没有 release assets）"
  exit 0
fi
echo "将删除 ${tag} 的以下资产，仅保留源代码归档："
printf '  - %s\n' ${assets}
if [[ "${confirm}" != "--yes" ]]; then
  echo "确认后重新执行：$0 ${tag} --yes"
  exit 0
fi
while IFS= read -r asset; do
  [[ -z "${asset}" ]] && continue
  gh release delete-asset "${tag}" "${asset}" --yes
done <<< "${assets}"
echo "[OK]   ${tag} 已转为 source-only"
