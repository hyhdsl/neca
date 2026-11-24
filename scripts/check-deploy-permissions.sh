#!/usr/bin/env bash
# Verify the documented neca deployment modes on a Linux host.
# Run as root on the host that contains NECA_ROOT (default /opt/ipsec).
set -u
ROOT="${NECA_ROOT:-/opt/ipsec}"
fail=0

check() {
  local path="$1" want="$2" label="$3"
  if [[ ! -e "${path}" ]]; then
    echo "[ERR] missing: ${label}" >&2
    fail=1
    return
  fi
  local got
  got="$(stat -c '%a %U:%G' "${path}" 2>/dev/null || true)"
  if [[ "${got}" != "${want}" ]]; then
    echo "[ERR] ${label}: got '${got}', want '${want}'" >&2
    fail=1
  fi
}

check "${ROOT}/pki/ca.key" "600 root:root" "pki/ca.key"
check "${ROOT}/pki/ca" "2755 root:neca-web" "pki/ca"
check "${ROOT}/pki/ca/index.txt" "640 root:neca-web" "pki/ca/index.txt"
check "${ROOT}/pki/meta" "2750 root:neca-web" "pki/meta"
check "${ROOT}/clients" "2750 root:neca-web" "clients"
check "${ROOT}/downloads" "2750 root:neca-web" "downloads"
check "${ROOT}/config" "750 root:neca-web" "config"
check "${ROOT}/config/neca.json" "640 root:neca-web" "config/neca.json"
check "${ROOT}/web-secrets" "710 root:neca-web" "web-secrets"
check "${ROOT}/web-secrets/session.key" "640 root:neca-web" "session.key"
check "${ROOT}/web-secrets/password.hash" "640 root:neca-web" "password.hash"
check "${ROOT}/web-secrets/web-audit.log" "660 root:neca-web" "web-audit.log"
check "${ROOT}/exports" "700 root:root" "exports"
check "${ROOT}/neca.env" "600 root:root" "neca.env"
check "${ROOT}/.env" "600 root:root" ".env"

# Metadata and exported CSV are sensitive; check every present file.
while IFS= read -r -d '' path; do
  check "${path}" "640 root:neca-web" "pki/meta/*.env"
done < <(find "${ROOT}/pki/meta" -maxdepth 1 -type f -name '*.env' -print0 2>/dev/null)
while IFS= read -r -d '' path; do
  check "${path}" "600 root:root" "exports/*.csv"
done < <(find "${ROOT}/exports" -maxdepth 1 -type f -name '*.csv' -print0 2>/dev/null)

# A world-readable private key, p12, metadata file or export is a failure.
bad="$(find "${ROOT}/clients" "${ROOT}/pki" "${ROOT}/exports" -xdev -type f \
  \( -iname '*.key' -o -iname '*.p12' -o -iname '*.env' -o -iname '*.csv' \) \
  -perm -o+r -print -quit 2>/dev/null || true)"
if [[ -n "${bad}" ]]; then
  echo "[ERR] world-readable sensitive file: ${bad}" >&2
  fail=1
fi

if [[ "${fail}" -ne 0 ]]; then
  echo "[ERR] deployment permission check failed" >&2
  exit 1
fi
echo "[OK]   deployment permissions verified"
