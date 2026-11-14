#!/usr/bin/env bash
# ============================================================
#  VPN 用户证书管理向导（strongSwan IKEv2 / X.509 / CRL）
#  功能：签发 / 吊销 / 恢复吊销 / 重签发 / 增加平台 / 导出 CSV
#  迁移：以下变量均可用环境变量覆盖
# ============================================================
set -uo pipefail

PKI_DIR="${PKI_DIR:-/opt/ipsec/pki}"
CA_CERT="${CA_CERT:-$PKI_DIR/ca.crt}"
CA_KEY="${CA_KEY:-$PKI_DIR/ca.key}"
CADB_DIR="${CADB_DIR:-$PKI_DIR/ca}"
INDEX_FILE="${INDEX_FILE:-$CADB_DIR/index.txt}"
CRLNUM_FILE="${CRLNUM_FILE:-$CADB_DIR/crlnumber}"
CA_CONF="${CA_CONF:-$CADB_DIR/openssl.cnf}"
CRL_DIR="${CRL_DIR:-/opt/ipsec/swanctl/x509crl}"
CRL_FILE="${CRL_FILE:-$CRL_DIR/ca.crl.pem}"
CLIENTS_DIR="${CLIENTS_DIR:-/opt/ipsec/clients}"
META_DIR="${META_DIR:-$PKI_DIR/meta}"
ARCHIVE_DIR="${ARCHIVE_DIR:-$PKI_DIR/archive}"
EXPORT_DIR="${EXPORT_DIR:-/opt/ipsec/exports}"
CONTAINER="${CONTAINER:-ipsec-vpn}"
DOCKER_BIN="${DOCKER_BIN:-docker}"
SERVER_ID="${SERVER_ID:-vpn.example.com}"
CA_CN="${CA_CN:-ExampleOrg VPN CA}"
DEFAULT_DAYS="${DEFAULT_DAYS:-365}"
KEY_BITS="${KEY_BITS:-3072}"
ORG="${ORG:-ExampleOrg}"

ok(){ printf '[OK]   %s\n' "$*"; }
info(){ printf '[INFO] %s\n' "$*"; }
warn(){ printf '[WARN] %s\n' "$*" >&2; }
die(){ printf '[ERR]  %s\n' "$*" >&2; exit 1; }
need(){ command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"; }
ask(){ local p="$1" d="${2:-}" a; read -r -p "$p" a; printf '%s' "${a:-$d}"; }
confirm(){ local a; read -r -p "$1 [y/N]: " a; case "$a" in y|Y|yes|YES) return 0;; *) return 1;; esac; }

init_env(){
  need openssl; need awk; need sed; need date
  mkdir -p "$CADB_DIR" "$CRL_DIR" "$CLIENTS_DIR" "$META_DIR" "$ARCHIVE_DIR" "$EXPORT_DIR"
  [ -f "$INDEX_FILE" ] || : > "$INDEX_FILE"
  [ -f "$CRLNUM_FILE" ] || echo 1000 > "$CRLNUM_FILE"
  [ -f "$CA_CONF" ] || cat > "$CA_CONF" <<EOF
[ ca ]
default_ca = CA_default
[ CA_default ]
dir = $CADB_DIR
database = $INDEX_FILE
crlnumber = $CRLNUM_FILE
certificate = $CA_CERT
private_key = $CA_KEY
default_md = sha256
default_crl_days = 3650
crl_extensions = crl_ext
[ crl_ext ]
authorityKeyIdentifier = keyid:always
EOF
  [ -f "$CA_CERT" ] || die "找不到 CA 证书: $CA_CERT"
}

cert_serial(){ openssl x509 -in "$1" -noout -serial | cut -d= -f2 | tr 'a-z' 'A-Z'; }
cert_exp_idx(){ local e; e=$(openssl x509 -in "$1" -noout -enddate | cut -d= -f2); date -u -d "$e" +%y%m%d%H%M%SZ; }
cert_subj(){ openssl x509 -in "$1" -noout -subject | sed 's/^subject=//'; }
cert_cn(){ openssl x509 -in "$1" -noout -subject -nameopt RFC2253 | sed 's/^subject=//' | tr ',' '\n' | sed -n 's/^CN=//p' | head -1; }
cert_san(){ openssl x509 -in "$1" -noout -ext subjectAltName 2>/dev/null | tail -n +2 | tr -d ' ' | paste -sd, -; }
cert_fp(){ openssl x509 -in "$1" -noout -fingerprint -sha256 | sed 's/^.*=//'; }
cert_days(){ local e; e=$(date -d "$(openssl x509 -in "$1" -noout -enddate | cut -d= -f2)" +%s 2>/dev/null) || { echo 0; return; }; echo $(( (e - $(date +%s)) / 86400 )); }

