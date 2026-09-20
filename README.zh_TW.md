<div align="center">

![MyAPI](/web/public/myapi-logo-v1.png)

# MyAPI

🍥 **新一代大模型網關與AI資產管理系統**

<p align="center">
  繁體中文 |
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.md">English</a> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/ForceMind/MyAPI/main/LICENSE">
    <img src="https://img.shields.io/github/license/ForceMind/MyAPI?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/ForceMind/MyAPI/releases/latest">
    <img src="https://img.shields.io/github/v/release/ForceMind/MyAPI?color=brightgreen&include_prereleases" alt="release">
  </a>
</p>

<p align="center">
  <a href="#-快速開始">快速開始</a> •
  <a href="#-主要特性">主要特性</a> •
  <a href="#-部署">部署</a> •
  <a href="#-文件">文件</a> •
  <a href="#-幫助支援">幫助</a>
</p>

</div>

## MyAPI 自建發行版

<img src="./web/public/myapi-logo-v1.png" alt="MyAPI" width="160" />

本倉庫提供 **MyAPI** 自建 CLI 與完整原始碼發行包，採用 rc.25 技術相容基線（API/協定契約，不是 UI 模板）。
發行層統一使用 my-api 機器識別；API、SSE、資料庫與上游協議契約保持相容。
所需的授權條款、NOTICE 與第三方歸屬文字保存在倉庫法律文件中。

```bash
npx @forcemind/myapi init ./myapi-source
npx @forcemind/myapi configure \
  --project-dir ./myapi-source \
  --public-url https://api.your-domain.com
npx @forcemind/myapi doctor --project-dir ./myapi-source
```

> **目前發布狀態：** NPM 套件尚未正式發布。在維護者發布前，請於源碼檢出目錄將上述
> 指令替換為 `node cli/myapi.mjs ...`，或使用本地打包的 tarball；私有倉庫無法由
> `npx` 自動取得。

部署、既有資料接管、驗證與發布步驟請參閱
[MyAPI 發行說明](./docs/MYAPI_DISTRIBUTION.md)。

## 📝 項目說明

