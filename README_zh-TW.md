# Neko's Efficient Certificate Administrator

**README 語言 / Language：** [繁體中文（臺灣）](README_zh-TW.md) · [簡體中文](README.md) · [English](README_EN.md)

> [!NOTE]
> 本文件為 **README.md 的繁體中文（臺灣）譯本**。內容以簡體中文主版本 [README.md](README.md) 為準；其他語言版本另行安排時間統一更新，可能落後於主版本。英文翻譯：[README_EN.md](README_EN.md)。

> [!WARNING]
> **AI 揭露（使用前必讀）**：本專案的程式碼、除錯、測試、CI／建置／部署指令碼、說明文件與說明網站內容主要由 AI 程式設計助理完成；人類維護者負責需求、審查、功能驗收與正式環境操作。完整邊界見 [AI-DISCLOSURE.md](AI-DISCLOSURE.md)。
>
> **責任限制**：軟體依「現狀」提供，不提供明示或默示擔保；在適用法律允許的範圍內，作者與 AI 工具提供方不對使用或誤用造成的損失負責。
>
> **安全邊界**：安全性測試尚不完整，不建議用於商業用途或高風險正式環境；如要使用，請自行完成獨立安全審查、備份與回復驗證。
>
> **貢獻與回饋**：請先透過 Issue 說明問題或需求；提交 PR 前請閱讀 [開發政策](docs/SECONDARY-DEVELOPMENT.md)。二次開發須遵守 AGPL。
>
> 相關檔案：[LICENSE](LICENSE) · [NOTICE](NOTICE) · [AI-DISCLOSURE.md](AI-DISCLOSURE.md) · [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)

**Neko's Efficient Certificate Administrator（neca，亦寫作 NECA）** 是面向 strongSwan IKEv2 的輕量憑證管理工具。一個 Go CLI、一個選用且預設唯讀的 Web 面板，以及一條受控的 root helper 寫入邊界。