index_add(){ printf 'V\t%s\t\t%s\tunknown\t%s\n' "$2" "$1" "$3" >> "$INDEX_FILE"; }
idx_status(){ awk -F'\t' -v s="$1" '$4==s{print $1; exit}' "$INDEX_FILE"; }
idx_set(){ local ser="$1" st="$2" rv="${3:-}" t; t=$(mktemp); awk -F'\t' -v OFS='\t' -v s="$ser" -v st="$st" -v rv="$rv" '{ if ($4==s) { $1=st; $3=(st=="R"?rv:"") } print }' "$INDEX_FILE" > "$t"; mv "$t" "$INDEX_FILE"; }
idx_del(){ local s="$1" t; t=$(mktemp); awk -F'\t' -v x="$s" '$4!=x' "$INDEX_FILE" > "$t"; mv "$t" "$INDEX_FILE"; }
find_cert_by_serial(){
  local s="$1" f
  for f in "$ARCHIVE_DIR"/*.crt "$PKI_DIR"/*.crt; do
    [ -f "$f" ] || continue
    [ "$(cert_serial "$f" 2>/dev/null)" = "$s" ] && { printf '%s' "$f"; return 0; }
  done
  return 1
}

gencrl(){ openssl ca -config "$CA_CONF" -gencrl -out "$CRL_FILE" -batch >/dev/null 2>&1 || die "CRL 生成失败"; chmod 644 "$CRL_FILE"; ok "CRL 已更新: $CRL_FILE"; }
reload_engine(){
  if command -v "$DOCKER_BIN" >/dev/null 2>&1 && "$DOCKER_BIN" ps --format '{{.Names}}' 2>/dev/null | grep -qx "$CONTAINER"; then
    warn "重启 $CONTAINER 以加载新 CRL（当前 VPN 会话会断开）..."
    "$DOCKER_BIN" restart "$CONTAINER" >/dev/null
    local i; for i in $(seq 1 40); do "$DOCKER_BIN" exec "$CONTAINER" swanctl --stats >/dev/null 2>&1 && { ok "strongSwan 已就绪"; return 0; }; sleep 1; done
    warn "等待 strongSwan 启动超时，请手动检查"
  else warn "未找到容器 $CONTAINER，请手动重启 strongSwan"; fi
}

meta_file(){ printf '%s/%s.env' "$META_DIR" "$1"; }
meta_write(){ local f; f=$(meta_file "$1"); { echo "CN=$1"; echo "PLATFORMS=$2"; echo "CREATED=$3"; echo "EXPIRES=$4"; echo "SERIAL=$5"; } > "$f"; }
meta_get(){ local f; f=$(meta_file "$1"); [ -f "$f" ] || return 1; sed -n "s/^$2=//p" "$f" | head -1; }
meta_set(){ local f t; f=$(meta_file "$1"); [ -f "$f" ] || return 1; t=$(mktemp); awk -F'=' -v OFS='=' -v p="$2" '$1=="PLATFORMS"{print $1,p; next} {print}' "$f" > "$t"; mv "$t" "$f"; }
norm_platforms(){
  local s="${1:-}" out="" p
  IFS=',' read -ra arr <<< "$s"
  for p in "${arr[@]}"; do
    p=$(printf '%s' "$p" | tr 'A-Z' 'a-z' | tr -d ' ')
    case "$p" in
      win|windows|w) out="$out,windows";;
      and|android|a|安卓) out="$out,android";;
      ios|apple|i|苹果) out="$out,ios";;
      "") ;;
      *) warn "忽略未知平台: $p";;
    esac
  done
  printf '%s' "${out#,}"
}
merge_platforms(){ local a="${1:-}" b="${2:-}" out="" p; for p in $(printf '%s,%s' "$a" "$b" | tr ',' ' '); do case ",$out," in *",$p,"*) ;; *) out="$out,$p";; esac; done; printf '%s' "${out#,}"; }

gen_pass(){ local p; p=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 12); [ -n "$p" ] || p="Pw$(date +%s | tail -c 7)"; printf '%s' "$p"; }
export_p12(){
  local cn="$1" dir="$CLIENTS_DIR/$1" p12 passf
  mkdir -p "$dir"; p12="$dir/$cn.p12"; passf="$dir/$cn.p12.pass"
  [ -f "$passf" ] || gen_pass > "$passf"
  openssl pkcs12 -export -inkey "$PKI_DIR/$cn.key" -in "$PKI_DIR/$cn.crt" -name "$cn" -keypbe PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -macalg sha1 -out "$p12" -passout file:"$passf" || die "p12 导出失败"
  chmod 600 "$p12" "$passf"
  install -m 0644 "$CA_CERT" "$dir/ca.crt"
  install -m 0644 "$PKI_DIR/$cn.crt" "$dir/$cn.crt"
  verify_p12 "$cn"
}
verify_p12(){
  local cn="$1" p="$CLIENTS_DIR/$1/$1.p12" pf="$CLIENTS_DIR/$1/$1.p12.pass" info cnt plen
  [ -f "$p" ] || { warn "p12 不存在: $cn"; return 1; }
  info=$(openssl pkcs12 -in "$p" -passin file:"$pf" -noout -info 2>&1 | head -8)
  cnt=$(openssl pkcs12 -in "$p" -nokeys -passin file:"$pf" 2>/dev/null | grep -c 'BEGIN CERTIFICATE')
  plen=$(wc -c < "$pf" | tr -d ' ')
  if printf '%s' "$info" | grep -qiE 'PBES2|aes256|aes128'; then warn "$cn: p12 使用 PBES2/AES（MIUI 可能卡在“提取证书”）"; fi
  printf '%s' "$info" | grep -q 'pbeWithSHA1And3-KeyTripleDES-CBC' || warn "$cn: p12 未见 3DES 加密"
  [ "$cnt" = "1" ] || warn "$cn: p12 内含 $cnt 张证书（MIUI 建议仅客户端证书，CA 单独安装）"
  [ "$plen" -ge 8 ] || warn "$cn: p12 密码长度 $plen 过短"
  ok "p12 检查: $cn (3DES ✓, 证书 $cnt 张, 密码 $plen 位)"
}

bundle_all(){ local cn="$1" p; for p in $(printf '%s' "$2" | tr ',' ' '); do bundle_platform "$cn" "$p"; done; }
bundle_platform(){
  local cn="$1" d="$CLIENTS_DIR/$1"
  case "$2" in windows) bundle_windows "$cn" "$d";; android) bundle_android "$cn" "$d";; ios) bundle_ios "$cn" "$d";; *) warn "未知平台: $2";; esac
}
bundle_windows(){
  local cn="$1" w="$2/Windows" prof="ExamplePanel-VPN-$1"
  mkdir -p "$w"
  sed -e "s|__CN__|$cn|g" -e "s|__PROFILE__|$prof|g" -e "s|__SERVER__|$SERVER_ID|g" > "$w/install.ps1" <<'PS1'
# Example VPN (__CN__) - one-click install for Windows 10/11
[CmdletBinding()]
param([string]$ProfileName = '__PROFILE__', [string]$ServerAddress = '__SERVER__', [string]$ClientCertCN = '__CN__', [switch]$NoPause)
$ErrorActionPreference = 'Stop'
$here = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
$p12Path = Join-Path $here '__CN__.p12'; $passPath = Join-Path $here '__CN__.p12.pass'; $caPath = Join-Path $here 'ca.crt'
function Test-Admin { $id = [Security.Principal.WindowsIdentity]::GetCurrent(); (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) }
if (-not (Test-Admin)) {
    Write-Host 'Administrator privileges required - requesting elevation (UAC)...' -ForegroundColor Yellow
    Start-Process -FilePath 'powershell.exe' -Verb RunAs -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File', ('"' + $PSCommandPath + '"'))
    exit
}
try {
    foreach ($f in @($p12Path, $passPath, $caPath)) { if (-not (Test-Path $f)) { throw "Missing file: $f" } }
    $ca = Get-ChildItem Cert:\LocalMachine\Root | Where-Object { $_.Subject -like '*CN=ExampleOrg VPN CA*' } | Select-Object -First 1
    if (-not $ca) { $ca = Import-Certificate -FilePath $caPath -CertStoreLocation Cert:\LocalMachine\Root; Write-Host "Imported CA: $($ca.Thumbprint)" }
    $pw = (Get-Content $passPath -Raw).Trim() | ConvertTo-SecureString -AsPlainText -Force
    Import-PfxCertificate -FilePath $p12Path -CertStoreLocation Cert:\LocalMachine\My -Password $pw | Out-Null
    $cert = Get-ChildItem Cert:\LocalMachine\My | Where-Object { $_.Subject -like "*CN=$ClientCertCN*" } | Sort-Object NotAfter -Descending | Select-Object -First 1
    if (-not $cert) { throw "Client certificate not found (CN=$ClientCertCN)" }
    $old = Get-VpnConnection -Name $ProfileName -AllUserConnection -ErrorAction SilentlyContinue
    if ($old) { if ($old.ConnectionStatus -eq 'Connected') { rasdial "$ProfileName" /disconnect 2>$null | Out-Null }; Remove-VpnConnection -Name $ProfileName -AllUserConnection -Force }
    Add-VpnConnection -Name $ProfileName -ServerAddress $ServerAddress -TunnelType Ikev2 -AuthenticationMethod MachineCertificate -EncryptionLevel Required -SplitTunneling -AllUserConnection -Force
    Write-Host "Installed: $ProfileName ($($cert.Subject))" -ForegroundColor Green
} catch { Write-Host "Install failed: $($_.Exception.Message)" -ForegroundColor Red }
finally { if (-not $NoPause -and $Host.Name -eq 'ConsoleHost') { Read-Host 'Press Enter to close' | Out-Null } }
PS1
  sed -e "s|__CN__|$cn|g" -e "s|__PROFILE__|$prof|g" > "$w/uninstall.ps1" <<'PS1'
# Example VPN (__CN__) - uninstall
[CmdletBinding()]
param([string]$ProfileName = '__PROFILE__', [string]$ClientCertCN = '__CN__', [switch]$RemoveCA, [switch]$NoPause)
function Test-Admin { $id = [Security.Principal.WindowsIdentity]::GetCurrent(); (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) }
if (-not (Test-Admin)) {
    Write-Host 'Administrator privileges required - requesting elevation (UAC)...' -ForegroundColor Yellow
    Start-Process -FilePath 'powershell.exe' -Verb RunAs -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File', ('"' + $PSCommandPath + '"'))
    exit
}
try {
    $c = Get-VpnConnection -Name $ProfileName -AllUserConnection -ErrorAction SilentlyContinue
    if ($c) { if ($c.ConnectionStatus -eq 'Connected') { rasdial "$ProfileName" /disconnect 2>$null | Out-Null }; Remove-VpnConnection -Name $ProfileName -AllUserConnection -Force -ErrorAction SilentlyContinue; Write-Host "Removed profile: $ProfileName" }
    $certs = @(); $certs += Get-ChildItem Cert:\LocalMachine\My -ErrorAction SilentlyContinue | Where-Object { $_.Subject -like "*CN=$ClientCertCN*" }; $certs += Get-ChildItem Cert:\CurrentUser\My -ErrorAction SilentlyContinue | Where-Object { $_.Subject -like "*CN=$ClientCertCN*" }
    foreach ($x in $certs) { Remove-Item -Path $x.PSPath -Force -ErrorAction SilentlyContinue }
    if ($RemoveCA) { Get-ChildItem Cert:\LocalMachine\Root -ErrorAction SilentlyContinue | Where-Object { $_.Subject -like '*CN=ExampleOrg VPN CA*' } | ForEach-Object { Remove-Item -Path $_.PSPath -Force -ErrorAction SilentlyContinue } }
} finally { if (-not $NoPause -and $Host.Name -eq 'ConsoleHost') { Read-Host 'Press Enter to close' | Out-Null } }
PS1
  sed -e "s|__PROFILE__|$prof|g" > "$w/connect.ps1" <<'PS1'
# Example VPN - connect helper (disconnects other VPNs first)
[CmdletBinding()]
param([string]$ProfileName = '__PROFILE__', [switch]$NoPause)
Get-VpnConnection -AllUserConnection -ErrorAction SilentlyContinue | Where-Object { $_.ConnectionStatus -eq 'Connected' -and $_.Name -ne $ProfileName } | ForEach-Object { & rasdial.exe $_.Name /disconnect | Out-Null }
Start-Sleep -Seconds 2
$out = & rasdial.exe $ProfileName 2>&1 | Out-String
Write-Output $out
if (-not $NoPause -and $Host.Name -eq 'ConsoleHost') { Read-Host 'Press Enter to close' | Out-Null }
PS1
  printf '@echo off\nchcp 65001 >nul\npowershell -NoProfile -ExecutionPolicy Bypass -File "%%~dp0install.ps1"\n' > "$w/安装-$cn.cmd"
  printf '@echo off\nchcp 65001 >nul\npowershell -NoProfile -ExecutionPolicy Bypass -File "%%~dp0uninstall.ps1"\n' > "$w/卸载-$cn.cmd"
  printf '@echo off\nchcp 65001 >nul\npowershell -NoProfile -ExecutionPolicy Bypass -File "%%~dp0connect.ps1"\n' > "$w/连接-$cn.cmd"
  cp -f "$CLIENTS_DIR/$cn/$cn.p12" "$CLIENTS_DIR/$cn/$cn.p12.pass" "$CLIENTS_DIR/$cn/ca.crt" "$w/"
  cat > "$w/使用说明.txt" <<EOF
Example VPN ($cn) — Windows 一键安装

1. 双击 安装-$cn.cmd → UAC 点“是”（信任 CA、导入证书、创建配置 $prof）。
2. 连接：双击 连接-$cn.cmd（会先断开其它已连 VPN）。
3. 卸载：双击 卸载-$cn.cmd（默认保留 CA；加 -RemoveCA 可一并删除）。
注意：Windows 同一时间只能连一个 VPN；脚本需与 p12、密码、ca.crt 同目录。
EOF
  ok "Windows 包: $w"
}
bundle_android(){
  local cn="$1" a="$2/Android"; mkdir -p "$a"
  cp -f "$CLIENTS_DIR/$cn/$cn.p12" "$CLIENTS_DIR/$cn/$cn.p12.pass" "$CLIENTS_DIR/$cn/ca.crt" "$a/"
  cat > "$a/使用说明.txt" <<EOF
Example VPN ($cn) — Android 系统原生 IKEv2

1. 安装 CA：设置 → 密码与安全 → 系统安全 → 加密与凭据 → 安装证书 → CA 证书 → ca.crt
2. 安装用户证书：同页 → VPN 和应用用户证书 → $cn.p12 → 输入 $cn.p12.pass 里的密码
   （p12 为 3DES 加密、只含客户端证书，密码为 12 位字母数字；CA 需单独安装，不要指望 p12 里带 CA）
3. 添加 VPN（IKEv2/IPSec RSA）：服务器地址与 IPSec 标识符都填 $SERVER_ID；
   用户证书 $cn；CA 证书 ExampleOrg VPN CA；服务器证书（来自服务器）
4. 连接。失败后开关点不动时重启手机再试。
EOF
  ok "Android 包: $a"
}
bundle_ios(){
  local cn="$1" i="$2/iOS"; mkdir -p "$i"
  command -v python3 >/dev/null 2>&1 || { warn "缺少 python3，跳过 iOS"; return 0; }
  python3 - "$CA_CERT" "$CLIENTS_DIR/$cn/$cn.p12" "$CLIENTS_DIR/$cn/$cn.p12.pass" "$cn" "$SERVER_ID" "$i/$cn.mobileconfig" <<'PY' || { warn "iOS 生成失败"; return 0; }
import base64, plistlib, re, sys, uuid
ca_path, p12_path, pass_path, cn, server, out = sys.argv[1:7]
ca = open(ca_path, 'rb').read(); p12 = open(p12_path, 'rb').read(); pw = open(pass_path).read().strip()
body = re.search(rb'-----BEGIN CERTIFICATE-----(.*?)-----END CERTIFICATE-----', ca, re.S).group(1)
ca_der = base64.b64decode(b''.join(body.split()))
u = lambda: str(uuid.uuid4()).upper()
uid_ca, uid_id, uid_vpn, uid_top = u(), u(), u(), u()
lid = 'work.example.vpn.' + cn.lower()
profile = {'PayloadType': 'Configuration', 'PayloadVersion': 1, 'PayloadIdentifier': lid, 'PayloadUUID': uid_top,
 'PayloadDisplayName': 'Example VPN (' + cn + ')', 'PayloadOrganization': 'ExampleOrg',
 'PayloadDescription': 'CA + client cert + IKEv2', 'PayloadRemovalDisallowed': False,
 'PayloadContent': [
  {'PayloadType': 'com.apple.security.root', 'PayloadVersion': 1, 'PayloadIdentifier': lid + '.ca', 'PayloadUUID': uid_ca,
   'PayloadDisplayName': 'ExampleOrg VPN CA', 'PayloadCertificateFileName': 'ca.crt', 'PayloadContent': ca_der},
  {'PayloadType': 'com.apple.security.pkcs12', 'PayloadVersion': 1, 'PayloadIdentifier': lid + '.id', 'PayloadUUID': uid_id,
   'PayloadDisplayName': cn + ' client cert', 'PayloadCertificateFileName': cn + '.p12', 'Password': pw, 'PayloadContent': p12},
  {'PayloadType': 'com.apple.vpn.managed', 'PayloadVersion': 1, 'PayloadIdentifier': lid + '.vpn', 'PayloadUUID': uid_vpn,
   'PayloadDisplayName': 'Example VPN (' + cn + ')', 'UserDefinedName': 'Example VPN ' + cn, 'VPNType': 'IKEv2',
   'IKEv2': {'AuthenticationMethod': 'Certificate', 'RemoteAddress': server, 'RemoteIdentifier': server, 'LocalIdentifier': cn,
             'PayloadCertificateUUID': uid_id, 'ServerCertificateIssuerCommonName': 'ExampleOrg VPN CA',
             'ServerCertificateCommonName': server, 'DeadPeerDetectionRate': 'Medium'},
   'IPv4': {'OverridePrimary': 0}}]}
open(out, 'wb').write(plistlib.dumps(profile))
PY
  ok "iOS 包: $i"
}

issue_cert(){
  local cn="$1" days="$2" plats="$3" crt="$PKI_DIR/$1.crt" ser
  [ -n "$cn" ] || die "CN 不能为空"
  printf '%s' "$cn" | grep -Eq '^[A-Za-z0-9._-]+$' || die "CN 只允许字母数字 . _ -"
  [ -f "$crt" ] && die "证书已存在: $crt（请用重签发或先删除）"
  info "生成密钥与证书 $cn（$days 天）..."
  openssl genrsa -out "$PKI_DIR/$cn.key" "$KEY_BITS" 2>/dev/null || die "生成私钥失败"
  chmod 600 "$PKI_DIR/$cn.key"
  openssl req -new -key "$PKI_DIR/$cn.key" -subj "/C=CN/O=$ORG/CN=$cn" -out "$PKI_DIR/$cn.csr" || die "生成 CSR 失败"
  cat > "$PKI_DIR/$cn.ext" <<EOF
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
subjectAltName=DNS:$SERVER_ID,DNS:$cn
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid,issuer
EOF
  openssl x509 -req -in "$PKI_DIR/$cn.csr" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial -days "$days" -sha256 -extfile "$PKI_DIR/$cn.ext" -out "$crt" >/dev/null 2>&1 || die "签发失败"
  ser=$(cert_serial "$crt")
  index_add "$ser" "$(cert_exp_idx "$crt")" "$(cert_subj "$crt")"
  meta_write "$cn" "$plats" "$(date +%F)" "$(openssl x509 -in "$crt" -noout -enddate | cut -d= -f2)" "$ser"
  export_p12 "$cn"
  bundle_all "$cn" "$plats"
  ok "已签发 $cn（序列 $ser，平台 $plats）"
  openssl x509 -in "$crt" -noout -subject -dates
}
revoke_cert(){
  local cn="$1" crt="$PKI_DIR/$1.crt" ser
  [ -f "$crt" ] || die "找不到证书: $crt"
  ser=$(cert_serial "$crt")
  [ "$(idx_status "$ser")" = "R" ] && { info "$cn 已处于吊销状态"; return 0; }
  idx_set "$ser" R "$(date -u +%y%m%d%H%M%SZ)"; gencrl; reload_engine
  ok "已吊销 $cn（序列 $ser）"
}
unrevoke_cert(){
  local cn="$1" crt="$PKI_DIR/$1.crt" ser
  [ -f "$crt" ] || die "找不到证书: $crt"
  ser=$(cert_serial "$crt")
  [ "$(idx_status "$ser")" = "V" ] && { info "$cn 未处于吊销状态"; return 0; }
  idx_set "$ser" V; gencrl; reload_engine
  ok "已恢复 $cn（序列 $ser）"
}
reissue_cert(){
  local cn="$1" days="$2" plats="$3" crt="$PKI_DIR/$1.crt" ser
  if [ -f "$crt" ]; then
    ser=$(cert_serial "$crt"); mkdir -p "$ARCHIVE_DIR"
    cp -f "$crt" "$ARCHIVE_DIR/$cn-$ser.crt" 2>/dev/null || true
    [ -f "$PKI_DIR/$cn.key" ] && cp -f "$PKI_DIR/$cn.key" "$ARCHIVE_DIR/$cn-$ser.key" 2>/dev/null || true
    [ "$(idx_status "$ser")" = "V" ] && idx_set "$ser" R "$(date -u +%y%m%d%H%M%SZ)"
    rm -f "$crt" "$PKI_DIR/$cn.key" "$PKI_DIR/$cn.csr" "$PKI_DIR/$cn.ext"
    info "旧证书已归档并吊销（$ser）"
  fi
  issue_cert "$cn" "$days" "$plats"
  gencrl; reload_engine
}
add_platform(){
  local cn="$1" plats="$2" crt="$PKI_DIR/$1.crt" cur
  [ -f "$crt" ] || die "找不到证书: $crt"
  [ -f "$CLIENTS_DIR/$cn/$cn.p12" ] || export_p12 "$cn"
  bundle_all "$cn" "$plats"
  cur=$(meta_get "$cn" PLATFORMS || true)
  if [ -n "$cur" ]; then meta_set "$cn" "$(merge_platforms "$cur" "$plats")"; else meta_write "$cn" "$plats" "$(date +%F)" "$(openssl x509 -in "$crt" -noout -enddate | cut -d= -f2)" "$(cert_serial "$crt")"; fi
  ok "已为 $cn 增加平台: $plats"
}
set_platforms(){ [ -f "$PKI_DIR/$1.crt" ] || die "找不到证书"; meta_write "$1" "$2" "$(date +%F)" "$(openssl x509 -in "$PKI_DIR/$1.crt" -noout -enddate | cut -d= -f2)" "$(cert_serial "$PKI_DIR/$1.crt")"; ok "平台标注已更新: $1 -> $2"; }
delete_cert(){
  local cn="$1" crt="$PKI_DIR/$1.crt" f
  [ -f "$crt" ] && idx_del "$(cert_serial "$crt")"
  for f in "$ARCHIVE_DIR/$cn"-*.crt; do [ -f "$f" ] && idx_del "$(cert_serial "$f")"; done
  rm -f "$PKI_DIR/$cn".crt "$PKI_DIR/$cn".key "$PKI_DIR/$cn".csr "$PKI_DIR/$cn".ext
  rm -rf "$CLIENTS_DIR/$cn" "$META_DIR/$cn.env"; rm -f "$ARCHIVE_DIR/$cn"-*
  gencrl; reload_engine
  ok "已删除 $cn"
}
sync_index(){
  local f ser cn
  for f in "$PKI_DIR"/*.crt; do
    [ -f "$f" ] || continue
    case "$(basename "$f")" in ca.crt|server.crt) continue;; esac
    ser=$(cert_serial "$f") || continue
    [ -z "$(idx_status "$ser")" ] && { index_add "$ser" "$(cert_exp_idx "$f")" "$(cert_subj "$f")"; ok "索引补充: $(basename "$f")"; }
    cn=$(cert_cn "$f")
    [ -f "$(meta_file "$cn")" ] || meta_write "$cn" unknown "$(date +%F)" "$(openssl x509 -in "$f" -noout -enddate | cut -d= -f2)" "$ser"
  done
  ok "索引同步完成"
}
format_table(){
  local tmp; tmp=$(mktemp); cat > "$tmp"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$tmp" <<'PY'
import sys, unicodedata
rows = [l.rstrip('\n').split('\t') for l in open(sys.argv[1], encoding='utf-8', errors='replace') if l.strip('\n')]
def w(s):
    return sum(2 if unicodedata.east_asian_width(c) in 'WF' else 1 for c in s)
if rows:
    cols = max(len(r) for r in rows)
    widths = [max(w(r[i]) if i < len(r) else 0 for r in rows) for i in range(cols)]
    for idx, r in enumerate(rows):
        cells = []
        for i in range(cols):
            cell = r[i] if i < len(r) else ''
            cells.append(cell + ' ' * (widths[i] - w(cell)))
        print('  '.join(cells).rstrip())
        if idx == 0:
            print('  '.join('-' * widths[i] for i in range(cols)))
PY
  else
    if command -v column >/dev/null 2>&1; then column -t -s $'\t' "$tmp"; else cat "$tmp"; fi
  fi
  rm -f "$tmp"
}
list_certs(){
  local f cn ser st pl exp rser
  {
    printf 'CN\t状态\t序列号\t平台\t到期\t备注\n'
    for f in "$PKI_DIR"/*.crt; do
      [ -f "$f" ] || continue
      case "$(basename "$f")" in ca.crt|server.crt) continue;; esac
      cn=$(cert_cn "$f"); ser=$(cert_serial "$f"); st=$(idx_status "$ser")
      if [ -z "$st" ]; then st="未索引"; elif [ "$st" = "V" ]; then st="有效"; else st="已吊销"; fi
      pl=$(meta_get "$cn" PLATFORMS 2>/dev/null); [ -n "$pl" ] || pl="-"
      exp=$(openssl x509 -in "$f" -noout -enddate | cut -d= -f2 | sed 's/ GMT$//')
      printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$cn" "$st" "$ser" "$pl" "$exp" ""
    done
    awk -F'\t' '$1=="R"{print $4}' "$INDEX_FILE" | sort -u | while IFS= read -r rser; do
      [ -n "$rser" ] || continue
      find_cert_by_serial "$rser" >/dev/null 2>&1 && continue
      printf '%s\t%s\t%s\t%s\t%s\t%s\n' "(证书已删除)" "已吊销" "$rser" "-" "-" "文件已删除"
    done
  } | format_table
  info "CRL 吊销条目: $(openssl crl -in "$CRL_FILE" -noout -text 2>/dev/null | grep -c 'Serial Number')"
}
csvq(){ printf '"%s"' "$(printf '%s' "$1" | sed 's/"/""/g')"; }
row_for_cert(){
  local crt="$1" st="$2" cn ser nb na dl san pl fp p12
  cn=$(cert_cn "$crt"); ser=$(cert_serial "$crt")
  nb=$(date -d "$(openssl x509 -in "$crt" -noout -startdate | cut -d= -f2)" +%F 2>/dev/null)
  na=$(date -d "$(openssl x509 -in "$crt" -noout -enddate | cut -d= -f2)" +%F 2>/dev/null)
  dl=$(cert_days "$crt"); san=$(cert_san "$crt"); fp=$(cert_fp "$crt")
  pl=$(meta_get "$cn" PLATFORMS 2>/dev/null || echo unknown)
  p12="$CLIENTS_DIR/$cn/$cn.p12"; [ -f "$p12" ] || p12="(无)"
  printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' "$(csvq "$cn")" "$(csvq "$ser")" "$(csvq "$st")" "$(csvq "$nb")" "$(csvq "$na")" "$(csvq "$dl")" "$(csvq "$san")" "$(csvq "$pl")" "$(csvq "$fp")" "$(csvq "$p12")"
}
export_issued(){
  local out="${1:-}" ser crt
  [ -n "$out" ] || out="$EXPORT_DIR/certs-issued-$(date +%Y%m%d-%H%M%S).csv"
  mkdir -p "$(dirname "$out")"
  { printf '\xEF\xBB\xBF'; echo '"CN","序列号","状态","生效","到期","剩余天数","SAN","平台","SHA256指纹","P12路径"'
    awk -F'\t' '$1=="V"{print $4}' "$INDEX_FILE" | sort -u | while IFS= read -r ser; do
      [ -n "$ser" ] || continue
      crt=$(find_cert_by_serial "$ser") || continue
      case "$(basename "$crt")" in server.crt|ca.crt) continue;; esac
      row_for_cert "$crt" 有效
    done; } > "$out"
  ok "已导出已签发证书: $out"
}
export_revoked(){
  local out="${1:-}" ser crt
  [ -n "$out" ] || out="$EXPORT_DIR/certs-revoked-$(date +%Y%m%d-%H%M%S).csv"
  mkdir -p "$(dirname "$out")"
  { printf '\xEF\xBB\xBF'; echo '"CN","序列号","状态","生效","到期","剩余天数","SAN","平台","SHA256指纹","说明"'
    awk -F'\t' '$1=="R"{print $4}' "$INDEX_FILE" | sort -u | while IFS= read -r ser; do
      [ -n "$ser" ] || continue
      if crt=$(find_cert_by_serial "$ser"); then
        case "$(basename "$crt")" in server.crt|ca.crt) continue;; esac
        row_for_cert "$crt" 已吊销
      else
        printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' "$(csvq "(已删除)")" "$(csvq "$ser")" "$(csvq 已吊销)" "$(csvq '')" "$(csvq '')" "$(csvq '')" "$(csvq '')" "$(csvq '')" "$(csvq '')" "$(csvq 证书文件已删除)"
      fi
    done; } > "$out"
  ok "已导出已吊销证书: $out"
}

select_platforms(){
  printf '选择平台：\n'
  printf '  %2s) %s\n' 1 "Windows" 2 "Android" 3 "iOS" 4 "全部"
  local sel p out=""
  read -r -p "编号（可多选，如 1,2）: " sel
  IFS=',' read -ra arr <<< "$sel"
  for p in "${arr[@]}"; do case "$p" in 1) out="$out,windows";; 2) out="$out,android";; 3) out="$out,ios";; 4) out="windows,android,ios";; *) warn "忽略 $p";; esac; done
  out="${out#,}"; [ -n "$out" ] || die "未选择平台"; printf '%s' "$out"
}

wizard(){
  while true; do
    echo; echo "======== VPN 证书管理 ========"
    printf '  %2s) %s\n' 1 "签发新证书（选择平台）"
    printf '  %2s) %s\n' 2 "吊销证书"
    printf '  %2s) %s\n' 3 "恢复已吊销证书"
    printf '  %2s) %s\n' 4 "重签发旧证书"
    printf '  %2s) %s\n' 5 "为已有证书增加平台包"
    printf '  %2s) %s\n' 6 "导出已签发证书列表 (CSV)"
    printf '  %2s) %s\n' 7 "导出已吊销证书列表 (CSV)"
    printf '  %2s) %s\n' 8 "查看证书列表"
    printf '  %2s) %s\n' 9 "同步索引 / 修复元数据"
    printf '  %2s) %s\n' 10 "删除证书（危险）"
    printf '  %2s) %s\n' 0 "退出"
    read -r -p "请选择: " ch
    case "$ch" in
      1) read -r -p "证书 CN: " cn; [ -n "$cn" ] || continue; pl=$(select_platforms); d=$(ask "有效期天数 [${DEFAULT_DAYS}]: " "$DEFAULT_DAYS"); issue_cert "$cn" "$d" "$pl";;
      2) read -r -p "要吊销的 CN: " cn; [ -n "$cn" ] && revoke_cert "$cn";;
      3) read -r -p "要恢复的 CN: " cn; [ -n "$cn" ] && unrevoke_cert "$cn";;
      4) read -r -p "要重签的 CN: " cn; pl=$(select_platforms); d=$(ask "有效期天数 [${DEFAULT_DAYS}]: " "$DEFAULT_DAYS"); confirm "旧证书将归档并吊销，继续？" && reissue_cert "$cn" "$d" "$pl";;
      5) read -r -p "CN: " cn; pl=$(select_platforms); add_platform "$cn" "$pl";;
      6) export_issued "";;
      7) export_revoked "";;
      8) list_certs;;
      9) sync_index;;
      10) read -r -p "要删除的 CN: " cn; confirm "删除 $cn 并从索引/CRL 移除？" && delete_cert "$cn";;
      0|q|Q) exit 0;;
      *) warn "无效选项";;
    esac
  done
}

ACTION="" CN="" PLATFORMS="windows,android" DAYS="$DEFAULT_DAYS" OUT="" YES=0
while [ $# -gt 0 ]; do
  case "$1" in
    --issue|--revoke|--unrevoke|--reissue|--add-platform|--set-platforms|--delete|--export-issued|--export-revoked|--list|--sync|--verify) ACTION="${1#--}"; shift;;
    --platforms) PLATFORMS="${2:-}"; shift 2;;
    --days) DAYS="${2:-$DEFAULT_DAYS}"; shift 2;;
    --out) OUT="${2:-}"; shift 2;;
    --cn) CN="${2:-}"; shift 2;;
    --yes|-y) YES=1; shift;;
    -h|--help) echo "用法: $0 [--issue CN --platforms windows,android --days 365] [--revoke CN] [--unrevoke CN] [--reissue CN] [--add-platform CN --platforms ios] [--set-platforms CN --platforms ...] [--delete CN --yes] [--export-issued [--out F]] [--export-revoked] [--list] [--sync]"; exit 0;;
    *) [ -z "$CN" ] && CN="$1"; shift;;
  esac
done

init_env
case "$ACTION" in
  "") wizard;;
  issue) [ -n "$CN" ] || die "--issue 需要 CN"; issue_cert "$CN" "$DAYS" "$(norm_platforms "$PLATFORMS")";;
  revoke) [ -n "$CN" ] || die "--revoke 需要 CN"; revoke_cert "$CN";;
  unrevoke) [ -n "$CN" ] || die "--unrevoke 需要 CN"; unrevoke_cert "$CN";;
  reissue) [ -n "$CN" ] || die "--reissue 需要 CN"; reissue_cert "$CN" "$DAYS" "$(norm_platforms "$PLATFORMS")";;
  add-platform) [ -n "$CN" ] || die "--add-platform 需要 CN"; add_platform "$CN" "$(norm_platforms "$PLATFORMS")";;
  set-platforms) [ -n "$CN" ] || die "--set-platforms 需要 CN"; set_platforms "$CN" "$PLATFORMS";;
  delete) [ -n "$CN" ] || die "--delete 需要 CN"; if [ "$YES" = 1 ]; then delete_cert "$CN"; else confirm "确定删除 $CN？" && delete_cert "$CN"; fi;;
  verify) for d in "$CLIENTS_DIR"/*/; do [ -d "$d" ] && verify_p12 "$(basename "$d")"; done;;
  export-issued) export_issued "$OUT";;
  export-revoked) export_revoked "$OUT";;
  list) list_certs;;
  sync) sync_index;;
  *) die "未知操作: $ACTION";;
esac
