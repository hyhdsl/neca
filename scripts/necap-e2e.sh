#!/usr/bin/env bash
set -euo pipefail

ROOT=$(mktemp -d /tmp/necap-e2e.XXXXXX)
PORT=18081
BASE="http://127.0.0.1:$PORT"
CFG="$ROOT/neca.json"
NECAP_CFG="$ROOT/necap.json"
NECAP_SOCK="$ROOT/necap.sock"
HELPER_SOCK="$ROOT/helper.sock"
JAR="$ROOT/cookies"
PASS="e2e-only-password"
BIN="${BIN:-/src/dist}"
NECAP_PID=""
HELPER_PID=""
NECAD_PID=""

cleanup() {
  set +e
  if [ -n "$NECAD_PID" ]; then kill "$NECAD_PID" 2>/dev/null; fi
  if [ -n "$HELPER_PID" ]; then kill "$HELPER_PID" 2>/dev/null; fi
  if [ -n "$NECAP_PID" ]; then kill "$NECAP_PID" 2>/dev/null; fi
  wait 2>/dev/null
  echo "--- necap.log ---"
  tail -40 "$ROOT/necap.log" 2>/dev/null || true
  echo "--- helper.log ---"
  tail -40 "$ROOT/helper.log" 2>/dev/null || true
  echo "--- necad.log ---"
  tail -40 "$ROOT/necad.log" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p "$ROOT/pki/ca" "$ROOT/pki/crl" "$ROOT/clients" "$ROOT/meta" "$ROOT/archive" "$ROOT/exports" "$ROOT/downloads" "$ROOT/web-secrets"
printf 'dummy-ca\n' > "$ROOT/pki/ca.crt"
printf '%s\n' "$PASS" | "$BIN/neca-linux-amd64" web hash-password > "$ROOT/web-secrets/password.hash"

cat > "$CFG" <<JSON
{
  "schemaVersion": 2,
  "serverId": "e2e.localhost",
  "paths": {
    "pkiDir": "$ROOT/pki",
    "caCert": "$ROOT/pki/ca.crt",
    "caKey": "$ROOT/pki/ca.key",
    "caDbDir": "$ROOT/pki/ca",
    "indexFile": "$ROOT/pki/ca/index.txt",
    "crlNumFile": "$ROOT/pki/ca/crlnumber",
    "caConf": "$ROOT/pki/ca/openssl.cnf",
    "crlDir": "$ROOT/pki/crl",
    "crlFile": "$ROOT/pki/crl/ca.crl.pem",
    "clientsDir": "$ROOT/clients",
    "metaDir": "$ROOT/meta",
    "archiveDir": "$ROOT/archive",
    "exportDir": "$ROOT/exports",
    "lockFile": "$ROOT/pki/.lock"
  },
  "download": {"dir": "$ROOT/downloads", "ttl": "30m", "maxFiles": 50},
  "web": {"enabled": true, "listen": "127.0.0.1:$PORT", "readOnly": true, "idleTimeout": "30m", "adminCNs": []}
}
JSON

"$BIN/necap-linux-amd64" init   --config "$NECAP_CFG"   --rp-id localhost   --rp-name "neca e2e"   --origins "http://localhost:$PORT"   --database "$ROOT/necap.db"   --master-key-file "$ROOT/master.key"   --socket "$NECAP_SOCK"   --password-hash-file "$ROOT/web-secrets/password.hash" >/dev/null

"$BIN/necap-linux-amd64" serve --config "$NECAP_CFG" > "$ROOT/necap.log" 2>&1 &
NECAP_PID=$!
for i in $(seq 1 40); do
  if [ -S "$NECAP_SOCK" ]; then break; fi
  sleep 0.2
done
test -S "$NECAP_SOCK"

NECA_CONFIG="$CFG" NECAP_SOCKET="$NECAP_SOCK" "$BIN/neca-web-helper-linux-amd64" -config "$CFG" -socket "$HELPER_SOCK" > "$ROOT/helper.log" 2>&1 &
HELPER_PID=$!
for i in $(seq 1 40); do
  if [ -S "$HELPER_SOCK" ]; then break; fi
  sleep 0.2
done
test -S "$HELPER_SOCK"

NECA_CONFIG="$CFG" NECA_WEB_SESSION_KEY="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" NECA_WEB_HELPER_SOCKET="$HELPER_SOCK" NECA_NECAP_ENABLED=1 NECA_NECAP_USERNAME=admin "$BIN/necad-linux-amd64" > "$ROOT/necad.log" 2>&1 &
NECAD_PID=$!
for i in $(seq 1 60); do
  if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.2
done
curl -fsS "$BASE/healthz" >/dev/null

necap_post() {
  curl -fsS --unix-socket "$NECAP_SOCK" -H 'Content-Type: application/json' -X POST "http://necap$1" -d "$2"
}
json_value() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}
json_get() {
  python3 -c 'import json,sys; print(json.load(sys.stdin).get(sys.argv[1],""))' "$1"
}

