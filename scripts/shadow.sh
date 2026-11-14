#!/usr/bin/env bash
# shadow.sh —— 新旧实现影子对比（开发文档 §13.3）。
#
# 对同一份 fixture 的独立副本分别执行旧 bash 脚本与新 neca，
# 逐项比较 list/verify/export/index sync 的输出与产物；任何差异都返回非 0。
#
# 依赖：bash、openssl、python3（旧脚本的表格对齐使用 Python unicodedata）、
#       GNU date（测试镜像 debian 与 GitHub ubuntu 均满足）。
#
# 用法：
#   scripts/shadow.sh                # 使用 dist/neca-linux-amd64
#   NECA_BIN=/path/to/neca scripts/shadow.sh
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
NECA_BIN="${NECA_BIN:-$ROOT/dist/neca-linux-amd64}"
OLD_SCRIPT="$ROOT/legacy/vpn-cert-manager.sh"
FIXTURE_GEN="$ROOT/testdata/gen-fixture.sh"

if [ ! -f "$NECA_BIN" ]; then
  echo "[ERR]  找不到新二进制: $NECA_BIN（先执行 make build）" >&2
  exit 1
fi
for tool in openssl python3 diff; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "[ERR]  缺少依赖: $tool" >&2
    exit 1
  }
done

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

OLD="$WORK/old"
NEW="$WORK/new"
SOLD="$WORK/sync-old"
SNEW="$WORK/sync-new"

bash "$FIXTURE_GEN" "$WORK/base"
cp -a "$WORK/base" "$OLD"
cp -a "$WORK/base" "$NEW"
cp -a "$WORK/base" "$SOLD"
cp -a "$WORK/base" "$SNEW"

# 为某一侧构造完整环境变量；两侧路径不同，其余参数一致。
side_env() {
  local side="$1"
  shift
  env \
    PKI_DIR="$side/pki" \
    CA_CERT="$side/pki/ca.crt" \
    CA_KEY="$side/pki/ca.key" \
    CADB_DIR="$side/pki/ca" \
    INDEX_FILE="$side/pki/ca/index.txt" \
    CRLNUM_FILE="$side/pki/ca/crlnumber" \
    CA_CONF="$side/pki/ca/openssl.cnf" \
    CRL_DIR="$side/crl" \
    CRL_FILE="$side/crl/ca.crl.pem" \
    CLIENTS_DIR="$side/clients" \
    META_DIR="$side/pki/meta" \
    ARCHIVE_DIR="$side/pki/archive" \
    EXPORT_DIR="$side/exports" \
    CONTAINER="neca-shadow-not-running" \
    DOCKER_BIN="false" \
    SERVER_ID="vpn.example.com" \
    CA_CN="ExampleOrg VPN CA" \
    ORG="ExampleOrg" \
    DEFAULT_DAYS="365" \
    KEY_BITS="2048" \
    NECA_RELOAD_MODE="creds" \
    NECA_LOCK_FILE="$side/pki/.lock" \
    "$@"
}

run_old() {
  local side="$1"
  shift
  side_env "$side" bash "$OLD_SCRIPT" "$@"
}

run_new() {
  local side="$1"
  shift
  side_env "$side" "$NECA_BIN" "$@"
}

# normalize 把两侧的临时路径统一成 SIDE，便于比较含路径的输出。
normalize() {
  sed -e "s#$OLD#SIDE#g" -e "s#$NEW#SIDE#g" -e "s#$SOLD#SIDE#g" -e "s#$SNEW#SIDE#g"
}

FAILED=0
check_same() {
  local label="$1" old_file="$2" new_file="$3"
  if diff -u "$old_file" "$new_file"; then
    echo "[OK]   $label 一致"
  else
    echo "[ERR]  $label 存在差异（见上方 diff）" >&2
    FAILED=1
  fi
}

# 1. list
run_old "$OLD" --list >"$WORK/old.list" 2>"$WORK/old.list.err"
run_new "$NEW" list >"$WORK/new.list" 2>"$WORK/new.list.err"
check_same "list stdout" "$WORK/old.list" "$WORK/new.list"
check_same "list stderr" "$WORK/old.list.err" "$WORK/new.list.err"

# 2. verify
run_old "$OLD" --verify >"$WORK/old.verify" 2>"$WORK/old.verify.err"
run_new "$NEW" verify >"$WORK/new.verify" 2>"$WORK/new.verify.err"
check_same "verify stdout" "$WORK/old.verify" "$WORK/new.verify"
check_same "verify stderr" "$WORK/old.verify.err" "$WORK/new.verify.err"

# 3. export issued / revoked（比较 CSV 内容；路径列先归一化）
run_old "$OLD" --export-issued --out "$OLD/exports/issued.csv" >"$WORK/old.issued.out" 2>&1
run_new "$NEW" export issued --out "$NEW/exports/issued.csv" >"$WORK/new.issued.out" 2>&1
normalize <"$OLD/exports/issued.csv" >"$WORK/old.issued.csv"
normalize <"$NEW/exports/issued.csv" >"$WORK/new.issued.csv"
check_same "export issued CSV" "$WORK/old.issued.csv" "$WORK/new.issued.csv"
normalize <"$WORK/old.issued.out" >"$WORK/old.issued.norm"
normalize <"$WORK/new.issued.out" >"$WORK/new.issued.norm"
check_same "export issued 输出" "$WORK/old.issued.norm" "$WORK/new.issued.norm"

run_old "$OLD" --export-revoked --out "$OLD/exports/revoked.csv" >"$WORK/old.revoked.out" 2>&1
run_new "$NEW" export revoked --out "$NEW/exports/revoked.csv" >"$WORK/new.revoked.out" 2>&1
normalize <"$OLD/exports/revoked.csv" >"$WORK/old.revoked.csv"
normalize <"$NEW/exports/revoked.csv" >"$WORK/new.revoked.csv"
check_same "export revoked CSV" "$WORK/old.revoked.csv" "$WORK/new.revoked.csv"
normalize <"$WORK/old.revoked.out" >"$WORK/old.revoked.norm"
normalize <"$WORK/new.revoked.out" >"$WORK/new.revoked.norm"
check_same "export revoked 输出" "$WORK/old.revoked.norm" "$WORK/new.revoked.norm"

# 4. index sync：比较输出、index.txt 与 meta 目录
run_old "$SOLD" --sync >"$WORK/old.sync" 2>"$WORK/old.sync.err"
run_new "$SNEW" index sync >"$WORK/new.sync" 2>"$WORK/new.sync.err"
check_same "index sync stdout" "$WORK/old.sync" "$WORK/new.sync"
check_same "index sync stderr" "$WORK/old.sync.err" "$WORK/new.sync.err"
check_same "index.txt" "$SOLD/pki/ca/index.txt" "$SNEW/pki/ca/index.txt"
if diff -ru "$SOLD/pki/meta" "$SNEW/pki/meta"; then
  echo "[OK]   meta 目录一致"
else
  echo "[ERR]  meta 目录存在差异" >&2
  FAILED=1
fi

# 5. 同步后再跑一次 list，确认补索引后的列表同样一致
run_old "$SOLD" --list >"$WORK/old.sync.list" 2>&1
run_new "$SNEW" list >"$WORK/new.sync.list" 2>&1
check_same "sync 后 list" "$WORK/old.sync.list" "$WORK/new.sync.list"

if [ "$FAILED" -ne 0 ]; then
  echo "[ERR]  影子对比失败：新旧实现存在差异" >&2
  exit 1
fi
echo "[OK]   影子对比全部通过（diff = 0）"
