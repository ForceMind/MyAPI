# My API

將自己的上游帳號、模型與應用程式，接入可自行部署的 AI API 閘道。

[English](README.md) · [简体中文](README.zh_CN.md) · [繁體中文](README.zh_TW.md) · [Français](README.fr.md) · [日本語](README.ja.md)

My API 集中管理渠道、下游 API Key、權限與用量，優先服務個人自用，也可受控地分享給少量使用者。新安裝預設關閉商業模組，不要求先儲值內部錢包；使用者、Key、權限與用量限制仍然保留。

## 功能與邊界

- 接入 OpenAI 相容 API、Responses、Claude Messages、Gemini、Codex 等既有適配器，明確設定開放模型。支援範圍取決於適配器與上游帳號。
- 為每個應用程式或使用者分配獨立下游 Key，限制模型、存取方案與適用用量；上游憑據保留在伺服器。
- 查看請求、錯誤、用量與受支援供應商的額度觀測，區分缺失、失敗及視窗重設。
- 選擇模型、端點與串流模式測試渠道，在適用時重用最近成功選項。
- SQLite、MySQL、PostgreSQL 三選一。介面支援英、簡中、繁中、法、日、俄、越七種語言。

嚴格 Token/USD 預算目前只限已取得資格的官方原生 Responses 純文字路徑；USD 另需適用的凍結價格與服務級別。模型別名、轉換、工具或多模態不會自動取得資格。Codex 百分比是帳戶/視窗剩餘量的安全門檻，不是逐 Key 的共享訂閱消耗帳本。訂閱 API 等價成本僅供參考，不等於實際帳單；未知或估算用量也不等於實際零用量。

## 版本狀態

截至 2026-10-04：

- **`v0.2.0-beta.3` 已預發布**：Linux amd64/arm64 Full 與舊版 LAN 映像；[發布記錄](docs/RELEASE_BETA_3.md)提供來源、digest、簽章與驗證範圍。
- **beta.4 僅已驗證開發原始碼，未發布**：限定的模型發現、映射、預覽、分發和日誌流程不在 beta.3 映像內。
- **beta.5 僅本機開發候選**：帳戶排程、暫時冷卻、有界故障切換與嘗試說明尚未發布，該候選的遠端三資料庫與 Chromium 驗證仍待完成。

預發布不表示已適合正式營運。真實 OAuth、額度重設/429、帳單核對與目標 HTTPS 驗收仍有限或未完成。容器健康不等於上游驗收；取得開發原始碼也不會改變預設 beta.3 映像。統一 Lite/Desktop 安裝更新器尚未交付，桌面建置成功不等於實機驗收。

## 安裝已發布版本

推薦 **Linux Full、固定版本 Docker 映像、SQLite、回環監聽加 HTTPS 反向代理**。

需要 Linux amd64/arm64、Git、Bash、Docker daemon，以及支援 `up --wait --wait-timeout` 的 Compose v2；另需倉庫讀取權限、GitHub/GHCR 連線、持久儲存與自己的 HTTPS Origin。反代指向 `http://127.0.0.1:3000`。下列密鑰生成使用 OpenSSL。映像安裝不需要 Go、Bun、Node.js、Redis 或獨立資料庫。預設 2 CPU / 2 GiB 是容器資源上限，不是測得的最低硬體需求。

### 1. 取得部署檔案

僅用於新目錄的全新安裝；既有實例不得覆寫 `.env`：