> [!IMPORTANT]
> - 本專案僅面向合法授權的 AI API 閘道、組織內部鑑權、多模型管理、用量統計、成本核算和私有化部署場景。
> - 使用者必須合法取得上游 API Key、帳號、模型服務或介面權限，並遵守上游服務條款及適用法律法規。
> - 使用者應確保其使用方式符合上游服務條款及適用法律法規。
> - 面向公眾提供生成式人工智慧服務時，使用者應遵守[《生成式人工智慧服務管理暫行辦法》](http://www.cac.gov.cn/2023-07/13/c_1690898327029107.htm)等監管要求，自行完成所在司法轄區要求的備案、許可、內容安全、實名、日誌留存、稅務和上游授權等合規義務。

---

## 🙏 致謝

MyAPI 僅在介面或授權條款要求時保留第三方引用。完整通知請查看 LICENSE、NOTICE
與依賴元資料；預設發行版不啟用第三方客戶端或贊助推廣。

---

## 🚀 快速開始

### 使用 Docker Compose（推薦）

```bash
# 複製項目
git clone https://github.com/ForceMind/MyAPI.git my-api
cd my-api

# 編輯 docker-compose.yml 配置
nano docker-compose.yml

# 啟動服務
docker-compose up -d
```

<details>
<summary><strong>使用 Docker 命令</strong></summary>

```bash
# Build the local image (for local development only)
docker build -t local/my-api:custom-rc25 .

# 使用 SQLite（預設）
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25

# 使用 MySQL
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/myapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

> **💡 提示：** `-v ./data:/data` 會將數據保存在當前目錄的 `data` 資料夾中，你也可以改為絕對路徑如 `-v /your/custom/path:/data`

</details>

---

🎉 部署完成後，訪問 `http://localhost:3000` 即可使用！

> [!WARNING]
> 將本專案作為面向公眾的生成式 AI 服務或 API 轉售服務運營時，使用者應先完成備案、內容安全、實名、日誌留存、稅務、支付和上游授權等合規義務。

📖 更多部署方式請參考 [部署指南](./DEPLOYMENT_CUSTOM.md)

---

## 📚 文件

<div align="center">

### 📖 [官方文件](https://github.com/ForceMind/MyAPI/tree/main/docs) | [![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://github.com/ForceMind/MyAPI/discussions)

</div>

**快速導航：**

| 分類 | 連結 |
|------|------|
| 🚀 部署指南 | [安裝文件](./DEPLOYMENT_CUSTOM.md) |
| ⚙️ 環境配置 | [環境變數](./DEPLOYMENT_CUSTOM.md) |
| 📡 接口文件 | [API 文件](./docs/openapi/relay.json) |
| 📊 Claude 組織用量 | [官方 API 邊界](./docs/CLAUDE_USAGE_REPORT.md) |
| 🧭 產品與工程計畫 | [MyAPI 總體計畫](./docs/MYAPI_MASTER_PLAN.md) |
| ✅ 完成度證據 | [完成度稽核](./docs/COMPLETION_AUDIT.md) |
| 🖥️ LAN Lite 與桌面版 | [LAN Lite 指南](./docs/LAN_LITE.md) |
| 🍎 macOS 開發遷移 | [macOS 開發指南](./docs/DEVELOPMENT_ON_MACOS.md) |
| 🧭 新裝置 Codex 交接 | [可複製的交接提示詞](./docs/CODEX_HANDOFF_PROMPT.md) |
| ❓ 常見問題 | [FAQ](https://github.com/ForceMind/MyAPI/discussions) |
| 💬 社群交流 | [交流管道](https://github.com/ForceMind/MyAPI/discussions) |

---

## ✨ 主要特性

> 詳細特性請參考 [特性說明](https://github.com/ForceMind/MyAPI/tree/main/docs)

### 🎨 核心功能

| 特性 | 說明 |
|------|------|
| 🎨 全新 UI | 現代化的用戶界面設計 |
| 🌍 多語言 | 支援簡體中文、繁體中文、英文、法語、日語 |
| 🔄 數據兼容 | 支援既有資料遷移與相容 |
| 📈 數據看板 | 視覺化控制檯與統計分析 |
| 🔒 權限管理 | 令牌分組、模型限制、用戶管理 |

### 💰 授權用量與成本管理

- ✅ 合法授權場景下的內部儲值與額度分配（易支付、Stripe）
- ✅ 組織內按次、按量或快取命中成本核算
- ✅ 支援 OpenAI、Azure、DeepSeek、Claude、Qwen 等模型的快取計費統計
- ✅ 面向內部管理或企業客戶的靈活計費策略配置

### 🔐 授權與安全

- 😈 Discord 授權登錄
- 🤖 LinuxDO 授權登錄
- 📱 Telegram 授權登錄
- 🔑 OIDC 統一認證

### 🚀 高級功能

**API 格式支援：**
- ⚡ [OpenAI Responses](./docs/openapi/relay.json)
- ⚡ [OpenAI Realtime API](./docs/openapi/relay.json)（含 Azure）
- ⚡ [Claude Messages](./docs/openapi/relay.json)
- ⚡ [Google Gemini](./docs/openapi/relay.json)
- 🔄 [Rerank 模型](./docs/openapi/relay.json)（Cohere、Jina）

**智慧路由：**
- ⚖️ 管道加權隨機
- 🔄 失敗自動重試
- 🚦 用戶級別模型限流

**格式轉換：**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible**
- 🔄 **OpenAI Compatible ⇄ OpenAI Responses** - 透過 Responses 相容轉接器支援
- 🔄 **思考轉內容功能**

**Reasoning Effort 支援：**

<details>
<summary>查看詳細配置</summary>

**OpenAI 系列模型：**
- `o3-mini-high` - High reasoning effort
- `o3-mini-medium` - Medium reasoning effort
- `o3-mini-low` - Low reasoning effort
- `gpt-5-high` - High reasoning effort
- `gpt-5-medium` - Medium reasoning effort
- `gpt-5-low` - Low reasoning effort

**Claude 思考模型：**
- `claude-3-7-sonnet-20250219-thinking` - 啟用思考模式

**Google Gemini 系列模型：**
- `gemini-2.5-flash-thinking` - 啟用思考模式
- `gemini-2.5-flash-nothinking` - 禁用思考模式
- `gemini-2.5-pro-thinking` - 啟用思考模式
- `gemini-2.5-pro-thinking-128` - 啟用思考模式，並設置思考預算為128tokens
- 也可以直接在 Gemini 模型名稱後追加 `-low` / `-medium` / `-high` 來控制思考力道（無需再設置思考預算後綴）

</details>

---

## 🤖 模型支援

> 詳情請參考 [接口文件 - 閘道接口](./docs/openapi/relay.json)

| 模型類型 | 說明 | 文件 |
|---------|------|------|
| 🤖 OpenAI-Compatible | OpenAI 兼容模型 | [文件](./docs/openapi/relay.json) |
| 🤖 OpenAI Responses | OpenAI Responses 格式 | [文件](./docs/openapi/relay.json) |
| 🎨 Midjourney-Proxy | [Midjourney-Proxy(Plus)](https://github.com/novicezk/midjourney-proxy) | [文件](./docs/openapi/relay.json) |
| 🎵 Suno-API | [Suno API](https://github.com/Suno-API/Suno-API) | [文件](./docs/openapi/relay.json) |
| 🔄 Rerank | Cohere、Jina | [文件](./docs/openapi/relay.json) |
| 💬 Claude | Messages 格式 | [文件](./docs/openapi/relay.json) |
| 🌐 Gemini | Google Gemini 格式 | [文件](./docs/openapi/relay.json) |
| 🔧 Dify | ChatFlow 模式 | - |
| 🎯 自訂上游 | 支援配置合法授權的上游介面位址 | - |

### 📡 支援的接口

<details>
<summary>查看完整接口列表</summary>

- [聊天接口 (Chat Completions)](./docs/openapi/relay.json)
- [響應接口 (Responses)](./docs/openapi/relay.json)
- [圖像接口 (Image)](./docs/openapi/relay.json)
- [音訊接口 (Audio)](./docs/openapi/relay.json)
- [影片接口 (Video)](./docs/openapi/relay.json)
- [嵌入接口 (Embeddings)](./docs/openapi/relay.json)
- [重排序接口 (Rerank)](./docs/openapi/relay.json)
- [即時對話 (Realtime)](./docs/openapi/relay.json)
- [Claude 聊天](./docs/openapi/relay.json)
- [Google Gemini 聊天](./docs/openapi/relay.json)

</details>

---

## 🚢 部署

> [!TIP]
> **預設 Docker 映像：** `ghcr.io/forcemind/myapi:<version>`（由 GitHub Actions 產生，升級時將 `MYAPI_IMAGE` 改為已發布的版本標籤）。如需本地建置，請設定 `MYAPI_BUILD_LOCAL=true`。

GHCR 發行僅由維護者對既有版本 tag 手動發起，並要求 `PUBLISH`、專用環境和發布 gate；推送 tag 不會自動發布。預發布使用不可變 tag，不會更新穩定 `latest`；部署時只使用已發布的版本 tag 或 digest。

### 📋 部署要求

| 組件 | 要求 |
|------|------|
| **本地資料庫** | SQLite（Docker 需掛載 `/data` 目錄）|
| **遠端資料庫** | MySQL ≥ 5.7.8 或 PostgreSQL ≥ 9.6 |
| **容器引擎** | Docker / Docker Compose |
| **系統架構** | 僅支援 64 位元系統（amd64 / arm64），不支援 32 位元系統 |

### ⚙️ 環境變數配置

<details>
<summary>常用環境變數配置</summary>

| 變數名 | 說明                                                           | 預設值 |
|--------|--------------------------------------------------------------|--------|
| `SESSION_SECRET` | 鑑權簽章密鑰；所有節點必須保持一致                                           | - |
| `SESSION_COOKIE_SECURE` | `false`/未設定時關閉 refresh/logout OriginGuard 以相容本機 HTTP 開發代理；`true` 時啟用 Secure Cookie 和嚴格 Origin 驗證 | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Secure 模式必填：允許呼叫 refresh/logout 的精確 HTTPS Origin，多個值以英文逗號分隔；不是 relay CORS 白名單 | - |
| `TRUSTED_PROXIES` | 未設定/留空時信任本機回送、RFC1918 和 IPv6 ULA 並輸出啟動警告；`none` 不信任任何代理；明確指定的代理 IP/CIDR 清單會完整取代預設值 | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | 單一用戶最大活躍登入 Session 數 | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | 單一用戶在簽發視窗內可建立的 Session 總數，包含已撤銷 Session | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Session 簽發計數視窗（秒）；高於 revoked 保留期時自動限制 | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | revoked Session 用於稽核與簽發計數的保留天數 | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | 全域每小時 Session 簽發告警門檻；只告警，不拒絕登入 | `5000` |
| `CRYPTO_SECRET` | 快取鍵 HMAC 密鑰；共用 Redis 的節點必須使用相同有效值 | 預設跟隨 `SESSION_SECRET` |
| `SQL_DSN` | 資料庫連接字符串                                                     | - |
| `REDIS_CONN_STRING` | Redis 連接字符串                                                  | - |
| `STREAMING_TIMEOUT` | 流式超時時間（秒）                                                    | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | 流式掃描器單行最大緩衝（MB），圖像生成等超大 `data:` 片段（如 4K 圖片 base64）需適當調大 | `64` |
| `MAX_REQUEST_BODY_MB` | 請求體最大大小（MB，**解壓縮後**計；防止超大請求/zip bomb 導致記憶體暴漲），超過將返回 `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure API 版本                                                 | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | 錯誤日誌開關                                                       | `false` |
| `PYROSCOPE_URL` | Pyroscope 服務位址                                            | - |
| `PYROSCOPE_APP_NAME` | Pyroscope 應用名                                        | `my-api` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope Basic Auth 用戶名                        | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope Basic Auth 密碼                  | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutex 採樣率                               | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope block 採樣率                               | `5` |
| `HOSTNAME` | Pyroscope 標籤裡的主機名                                          | `my-api` |

📖 **完整配置：** [環境變數文件](./DEPLOYMENT_CUSTOM.md)

</details>

### 🔧 部署方式

<details>
<summary><strong>方式 1：Docker Compose（推薦）</strong></summary>

```bash
# 複製項目
git clone https://github.com/ForceMind/MyAPI.git my-api
cd my-api

# 編輯配置
nano docker-compose.yml

# 啟動服務
docker-compose up -d
```

</details>

<details>
<summary><strong>方式 2：Docker 命令</strong></summary>

**使用 SQLite：**
```bash
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

**使用 MySQL：**
```bash
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/myapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

> **💡 路徑說明：**
> - `./data:/data` - 相對路徑，數據保存在當前目錄的 data 資料夾
> - 也可使用絕對路徑，如：`/your/custom/path:/data`

</details>

<details>
<summary><strong>方式 3：寶塔面板</strong></summary>

1. 安裝寶塔面板（≥ 9.2.0 版本）
2. 在應用商店搜尋 **MyAPI**
3. 一鍵安裝

📖 [圖文教學](./docs/installation/BT.md)

</details>

### ⚠️ 多機部署注意事項

> [!WARNING]
> - 所有節點必須使用同一個主資料庫，並設定相同的 `SESSION_SECRET`；否則 Access Token、Refresh 工作階段和臨時鑑權流程無法一致驗證。
> - 連線至同一個 Redis 的節點還必須設定相同的 `CRYPTO_SECRET`，否則節點產生的快取鍵摘要不一致，無法正確共用快取。

登入 Session 和單一使用者的活躍數／簽發數限制均以資料庫為權威。Redis 中的 Session 僅為短期快取，TTL 跟隨 `SYNC_FREQUENCY`（預設 60 秒），且不會超過 Session 的剩餘有效期。

| Redis 拓撲 | Session 狀態傳播 | 限流語義 |
| --- | --- | --- |
| 所有節點共用 Redis | 撤銷和版本發布通常即時傳播 | Redis 限流額度在節點間共用 |
| 每個節點使用獨立 Redis | 最遲在有效 `SYNC_FREQUENCY` 內回源資料庫並收斂；版本輪換後，新 Token 在持有舊快取的節點上可能短暫傳回 401 | 每個節點獨立計數，叢集總額度最壞約為單一節點門檻乘以節點數 |
| 不使用 Redis | 每次 Session 驗證都直接讀取資料庫 | 各節點使用獨立的記憶體限流額度 |

縮短 `SYNC_FREQUENCY` 可減少獨立 Redis 的陳舊視窗，但每個活躍 SID 在每個節點上會依該 TTL 增加一次資料庫主鍵查詢。上述保證只讓 Session 鑑權在不同拓撲下維持有界陳舊；限流和其他 Redis 控制面快取仍受拓撲影響。

Token、Origin 驗證和 PAT 契約請參閱[使用者鑑權與登入工作階段](./docs/authentication.md)。

### 🔄 管道重試與快取

**重試配置：** `設置 → 運營設置 → 通用設置 → 失敗重試次數`

**快取配置：**
- `REDIS_CONN_STRING`：Redis 快取（推薦）
- `MEMORY_CACHE_ENABLED`：記憶體快取

---

## 🔗 相關項目

### 上游項目

| 項目 | 說明 |
|------|------|
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney 接口支援 |

### 配套工具

可直接使用 MyAPI 管理後台查詢 Key 額度與稽核日誌；預設發行版不推廣第三方工具。

---

## 💬 幫助支援

### 📖 文件資源

| 資源 | 連結 |
|------|------|
| 📘 常見問題 | [FAQ](https://github.com/ForceMind/MyAPI/discussions) |
| 💬 社群交流 | [交流管道](https://github.com/ForceMind/MyAPI/discussions) |
| 🐛 回饋問題 | [問題回饋](https://github.com/ForceMind/MyAPI/issues) |
| 📚 完整文件 | [官方文件](https://github.com/ForceMind/MyAPI/tree/main/docs) |

### 🤝 貢獻指南

歡迎各種形式的貢獻！

- 🐛 報告 Bug
- 💡 提出新功能
- 📝 改進文件
- 🔧 提交程式碼

---

## 📜 許可證

本項目採用 [GNU Affero 通用公共許可證 v3.0 (AGPLv3)](./LICENSE) 授權。

MyAPI 是獨立的發行版，並透過相容適配器支援既有資料與 API 契約。

AGPLv3 第 7 節及歸屬義務記錄在 LICENSE 與 NOTICE 中；重新發布修改版本前請先閱讀。
如果您的組織無法接受 AGPLv3 義務，請在使用前諮詢法律顧問。


---

<div align="center">

### 💖 感謝使用 MyAPI

如果這個項目對你有幫助，歡迎給我們一個 ⭐️ Star！

**[官方文件](https://github.com/ForceMind/MyAPI/tree/main/docs)** • **[問題回饋](https://github.com/ForceMind/MyAPI/issues)** • **[最新發布](https://github.com/ForceMind/MyAPI/releases)**

<sub>Built with ❤️ by ForceMind</sub>

</div>