🌐 [首頁](https://neca.nekobiglazyer.work) · 🇨🇳 [簡體中文文件](https://neca.nekobiglazyer.work/zh/) · 🇬🇧 [English](https://neca.nekobiglazyer.work/en/) · 📦 [v1.2.0](https://github.com/hyhdsl/neca/releases/tag/v1.2.0)

## ✨ 特性

- 🔐 **安全預設**：Web 預設唯讀；Web 容器非 root、沒有 <code>ca.key</code>、沒有 <code>docker.sock</code>。
- 🧾 **完整生命週期**：issue / reissue / revoke / unrevoke / delete，自動維護 CRL。
- 📦 **平台套件**：Windows / Linux / macOS / Android / iOS；選用安裝指令碼、p12 profile 與 zip 下載。
- 🐳 **部署簡單**：單一靜態二進位檔；lite 與 neca-web 雙映像，支援 amd64 / arm64。
- ⏳ **到期概覽**：CLI 與 Web 提供 30/60/90 天唯讀到期檢視，不修改憑證或設定。
- ⚙️ **可維運**：JSON 設定、<code>--json</code> / <code>--dry-run</code>、稽核日誌、SIGHUP 熱重載與 requestId 冪等快取。
- ⌨️ **Shell completion**：bash / zsh / fish / PowerShell 自動補齊指令、子命令與主要旗標。
- 🔗 **憑證鏈校驗**：<code>neca cert verify</code> 使用 CA 校驗憑證鏈；鏈失敗只告警，不改變結束代碼約定。
- 🧩 **可擴充**：<code>internal/core/ports</code> 提供索引儲存與設定來源介面。

## 🚀 快速開始

固定版本標籤，先 dry-run，再簽發：

~~~bash
docker pull ghcr.io/hyhdsl/neca:v1.2.0
neca version
neca cert list
neca cert issue alice --platforms windows,android --zip --dry-run
neca cert issue alice --platforms windows,android --zip
~~~

部署時把 <code>/opt/ipsec</code> 掛載給容器，並透過 wrapper 或 compose 執行；完整步驟見 [安裝、升級與復原](https://neca.nekobiglazyer.work/zh-TW/#/install)。

## 🖥️ Web 面板

預設唯讀；需要寫入操作時，明確啟動 root helper 並關閉唯讀：

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web up -d web
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web --profile web-write up -d web web-helper
neca web readonly off
~~~

部署設定位於 <code>/opt/ipsec/config/neca.json</code>；Web 透過設定目錄掛載感知 SIGHUP 熱重載。預設仍建議使用 OpenSSH 本機轉送，不直接暴露公網。

## 📚 說明文件

| 入口 | 內容 |
|---|---|
| 🌐 [首頁](https://neca.nekobiglazyer.work) | 專案介紹與快速入口 |
| 🇨🇳 [簡體中文說明](https://neca.nekobiglazyer.work/zh/) | 安裝、憑證、Web、API、安全、FAQ |
| ⌨️ [常用指令速查](https://neca.nekobiglazyer.work/zh-TW/#/cheatsheet) | 複製即可使用的常用指令 |
| ⌨️ [子命令參考](https://neca.nekobiglazyer.work/zh-TW/#/commands) | 全域旗標、全部子命令、參數、確認與結束代碼 |
| 🇬🇧 [English](https://neca.nekobiglazyer.work/en/) | English core pages |
| 🧪 [Web API](https://neca.nekobiglazyer.work/zh-TW/#/api) | 實驗性介面說明 |
| 🐳 [安裝、升級與回復](https://neca.nekobiglazyer.work/zh-TW/#/install) | Docker／二進位安裝、升級、回復 |
| 🛠️ [開發、授權條款與回饋](https://neca.nekobiglazyer.work/zh-TW/#/development) | 本機建置、架構、發布流程、貢獻 |
| 🤖 [AI 揭露](AI-DISCLOSURE.md) | AI 參與範圍與驗收邊界 |

## 🛠️ 開發

~~~bash
docker compose run -T --rm dev make all      # 格式 + vet + 文件 + 測試 + 交叉編譯
docker compose run -T --rm dev make shadow   # 新舊實作影子比對
docker compose run -T --rm dev make test     # go test -race ./...
~~~

常用檢查：<code>make check-docs</code>、<code>make check-locales</code>、<code>make check-ascii</code>。詳細目標與架構說明見 [說明網站開發頁](https://neca.nekobiglazyer.work/zh-TW/#/development)。

## 📌 狀態

- **目前穩定版**：v1.2.0（2026-11-27 發布）
- **已完成**：M0–M12；v1.2.0（CLI shell completion、憑證鏈驗證、文件一致性）
- **後續**：v1.3.0 憑證匯入 preview；v1.4.0 cert+key 匯入；v1.5.0 設定 schema 與 Passkey 評估
- **邊界**：Passkey 僅有設定與介面預留，未註冊登入路由；Web API 為實驗性介面

## ⚖️ 授權條款與致謝

**AGPL-3.0-only**：允許商業使用，但散布或網路服務須依 AGPL 公開對應原始碼；必須保留原作者、儲存庫連結與授權條款。詳見 [LICENSE](LICENSE) 與 [二次開發政策](docs/SECONDARY-DEVELOPMENT.md)。

本專案建立在大量上游工作之上，特別感謝：

- **Go** 標準函式庫與工具鏈，以及 `golang.org/x` 系列相依套件；
- **strongSwan** 與 IKEv2 社群，為憑證、CRL 與 VPN 設定提供實務基礎；
- **OpenSSL**，負責憑證、金鑰與 PKCS#12 的核心操作；
- **Docker / Buildx / QEMU**，支援多架構映像與可重現的開發環境；
- **GitHub Actions**，提供 CI、發布與映像流水線；
- **Docsify** 與說明網站相關工具，以及所有提交 Issue、測試與建議的使用者。

上游專案的授權條款與著作權歸其各自作者所有；本專案僅保留必要的引用與授權聲明。