```bash
git clone --branch v0.2.0-beta.3 --single-branch https://github.com/ForceMind/MyAPI.git my-api
cd my-api
umask 077
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

### 2. 設定

編輯 `deploy/.env`，將範例改為自己的準確 HTTPS Origin，不含 API 路徑。安裝器會拒絕 `example.com` 佔位值：

```dotenv
MYAPI_IMAGE=ghcr.io/forcemind/myapi:v0.2.0-beta.3
MYAPI_BUILD_LOCAL=false
MYAPI_EDITION=full
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_ALLOW_LAN=false
MYAPI_SESSION_COOKIE_SECURE=true
MYAPI_PUBLIC_URL=https://api.example.com
FULL_CONTENT_LOG_ENABLED=false
```

生成新實例的 `SESSION_SECRET`，用私有編輯器填入結果；至少 48 字元：

```bash
openssl rand -hex 32
```

若需多帳戶額度取樣，再生成獨立密鑰，將 `CHANNEL_QUOTA_IDENTITY_KEYS` 填為 `active:v1:` 加上結果：

```bash
openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'
```

空密鑰環會停用依賴身分的取樣。共用資料庫的實例須共用完整密鑰環，並與資料庫安全備份。升級時保留原秘密，不重新生成、不提交 Git、不貼入回報。`.env` 只接受字面 `KEY=VALUE`，不執行 shell 替換。預設資料與日誌在 `deploy/data/`、`deploy/logs/`，掛載至 `/data`、`/app/logs`。

### 3. 啟動

```bash
bash deploy/install.sh
```

腳本校驗設定、拉取映像、啟動並最多等待 120 秒的健康檢查；不安裝 Docker、申請憑證、設定代理或防火牆。開啟設定的 HTTPS 地址，完成初始化及管理員帳號建立，確認登入與「系統資訊」的版本/revision。Full 使用 Secure Cookie，不建議從 HTTP localhost 登入。

僅本機或私網使用，請參閱[舊版 LAN 指南](docs/LAN_LITE.md)，選用 `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.3`；LAN 分享須明確開啟。

## 第一次呼叫

1. 在「渠道」(`/channels`) 選擇正確供應商、地址與合法憑據。容器 Codex 使用現有網頁登入流程，無法讀取宿主登入檔案。
2. 取得模型或手填準確 ID，確認啟用模型、分組/存取方案與映射。發現不等於授權或成功；增強 beta.4 流程需要對應開發原始碼。
3. 明確選模型、端點及串流模式做一次小型測試；這會聯絡上游，可能消耗額度或產生費用，請用非敏感輸入。
4. 為應用建立最小權限下游 Key，不分享上游憑據或管理員登入令牌。
5. OpenAI 相容客戶端使用 HTTPS Origin 加 `/v1`、下游 Key 和已啟用的公共模型名；其他協定使用其文件端點。先送簡短請求。
6. 到「用量日誌」(`/usage-logs/common`) 查看狀態、模型、用量證據與費用；開發版的管理員路由詳情可核對目標。預覽不保證隨機結果或某個 Key 准入。

失敗先檢查端點、模型、憑據、權限及上游額度，再考慮重試。結果不明時保留待核對狀態。

## 維護與恢復

[部署模板](deploy/.env.example)與[執行環境變數](.env.example)是不同層；任意加入 `.env` 的變數不會自動傳入容器，外部資料庫/Redis 設定須核對 [Compose](deploy/docker-compose.yml)。SQLite 為預設；MySQL ≥ 5.7.8、PostgreSQL ≥ 9.6 是相容基線，應選適當維護版本。Redis 可選，多節點的會話與限流需另行設定。

原始碼 CLI 需要 Node.js ≥ 20。本指南使用已取得倉庫中的 CLI，不依賴從 NPM 安裝套件：

```bash
node cli/myapi.mjs help
node cli/myapi.mjs doctor --project-dir .
node cli/myapi.mjs status --project-dir .
node cli/myapi.mjs logs --project-dir .
```

`doctor` 不等於真實上游驗收，日誌可能含敏感資訊；`install`、`switch`、`rollback` 尚非可用命令。

升級前記錄版本/digest、revision、路徑與設定，備份一致的資料庫（包含適用的 SQLite WAL）、秘密、完整密鑰環和必要日誌。在隔離副本驗證恢復，再演練已發布的目標版本。例如舊實例升級至 beta.3 的只讀預檢：

```bash
node cli/myapi.mjs upgrade --project-dir ./upgrade-copy --version v0.2.0-beta.3 --dry-run --json
```

依[升級與恢復指南](docs/UPGRADE_REHEARSAL.md)完成實際操作。CLI 的 `.env` 備份不是資料庫備份；嘗試目標啟動後不會自動重啟舊映像，資料庫可能已遷移。使用不相容舊程式前須恢復已驗證的升級前備份。勿刪資料卷、清空待核對記錄，或讓舊程式直接開啟新資料庫。

## 安全與文件

保留回環監聽，分享前檢查 HTTPS、可信代理、註冊、角色及 Key 權限。只使用合法授權帳號/API，遵守上游條款與適用法律；對外服務需另行處理合規義務。部署模板預設開啟完整內容日誌，本頁範例已關閉；若啟用，先設定權限、保留期限和備份，脫敏不保證內容沒有隱私。額度取樣會在背景聯絡上游。秘密、OAuth 檔案、資料庫及私有日誌不可放進 Git 或問題回報。

- [發布與驗證](docs/RELEASE_BETA_3.md) · [部署](DEPLOYMENT_CUSTOM.md) · [LAN](docs/LAN_LITE.md)
- [恢復](docs/UPGRADE_REHEARSAL.md) · [安裝驗收](docs/R1_INSTALLATION_CHECK.md)
- [Relay API](docs/openapi/relay.json) · [管理 API](docs/openapi/api.json)
- [額度分析](docs/QUOTA_ANALYTICS.md) · [Claude 組織用量](docs/CLAUDE_USAGE_REPORT.md)
- [認證](docs/authentication.md) · [內容日誌](docs/FULL_CONTENT_LOGGING_CUSTOM.md)
- [問題回報](https://github.com/ForceMind/MyAPI/issues)：附版本/revision、部署方式與脫敏重現步驟

開發者入口：[開發計畫與實作記錄](docs/MYAPI_MASTER_PLAN.md)。

## 授權與致謝

My API 是基於上游開源工作的修改發行版，ForceMind 負責相應發行修改。採用 [GNU AGPLv3](LICENSE)；[NOTICE](NOTICE) 保留上游歸屬及第 7 節附加條件，包括修改版介面中必須可見的前端署名與原專案連結。保留通知並標明修改；品牌變更不免除義務，修改版網路服務亦可能涉及對應原始碼提供要求。

[第三方授權](THIRD-PARTY-LICENSES.md)列出依賴歸屬。分發映像、二進位、前端及桌面包時保留適用通知，包括相關 Electron/Chromium 通知；使用或再分發前閱讀完整條款。
