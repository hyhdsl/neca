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
    NECA_DOWNLOAD_DIR="$side/downloads" \
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
  local args=(-e "s#$OLD#SIDE#g" -e "s#$NEW#SIDE#g" -e "s#$SOLD#SIDE#g" -e "s#$SNEW#SIDE#g")
  if [ -n "${WOLD:-}" ]; then args+=(-e "s#$WOLD#SIDE#g"); fi
  if [ -n "${WNEW:-}" ]; then args+=(-e "s#$WNEW#SIDE#g"); fi
  sed "${args[@]}"
}

# 剩余天数是按运行时刻计算的动态列；跨 UTC 日期时两侧可能相差 1 天。
# 只归一化这一列，其余字段仍严格比较。
normalize_remaining_days() {
  python3 - <<'PY'
import csv
import sys
rows = list(csv.reader(sys.stdin))
for row in rows:
    if len(row) >= 6:
        row[5] = "REMAINING_DAYS"
csv.writer(sys.stdout, lineterminator="\n").writerows(rows)
PY
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

# check_same_norm 先归一化临时路径再比较。
check_same_norm() {
  local label="$1" old_file="$2" new_file="$3"
  normalize <"$old_file" >"$WORK/norm.old"
  normalize <"$new_file" >"$WORK/norm.new"
  check_same "$label" "$WORK/norm.old" "$WORK/norm.new"
}

# 1. list
run_old "$OLD" --list >"$WORK/old.list" 2>"$WORK/old.list.err"
run_new "$NEW" cert list >"$WORK/new.list" 2>"$WORK/new.list.err"
check_same "list stdout" "$WORK/old.list" "$WORK/new.list"
check_same "list stderr" "$WORK/old.list.err" "$WORK/new.list.err"

# 2. verify
run_old "$OLD" --verify >"$WORK/old.verify" 2>"$WORK/old.verify.err"
run_new "$NEW" cert verify >"$WORK/new.verify" 2>"$WORK/new.verify.err"
check_same "verify stdout" "$WORK/old.verify" "$WORK/new.verify"
check_same "verify stderr" "$WORK/old.verify.err" "$WORK/new.verify.err"

# 3. export issued / revoked（比较 CSV 内容；路径列先归一化）
run_old "$OLD" --export-issued --out "$OLD/exports/issued.csv" >"$WORK/old.issued.out" 2>&1
run_new "$NEW" cert export issued --out "$NEW/exports/issued.csv" >"$WORK/new.issued.out" 2>&1
normalize <"$OLD/exports/issued.csv" | normalize_remaining_days >"$WORK/old.issued.csv"
normalize <"$NEW/exports/issued.csv" | normalize_remaining_days >"$WORK/new.issued.csv"
check_same "export issued CSV" "$WORK/old.issued.csv" "$WORK/new.issued.csv"
normalize <"$WORK/old.issued.out" >"$WORK/old.issued.norm"
normalize <"$WORK/new.issued.out" >"$WORK/new.issued.norm"
check_same "export issued 输出" "$WORK/old.issued.norm" "$WORK/new.issued.norm"

run_old "$OLD" --export-revoked --out "$OLD/exports/revoked.csv" >"$WORK/old.revoked.out" 2>&1
run_new "$NEW" cert export revoked --out "$NEW/exports/revoked.csv" >"$WORK/new.revoked.out" 2>&1
normalize <"$OLD/exports/revoked.csv" >"$WORK/old.revoked.csv"
normalize <"$NEW/exports/revoked.csv" >"$WORK/new.revoked.csv"
check_same "export revoked CSV" "$WORK/old.revoked.csv" "$WORK/new.revoked.csv"
normalize <"$WORK/old.revoked.out" >"$WORK/old.revoked.norm"
normalize <"$WORK/new.revoked.out" >"$WORK/new.revoked.norm"
check_same "export revoked 输出" "$WORK/old.revoked.norm" "$WORK/new.revoked.norm"

# 4. index sync：比较输出、index.txt 与 meta 目录
run_old "$SOLD" --sync >"$WORK/old.sync" 2>"$WORK/old.sync.err"
run_new "$SNEW" pki index sync >"$WORK/new.sync" 2>"$WORK/new.sync.err"
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
run_new "$SNEW" cert list >"$WORK/new.sync.list" 2>&1
check_same "sync 后 list" "$WORK/old.sync.list" "$WORK/new.sync.list"

# 6. M2 写命令影子对比：revoke/unrevoke/platform set/delete/platform add
WOLD="$WORK/write-old"
WNEW="$WORK/write-new"
cp -a "$WORK/base" "$WOLD"
cp -a "$WORK/base" "$WNEW"

# 6.1 revoke TMP01
run_old "$WOLD" --revoke TMP01 >"$WORK/old.revoke" 2>"$WORK/old.revoke.err"
run_new "$WNEW" cert revoke TMP01 >"$WORK/new.revoke" 2>"$WORK/new.revoke.err"
check_same_norm "revoke stdout" "$WORK/old.revoke" "$WORK/new.revoke"
check_same_norm "revoke stderr" "$WORK/old.revoke.err" "$WORK/new.revoke.err"
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$WOLD/pki/ca/index.txt" >"$WORK/old.revoke.index"
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$WNEW/pki/ca/index.txt" >"$WORK/new.revoke.index"
check_same "revoke index" "$WORK/old.revoke.index" "$WORK/new.revoke.index"
old_crl_count=$(openssl crl -in "$WOLD/crl/ca.crl.pem" -noout -text 2>/dev/null | grep -c 'Serial Number' || true)
new_crl_count=$(openssl crl -in "$WNEW/crl/ca.crl.pem" -noout -text 2>/dev/null | grep -c 'Serial Number' || true)
if [ "$old_crl_count" = "$new_crl_count" ]; then echo "[OK]   revoke CRL 条目一致"; else echo "[ERR]  revoke CRL 条目: $old_crl_count vs $new_crl_count" >&2; FAILED=1; fi

# 6.2 unrevoke TMP01
run_old "$WOLD" --unrevoke TMP01 >"$WORK/old.unrevoke" 2>"$WORK/old.unrevoke.err"
run_new "$WNEW" cert unrevoke TMP01 >"$WORK/new.unrevoke" 2>"$WORK/new.unrevoke.err"
check_same_norm "unrevoke stdout" "$WORK/old.unrevoke" "$WORK/new.unrevoke"
check_same_norm "unrevoke stderr" "$WORK/old.unrevoke.err" "$WORK/new.unrevoke.err"
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$WOLD/pki/ca/index.txt" >"$WORK/old.unrevoke.index"
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$WNEW/pki/ca/index.txt" >"$WORK/new.unrevoke.index"
check_same "unrevoke index" "$WORK/old.unrevoke.index" "$WORK/new.unrevoke.index"

# 6.3 platform set TMP02
run_old "$WOLD" --set-platforms TMP02 --platforms windows,ios >"$WORK/old.set" 2>&1
run_new "$WNEW" cert platform set TMP02 --platforms windows,ios >"$WORK/new.set" 2>&1
check_same "platform set stdout" "$WORK/old.set" "$WORK/new.set"
check_same "platform set meta" "$WOLD/pki/meta/TMP02.env" "$WNEW/pki/meta/TMP02.env"

# 6.4 platform add TMP03 --platforms ios（只比较输出/meta，mobileconfig UUID 随机）
run_old "$WOLD" --add-platform TMP03 --platforms ios >"$WORK/old.add" 2>&1
run_new "$WNEW" cert platform add TMP03 --platforms ios >"$WORK/new.add" 2>&1
check_same_norm "platform add stdout" "$WORK/old.add" "$WORK/new.add"
check_same "platform add meta" "$WOLD/pki/meta/TMP03.env" "$WNEW/pki/meta/TMP03.env"

# 6.5 delete TMP02（V 状态）
run_old "$WOLD" --delete TMP02 --yes >"$WORK/old.delete" 2>&1
# Old delete and new cert trash intentionally have different wording; compare index/meta only.
run_new "$WNEW" cert trash TMP02 >"$WORK/new.delete" 2>&1
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$WOLD/pki/ca/index.txt" >"$WORK/old.delete.index"
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$WNEW/pki/ca/index.txt" >"$WORK/new.delete.index"
check_same "delete index" "$WORK/old.delete.index" "$WORK/new.delete.index"
if diff -ru "$WOLD/pki/meta" "$WNEW/pki/meta"; then echo "[OK]   delete 后 meta 一致"; else echo "[ERR]  delete 后 meta 差异" >&2; FAILED=1; fi

# 7. M2 issue 流：新旧各自签发 TMP20，归一化序列号/日期/路径后比较
IOLD="$WORK/issue-old"
INEW="$WORK/issue-new"
cp -a "$WORK/base" "$IOLD"
cp -a "$WORK/base" "$INEW"
run_old "$IOLD" --issue TMP20 --platforms windows,android --days 30 >"$WORK/old.issue" 2>"$WORK/old.issue.err"
run_new "$INEW" cert issue TMP20 --platforms windows,android --days 30 --installers windows >"$WORK/new.issue" 2>"$WORK/new.issue.err"
normalize_issue() {
  sed -E -e 's/[0-9A-F]{16,}/SERIAL/g' \
         -e 's/[A-Z][a-z]{2} [ 0-9][0-9] [0-9:]{8} [0-9]{4} GMT/DATE/g' \
         -e "s#$IOLD#SIDE#g" -e "s#$INEW#SIDE#g" | sort
}
normalize_issue <"$WORK/old.issue" >"$WORK/old.issue.norm"
normalize_issue <"$WORK/new.issue" >"$WORK/new.issue.norm"
check_same "issue stdout" "$WORK/old.issue.norm" "$WORK/new.issue.norm"
check_same "issue stderr" "$WORK/old.issue.err" "$WORK/new.issue.err"
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$IOLD/pki/ca/index.txt" | sed -E 's/[0-9A-F]{16,}/SERIAL/g' >"$WORK/old.issue.index"
awk -F'\t' '{print $1"\t"$4"\t"$5"\t"$6}' "$INEW/pki/ca/index.txt" | sed -E 's/[0-9A-F]{16,}/SERIAL/g' >"$WORK/new.issue.index"
check_same "issue index" "$WORK/old.issue.index" "$WORK/new.issue.index"
sed -E 's/^(EXPIRES|SERIAL)=.*/\1=NORM/' "$IOLD/pki/meta/TMP20.env" >"$WORK/old.issue.meta"
sed -E 's/^(EXPIRES|SERIAL)=.*/\1=NORM/' "$INEW/pki/meta/TMP20.env" >"$WORK/new.issue.meta"
check_same "issue meta" "$WORK/old.issue.meta" "$WORK/new.issue.meta"
(cd "$IOLD/clients/TMP20" && find . -type f | sort) >"$WORK/old.issue.tree"
(cd "$INEW/clients/TMP20" && find . -type f | sort) >"$WORK/new.issue.tree"
check_same "issue clients tree" "$WORK/old.issue.tree" "$WORK/new.issue.tree"

if [ "$FAILED" -ne 0 ]; then
  echo "[ERR]  影子对比失败：新旧实现存在差异" >&2
  exit 1
fi
echo "[OK]   影子对比全部通过（diff = 0）"
