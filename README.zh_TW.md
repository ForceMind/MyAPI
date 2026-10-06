<p align="center">
  <img src="web/public/myapi-logo-v1.png" alt="MyAPI logo" width="144" />
</p>

# MyAPI

統一管理模型服務、應用程式存取、權限與用量的自建 AI API 閘道。

[English](README.md) · [简体中文](README.zh_CN.md) · [繁體中文](README.zh_TW.md) · [Français](README.fr.md) · [日本語](README.ja.md)

MyAPI 集中管理渠道、應用程式 API Key、權限與用量，優先服務個人自用，也可受控地分享給少量使用者。新安裝預設關閉商業模組，不要求先儲值內部錢包；使用者、Key、權限與用量限制仍然保留。

## 技術棧

![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)
![Gin](https://img.shields.io/badge/Gin-008ECF?style=flat-square)
![GORM](https://img.shields.io/badge/GORM-607D8B?style=flat-square)
![React 19](https://img.shields.io/badge/React-19-149ECA?style=flat-square&logo=react&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=flat-square&logo=typescript&logoColor=white)
![Rsbuild](https://img.shields.io/badge/Rsbuild-FF6B35?style=flat-square)

![Tailwind CSS 4](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white)
![Base UI](https://img.shields.io/badge/Base_UI-111827?style=flat-square)
![Bun](https://img.shields.io/badge/Bun-14151A?style=flat-square&logo=bun&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-003B57?style=flat-square&logo=sqlite&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-4479A1?style=flat-square&logo=mysql&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=flat-square&logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-DC382D?style=flat-square&logo=redis&logoColor=white)
![Docker Compose](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)

- **後端：** Go（`go.mod` 宣告 1.25.1）、Gin HTTP 路由、GORM 資料存取
- **管理介面：** React 19、TypeScript、Rsbuild、Tailwind CSS 4、Base UI；Bun 管理前端依賴及腳本
- **儲存：** 預設 SQLite，或選擇 MySQL/PostgreSQL；Redis 可選，用於共用快取與限流
- **部署：** Docker 映像與 Docker Compose；映像安裝不需本機 Go 或前端建置工具鏈

### 請求流程

```text
應用程式 + API Key → MyAPI 身分驗證與權限檢查
                  → 模型與渠道選擇 → 模型服務
                  ← 回應 / 串流輸出 ←
                    用量與錯誤記錄
```

管理介面設定渠道、模型、使用者及 Key；Go 服務檢查權限、選擇合格渠道、呼叫模型服務並記錄可取得的用量證據。服務憑據保留在伺服器，協定和預算資格仍遵循下文限制。

版本取自倉庫依賴宣告，不保證所有未來版本相容。詳見[後端依賴](go.mod)、[前端依賴](web/package.json)、[容器建置](Dockerfile)。

## 功能與邊界

- 接入 OpenAI 相容 API、Responses、Claude Messages、Gemini、Codex 等既有適配器，明確設定開放模型。支援範圍取決於適配器與模型服務帳號。
- 為每個應用程式或使用者分配獨立應用程式 Key，限制模型、存取方案與適用用量；模型服務憑據保留在伺服器。
- 查看請求、錯誤、用量與受支援供應商的額度觀測，區分缺失、失敗及視窗重設。
- 選擇模型、端點與串流模式測試渠道，在適用時重用最近成功選項。
- SQLite、MySQL、PostgreSQL 三選一。介面支援英、簡中、繁中、法、日、俄、越七種語言。

嚴格 Token/USD 預算目前只限已取得資格的官方原生 Responses 純文字路徑；USD 另需適用的凍結價格與服務級別。模型別名、轉換、工具或多模態不會自動取得資格。Codex 百分比是帳戶/視窗剩餘量的安全門檻，不是逐 Key 的共享訂閱消耗帳本。訂閱 API 等價成本僅供參考，不等於實際帳單；未知或估算用量也不等於實際零用量。

## 版本狀態

截至 2026-10-04：

- **預發布候選：`v0.2.0-beta.7`。** 這是累計包含 beta.4–7 工作的候選版本；在標籤與發布工作流完成前，`v0.2.0-beta.3` 仍是最後一個已發布預覽版。範圍、驗證證據與發布邊界見 [beta.7 發布記錄](docs/RELEASE_BETA_7.md)。
- **beta.4 僅已驗證開發原始碼，未發布**：限定的模型發現、映射、預覽、分發和日誌流程不在 beta.3 映像內。
- **beta.5 僅本機開發候選**：帳戶排程、暫時冷卻、有界故障切換與嘗試說明尚未發布，該候選的遠端三資料庫與 Chromium 驗證仍待完成。

預發布不表示已適合正式營運。真實 OAuth、額度重設/429、帳單核對與目標 HTTPS 驗收仍有限或未完成。容器健康不等於模型服務驗收；取得開發原始碼也不會改變預設 beta.3 映像。統一 Lite/Desktop 安裝更新器尚未交付，桌面建置成功不等於實機驗收。

## 安裝已發布版本

推薦 **Linux Full、固定版本 Docker 映像、SQLite、回環監聽加 HTTPS 反向代理**。

需要 Linux amd64/arm64、Git、Bash、Docker daemon，以及支援 `up --wait --wait-timeout` 的 Compose v2；另需倉庫讀取權限、GitHub/GHCR 連線、持久儲存與自己的 HTTPS Origin。反代指向 `http://127.0.0.1:3000`。下列密鑰生成使用 OpenSSL。映像安裝不需要 Go、Bun、Node.js、Redis 或獨立資料庫。預設 2 CPU / 2 GiB 是容器資源上限，不是測得的最低硬體需求。

### 1. 取得部署檔案

僅用於新目錄的全新安裝；既有實例不得覆寫 `.env`：

```bash
git clone --branch v0.2.0-beta.7 --single-branch https://github.com/ForceMind/MyAPI.git my-api
cd my-api
umask 077
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

### 2. 設定

編輯 `deploy/.env`，將範例改為自己的準確 HTTPS Origin，不含 API 路徑。安裝器會拒絕 `example.com` 佔位值：

```dotenv
MYAPI_IMAGE=ghcr.io/forcemind/myapi:v0.2.0-beta.7
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

僅本機或私網使用，請參閱[舊版 LAN 指南](docs/LAN_LITE.md)，選用 `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.7`；LAN 分享須明確開啟。

## 第一次呼叫

1. 在「渠道」(`/channels`) 選擇正確供應商、地址與合法憑據。容器 Codex 使用現有網頁登入流程，無法讀取宿主登入檔案。
2. 取得模型或手填準確 ID，確認啟用模型、分組/存取方案與映射。發現不等於授權或成功；增強 beta.4 流程需要對應開發原始碼。
3. 明確選模型、端點及串流模式做一次小型測試；這會聯絡模型服務，可能消耗額度或產生費用，請用非敏感輸入。
4. 為應用建立最小權限應用程式 Key，不分享模型服務憑據或管理員登入令牌。
5. OpenAI 相容客戶端使用 HTTPS Origin 加 `/v1`、應用程式 Key 和已啟用的公共模型名；其他協定使用其文件端點。先送簡短請求。
6. 到「用量日誌」(`/usage-logs/common`) 查看狀態、模型、用量證據與費用；開發版的管理員路由詳情可核對目標。預覽不保證隨機結果或某個 Key 准入。

失敗先檢查端點、模型、憑據、權限及模型服務額度，再考慮重試。結果不明時保留待核對狀態。

## 維護與恢復

[部署模板](deploy/.env.example)與[執行環境變數](.env.example)是不同層；任意加入 `.env` 的變數不會自動傳入容器，外部資料庫/Redis 設定須核對 [Compose](deploy/docker-compose.yml)。SQLite 為預設；MySQL ≥ 5.7.8、PostgreSQL ≥ 9.6 是相容基線，應選適當維護版本。Redis 可選，多節點的會話與限流需另行設定。

原始碼 CLI 需要 Node.js ≥ 20。本指南使用已取得倉庫中的 CLI，不依賴從 NPM 安裝套件：

```bash
node cli/myapi.mjs help
node cli/myapi.mjs doctor --project-dir .
node cli/myapi.mjs status --project-dir .
node cli/myapi.mjs logs --project-dir .
```

`doctor` 不等於真實模型服務驗收，日誌可能含敏感資訊；`install`、`switch`、`rollback` 尚非可用命令。

升級前記錄版本/digest、revision、路徑與設定，備份一致的資料庫（包含適用的 SQLite WAL）、秘密、完整密鑰環和必要日誌。在隔離副本驗證恢復，再演練已發布的目標版本。例如舊實例升級至 beta.3 的只讀預檢：

```bash
node cli/myapi.mjs upgrade --project-dir ./upgrade-copy --version v0.2.0-beta.7 --dry-run --json
```

依[升級與恢復指南](docs/UPGRADE_REHEARSAL.md)完成實際操作。CLI 的 `.env` 備份不是資料庫備份；嘗試目標啟動後不會自動重啟舊映像，資料庫可能已遷移。使用不相容舊程式前須恢復已驗證的升級前備份。勿刪資料卷、清空待核對記錄，或讓舊程式直接開啟新資料庫。

## 安全與文件

保留回環監聽，分享前檢查 HTTPS、可信代理、註冊、角色及 Key 權限。只使用合法授權帳號/API，遵守模型服務條款與適用法律；對外服務需另行處理合規義務。部署模板預設開啟完整內容日誌，本頁範例已關閉；若啟用，先設定權限、保留期限和備份，脫敏不保證內容沒有隱私。額度取樣會在背景聯絡模型服務。秘密、OAuth 檔案、資料庫及私有日誌不可放進 Git 或問題回報。

- [發布與驗證](docs/RELEASE_BETA_7.md) · [部署](DEPLOYMENT_CUSTOM.md) · [LAN](docs/LAN_LITE.md)
- [恢復](docs/UPGRADE_REHEARSAL.md) · [安裝驗收](docs/R1_INSTALLATION_CHECK.md)
- [Relay API](docs/openapi/relay.json) · [管理 API](docs/openapi/api.json)
- [額度分析](docs/QUOTA_ANALYTICS.md) · [Claude 組織用量](docs/CLAUDE_USAGE_REPORT.md)
- [認證](docs/authentication.md) · [內容日誌](docs/FULL_CONTENT_LOGGING_CUSTOM.md)
- [問題回報](https://github.com/ForceMind/MyAPI/issues)：附版本/revision、部署方式與脫敏重現步驟

開發者入口：[開發計畫與實作記錄](docs/MYAPI_MASTER_PLAN.md)。

## 授權與法律聲明

專案採用 [GNU AGPLv3](LICENSE)。[NOTICE](NOTICE) 說明第 7 節附加條件及必要法律／介面署名，[第三方授權](THIRD-PARTY-LICENSES.md) 收錄依賴通知。分發或透過網路提供修改版時，請保留適用通知、標明修改並履行對應原始碼提供義務；桌面發行亦須保留相關 Electron/Chromium 通知。使用或再分發前請閱讀完整條款。
