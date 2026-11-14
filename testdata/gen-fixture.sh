#!/usr/bin/env bash
# gen-fixture.sh —— 生成 M1 影子测试用的临时 PKI。
#
# 用法：testdata/gen-fixture.sh <目标目录>
#
# 产物布局与生产一致（开发文档 §2）：
#   <dest>/pki/       CA、证书、index.txt、meta、archive
#   <dest>/clients/   每个 CN 一个目录（p12/pass/ca.crt）
#   <dest>/crl/       ca.crl.pem
#   <dest>/exports/   CSV 导出目录
#
# 固定内容：
#   - TMP01：有效，平台 windows,android
#   - TMP02：有效，平台 ios
#   - TMP03：已吊销（index 中为 R，CRL 含该条目）
#   - TMP05：孤立证书（存在 crt/key，但无索引、无 meta、无客户端包），用于 index sync
#   - DEADBEEFDEADBEEF：只在 index 中存在的已吊销记录（证书文件已删除）
#
# 所有路径与密钥都落在临时目录，绝不触碰 /opt/ipsec。
set -euo pipefail

DEST="${1:?用法: gen-fixture.sh <目标目录>}"
ORG="${ORG:-NekoBigLazyer}"
SERVER="${SERVER_ID:-vpn.nekobiglazyer.work}"

rm -rf "$DEST"
mkdir -p "$DEST/pki/ca" "$DEST/pki/meta" "$DEST/pki/archive" \
         "$DEST/clients" "$DEST/exports" "$DEST/crl"

PKI="$DEST/pki"
CADB="$PKI/ca"
INDEX="$CADB/index.txt"
CRLNUM="$CADB/crlnumber"
CONF="$CADB/openssl.cnf"
CA_CRT="$PKI/ca.crt"
CA_KEY="$PKI/ca.key"
CRL="$DEST/crl/ca.crl.pem"

: > "$INDEX"
printf '1000\n' > "$CRLNUM"

# CA
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$CA_KEY" -out "$CA_CRT" \
  -subj "/C=CN/O=$ORG/CN=$ORG VPN CA" -days 3650 2>/dev/null
chmod 600 "$CA_KEY"

cat > "$CONF" <<EOF
[ ca ]
default_ca = CA_default
[ CA_default ]
dir = $CADB
database = $INDEX
crlnumber = $CRLNUM
certificate = $CA_CRT
private_key = $CA_KEY
default_md = sha256
default_crl_days = 3650
crl_extensions = crl_ext
[ crl_ext ]
authorityKeyIdentifier = keyid:always
EOF

# issue_cert CN PLATFORMS [orphan]
#   orphan=1 时只生成 crt/key，不写索引、meta 与客户端包。
issue_cert() {
  local cn="$1" platforms="$2" orphan="${3:-0}"

  openssl genrsa -out "$PKI/$cn.key" 2048 2>/dev/null
  chmod 600 "$PKI/$cn.key"
  openssl req -new -key "$PKI/$cn.key" -subj "/C=CN/O=$ORG/CN=$cn" -out "$PKI/$cn.csr" 2>/dev/null
  cat > "$PKI/$cn.ext" <<EOF
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
subjectAltName=DNS:$SERVER,DNS:$cn
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid,issuer
EOF
  openssl x509 -req -in "$PKI/$cn.csr" -CA "$CA_CRT" -CAkey "$CA_KEY" -CAcreateserial \
    -days 365 -sha256 -extfile "$PKI/$cn.ext" -out "$PKI/$cn.crt" >/dev/null 2>&1

  local serial expiry subject raw_end
  serial=$(openssl x509 -in "$PKI/$cn.crt" -noout -serial | cut -d= -f2 | tr 'a-z' 'A-Z')
  raw_end=$(openssl x509 -in "$PKI/$cn.crt" -noout -enddate | cut -d= -f2)
  expiry=$(date -u -d "$raw_end" +%y%m%d%H%M%SZ)
  subject=$(openssl x509 -in "$PKI/$cn.crt" -noout -subject | sed 's/^subject=//')

  [ "$orphan" = "1" ] && return 0

  printf 'V\t%s\t\t%s\tunknown\t%s\n' "$expiry" "$serial" "$subject" >> "$INDEX"
  printf 'CN=%s\nPLATFORMS=%s\nCREATED=%s\nEXPIRES=%s\nSERIAL=%s\n' \
    "$cn" "$platforms" "$(date +%F)" "$raw_end" "$serial" > "$PKI/meta/$cn.env"

  mkdir -p "$DEST/clients/$cn"
  printf 'TestPass1234\n' > "$DEST/clients/$cn/$cn.p12.pass"
  openssl pkcs12 -export -inkey "$PKI/$cn.key" -in "$PKI/$cn.crt" -name "$cn" \
    -keypbe PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -macalg sha1 \
    -out "$DEST/clients/$cn/$cn.p12" \
    -passout file:"$DEST/clients/$cn/$cn.p12.pass" 2>/dev/null
  chmod 600 "$DEST/clients/$cn/$cn.p12" "$DEST/clients/$cn/$cn.p12.pass"
  cp "$CA_CRT" "$DEST/clients/$cn/ca.crt"
}

issue_cert TMP01 windows,android
issue_cert TMP02 ios
issue_cert TMP03 windows
issue_cert TMP05 windows 1

# 吊销 TMP03：把 index 对应行改为 R 并写入吊销时间。
SERIAL03=$(openssl x509 -in "$PKI/TMP03.crt" -noout -serial | cut -d= -f2 | tr 'a-z' 'A-Z')
REVDATE=$(date -u +%y%m%d%H%M%SZ)
awk -F'\t' -v OFS='\t' -v s="$SERIAL03" -v rv="$REVDATE" \
  '$4==s { $1="R"; $3=rv } { print }' "$INDEX" > "$INDEX.tmp"
mv "$INDEX.tmp" "$INDEX"

# 只存在于索引中的已吊销记录：用于列表"文件已删除"与导出兜底行。
printf 'R\t271114200648Z\t261114200648Z\tDEADBEEFDEADBEEF\tunknown\t/CN=DELETED\n' >> "$INDEX"

# CRL（含 TMP03 与 DEADBEEF 两条吊销记录）。
openssl ca -config "$CONF" -gencrl -out "$CRL" -batch >/dev/null 2>&1

# 清理 CSR/ext，保留 cert/key，模拟真实 PKI 目录。
rm -f "$PKI"/*.csr "$PKI"/*.ext "$PKI"/*.srl
