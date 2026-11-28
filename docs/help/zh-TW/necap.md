# necap 認證元件（v1.7 開發中）

> 本頁適用於 v1.7 necap。目前仍為開發版，尚未發布或部署生產；生產繼續使用 v1.5.0。

necap 是獨立的可選認證服務，負責密碼、TOTP、Passkey、恢復碼與第二因素策略。Web 只負責 UI 與工作階段；認證請求透過 helper 經 Unix socket 轉送。necap 不監聽 TCP，也不掛載 CA、私鑰或 docker.sock。

## 支援流程

- 僅密碼
- 密碼 + TOTP
- 密碼 + Passkey
- 密碼 + 任一已註冊第二因素
- v1.7 不支援 TOTP-only 或 Passkey-only 無密碼登入。

## 前置條件

- 已有可用的 Web 面板與 web-helper。
- 已有 bcrypt 管理員密碼雜湊，預設 /opt/ipsec/web-secrets/password.hash。
- RP ID 與 Origin，例如 neca.example.com 與 https://neca.example.com。
- /opt/ipsec/necap 與 /run/necap 屬主為 UID/GID 10002。

~~~bash
install -d -m 0700 -o 10002 -g 10002 /opt/ipsec/necap
install -d -m 0700 -o 10002 -g 10002 /run/necap
~~~

必要時產生密碼雜湊：

~~~bash
neca web hash-password > /opt/ipsec/web-secrets/password.hash
chmod 600 /opt/ipsec/web-secrets/password.hash
~~~

## 初始化

以 root 執行一次性初始化，完成後檔案會 chown 到 10002：

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile necap run --rm   --user 0:0 --entrypoint /usr/local/bin/necap necap init   --config /opt/ipsec/necap/necap.json   --rp-id neca.example.com   --rp-name "neca Admin"   --origins https://neca.example.com   --uid 10002 --gid 10002
~~~

會建立 necap.json、necap.db 與 master.key，權限均為 0600。master.key 必須另外備份；遺失後已儲存的 TOTP secret 無法解密。

## 啟用

在 /opt/ipsec/neca.env 增加：

~~~text
NECA_NECAP_ENABLED=1
NECA_NECAP_USERNAME=admin
~~~

啟動服務：

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml   --profile web --profile web-write --profile necap   up -d web web-helper necap
~~~

登入時先驗證密碼，再使用 TOTP、Passkey 或恢復碼；第二因素成功後才簽發完整 Web 工作階段。

在「設定 → 安全性」可以重新驗證、註冊/刪除 TOTP、註冊/列出/刪除 Passkey、重新產生恢復碼及修改策略。設定再驗證使用密碼加 TOTP/恢復碼；Passkey 使用者可使用恢復碼。

## CLI

在 root/helper 可用且完成再驗證後：

~~~bash
neca pass status
neca pass totp add
neca pass totp remove
neca pass passkey list
neca pass passkey remove <ID>
neca pass recovery regenerate
neca pass policy <password|totp|passkey|any>
~~~

Passkey 註冊只在 Web 完成；CLI 僅列出與刪除。CLI 寫入操作會要求密碼與現有第二因素/恢復碼。

## 回滾

1. 從 /opt/ipsec/neca.env 移除 NECA_NECAP_ENABLED。
2. 停止 necap 容器。
3. 重新啟動 web 與 web-helper。
4. 保留或備份 /opt/ipsec/necap；不需要修改 neca.json 或密碼雜湊。

回滾後 Web 恢復舊密碼雜湊登入，necap 資料不會自動刪除。

## 邊界

- necap 無 TCP、CA 私鑰、私鑰或 docker.sock。
- necap.db 與 master.key 必須 0600；目錄必須 0700。
- 恢復碼只儲存雜湊，明文只顯示一次。
- 刪除目前策略依賴的唯一第二因素會被拒絕。
- v1.7 不提供無密碼登入與多使用者 Web UI。
- 這是開發版文件；發布前仍需瀏覽器 E2E 與安全審查。