AUTH=$(necap_post /v1/auth/password "{\"username\":\"admin\",\"password\":\"$PASS\",\"reauth\":true}")
TOKEN=$(printf '%s' "$AUTH" | json_get pendingToken)
USER=$(printf '%s' "$AUTH" | json_get userId)
test -n "$TOKEN"
test -n "$USER"

BEGIN=$(necap_post /v1/factors/totp/begin "{\"token\":\"$TOKEN\",\"userId\":\"$USER\"}")
CHALLENGE=$(printf '%s' "$BEGIN" | json_value challengeId)
URI=$(printf '%s' "$BEGIN" | json_value uri)
SECRET=$(python3 -c 'import sys,urllib.parse; print(urllib.parse.parse_qs(urllib.parse.urlparse(sys.argv[1]).query)["secret"][0])' "$URI")
CODE=$(python3 - "$SECRET" <<'PY'
import base64, hashlib, hmac, struct, sys, time
secret = base64.b32decode(sys.argv[1] + "=" * ((8 - len(sys.argv[1]) % 8) % 8))
counter = int(time.time()) // 30
digest = hmac.new(secret, struct.pack(">Q", counter), hashlib.sha1).digest()
offset = digest[-1] & 15
value = (struct.unpack(">I", digest[offset:offset+4])[0] & 0x7fffffff) % 1000000
print(f"{value:06d}")
PY
)
necap_post /v1/factors/totp/finish "{\"token\":\"$TOKEN\",\"userId\":\"$USER\",\"challengeId\":\"$CHALLENGE\",\"code\":\"$CODE\"}" >/dev/null
necap_post /v1/factors/policy "{\"token\":\"$TOKEN\",\"userId\":\"$USER\",\"mode\":\"totp\"}" >/dev/null

LOGIN_HTML=$(curl -fsS -c "$JAR" -b "$JAR" "$BASE/login")
CSRF=$(printf '%s' "$LOGIN_HTML" | python3 -c 'import re,sys; print(re.search(r"name=\"csrf_token\" value=\"([^\"]+)\"", sys.stdin.read()).group(1))')
LOCATION=$(curl -sS -c "$JAR" -b "$JAR" -D - -o /dev/null -X POST \
  --data-urlencode "csrf_token=$CSRF" \
  --data-urlencode "password=$PASS" \
  "$BASE/login" | tr -d '\r' | awk -F': ' 'tolower($1)=="location"{print $2}')
echo "login_location=$LOCATION"
printf '%s' "$LOCATION" | grep -q '/login/2fa'

MFA_HTML=$(curl -fsS -c "$JAR" -b "$JAR" "$BASE/login/2fa")
CSRF2=$(printf '%s' "$MFA_HTML" | python3 -c 'import re,sys; print(re.search(r"name=\"csrf_token\" value=\"([^\"]+)\"", sys.stdin.read()).group(1))')
CODE2=$(python3 - "$SECRET" <<'PY'
import base64, hashlib, hmac, struct, sys, time
secret = base64.b32decode(sys.argv[1] + "=" * ((8 - len(sys.argv[1]) % 8) % 8))
counter = int(time.time()) // 30
digest = hmac.new(secret, struct.pack(">Q", counter), hashlib.sha1).digest()
offset = digest[-1] & 15
value = (struct.unpack(">I", digest[offset:offset+4])[0] & 0x7fffffff) % 1000000
print(f"{value:06d}")
PY
)
LOCATION2=$(curl -sS -c "$JAR" -b "$JAR" -D - -o /dev/null -X POST \
  --data-urlencode "csrf_token=$CSRF2" \
  --data-urlencode "code=$CODE2" \
  "$BASE/login/totp" | tr -d '\r' | awk -F': ' 'tolower($1)=="location"{print $2}')
echo "mfa_location=$LOCATION2"
printf '%s' "$LOCATION2" | grep -q '^/$'
DASH_STATUS=$(curl -sS -o /dev/null -w '%{http_code}' -b "$JAR" "$BASE/")
echo "dashboard_status=$DASH_STATUS"
test "$DASH_STATUS" = "200"
echo "E2E_RESULT=PASS"
echo "root=$ROOT"