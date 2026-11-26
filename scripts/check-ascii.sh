#!/usr/bin/env bash
# 本脚本校验 Windows 平台模板为纯 ASCII。
#
# check-ascii.sh —— validate that Windows platform templates are pure ASCII.
#
# Why (development doc section 9, hard rule 1):
#   PowerShell 5.1 reads scripts without a BOM using the system ANSI code page.
#   Any non-ASCII byte (Chinese comments, smart quotes, emoji) becomes mojibake
#   and usually a syntax error on end users' machines. CI therefore enforces
#   ASCII for templates/win, where end users actually execute the files.
#
# Usage:
#   scripts/check-ascii.sh [dir ...]        # defaults to templates/win
#   scripts/check-ascii.sh templates/win
#
# Exit codes: 0 = all clean (or directory absent); 1 = non-ASCII found.
set -euo pipefail

if [ "$#" -eq 0 ]; then
  set -- templates/win
fi

# globstar lets us recurse without relying on find's platform-specific flags.
shopt -s globstar nullglob

failed=0
checked=0

for dir in "$@"; do
  if [ ! -d "$dir" ]; then
    echo "[INFO] directory not present, skipped: $dir"
    continue
  fi

  for file in "$dir"/**/*; do
    # Skip directories; only regular files are validated.
    [ -f "$file" ] || continue
    checked=$((checked + 1))

    # Under LC_ALL=C, [:print:] matches only 0x20-0x7E and [:space:] covers
    # tab/newline, so any byte >= 0x80 (including UTF-8 multibyte) matches the
    # negated class below.
    if LC_ALL=C grep -n '[^[:print:][:space:]]' "$file" >/dev/null 2>&1; then
      echo "[ERR]  non-ASCII content in: $file"
      LC_ALL=C grep -n '[^[:print:][:space:]]' "$file" | head -5 || true
      failed=1
    fi
  done
done

if [ "$failed" -ne 0 ]; then
  echo "[ERR]  found non-ASCII content; Windows templates must stay pure ASCII"
  exit 1
fi

echo "[OK]   ASCII check passed ($checked file(s) checked)"
