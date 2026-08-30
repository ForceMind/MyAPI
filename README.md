<div align="center">

![MyAPI](/web/public/myapi-logo-v1.png)

# MyAPI

🍥 **Next-Generation LLM Gateway and AI Asset Management System**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <strong>English</strong> |
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
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-key-features">Key Features</a> •
  <a href="#-deployment">Deployment</a> •
  <a href="#-documentation">Documentation</a> •
  <a href="#-help-support">Help</a>
</p>

</div>

## MyAPI self-hosted distribution

<img src="./web/public/myapi-logo-v1.png" alt="MyAPI" width="160" />

This repository ships the **MyAPI** self-hosting CLI and complete source
distribution for the rc.25 compatibility baseline. The distribution layer uses
the `my-api` machine slug; API, SSE, database, and provider protocol contracts
remain compatible. Required license, NOTICE, and third-party attribution text
is kept in the repository's legal files.

```bash
npx @forcemind/myapi init ./myapi-source
npx @forcemind/myapi configure \
  --project-dir ./myapi-source \
  --public-url https://api.your-domain.com
npx @forcemind/myapi doctor --project-dir ./myapi-source
```

See [the MyAPI distribution guide](./docs/MYAPI_DISTRIBUTION.md) for deployment,
data adoption, validation, and publishing instructions.

## 📝 Project Description

> [!IMPORTANT]
> - This project is intended solely for lawful and authorized AI API gateway, organization-level authentication, multi-model management, usage analytics, cost accounting, and private deployment scenarios.
> - Users must lawfully obtain upstream API keys, accounts, model services, and interface permissions, and must comply with upstream terms of service and applicable laws and regulations.
> - Users should ensure their use complies with upstream terms of service and applicable laws and regulations.
> - When providing generative AI services to the public, users should comply with applicable regulatory requirements and fulfill all filing, licensing, content safety, real-name verification, log retention, tax, and upstream authorization obligations required by their jurisdiction.

---

## 🙏 Acknowledgements

MyAPI uses and links to third-party projects and providers where their APIs or
licenses require it. See the repository's LICENSE, NOTICE, and dependency
metadata for the complete legal and attribution notices. No third-party client
or sponsorship promotion is enabled by default.

---

## 🚀 Quick Start

### Using Docker Compose (Recommended)

```bash
# Clone the project
git clone https://github.com/ForceMind/MyAPI.git my-api
cd my-api

# Edit docker-compose.yml configuration
nano docker-compose.yml

# Start the service
docker-compose up -d
```

<details>
<summary><strong>Using Docker Commands</strong></summary>

```bash
# Build the local image (for local development only)
docker build -t local/my-api:custom-rc25 .

# Using SQLite (default)
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25

# Using MySQL
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

> **💡 Tip:** `-v ./data:/data` will save data in the `data` folder of the current directory, you can also change it to an absolute path like `-v /your/custom/path:/data`

</details>

---

🎉 After deployment is complete, visit `http://localhost:3000` to start using!

> [!WARNING]
> When operating this project as a public generative AI service or API resale service, users should first complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations.

📖 For more deployment methods, please refer to the [MyAPI deployment guide](./DEPLOYMENT_CUSTOM.md)

---

## 📚 Documentation

<div align="center">

### 📖 [MyAPI Documentation](https://github.com/ForceMind/MyAPI/tree/main/docs)

</div>

**Quick Navigation:**

| Category | Link |
|------|------|
| 🚀 Deployment Guide | [Installation Documentation](./DEPLOYMENT_CUSTOM.md) |
| ⚙️ Environment Configuration | [Deployment configuration](./DEPLOYMENT_CUSTOM.md) |
| 📡 API Documentation | [OpenAPI specifications](./docs/openapi) |
| 📊 Claude organization usage | [Official API boundary](./docs/CLAUDE_USAGE_REPORT.md) |
| ❓ FAQ | [GitHub Discussions](https://github.com/ForceMind/MyAPI/discussions) |
| 💬 Community Interaction | [GitHub Discussions](https://github.com/ForceMind/MyAPI/discussions) |

---

## ✨ Key Features

> For detailed features, see the [MyAPI distribution guide](./docs/MYAPI_DISTRIBUTION.md).

### 🎨 Core Functions

| Feature | Description |
|------|------|
| 🎨 New UI | Modern user interface design |
| 🌍 Multi-language | Supports Simplified Chinese, Traditional Chinese, English, French, Japanese |
| 🔄 Data Compatibility | Compatible with existing data migrations |
| 📈 Data Dashboard | Visual console and statistical analysis |
| 🔒 Permission Management | Token grouping, model restrictions, user management |

### 💰 Authorized Usage Accounting and Billing

- ✅ Internal top-up and quota allocation for lawful authorized scenarios (EPay, Stripe)
- ✅ Organization-level per-request, usage-based, and cache-hit cost accounting
- ✅ Cache billing statistics for OpenAI, Azure, DeepSeek, Claude, Qwen, and supported models
- ✅ Flexible billing policies for internal management or authorized enterprise customers

### 🔐 Authorization and Security

- 😈 Discord authorization login
- 🤖 LinuxDO authorization login
- 📱 Telegram authorization login
- 🔑 OIDC unified authentication
- 🔍 Key quota query and audit logs are available from the MyAPI administration panel

### 🚀 Advanced Features

**API Format Support:**
- ⚡ OpenAI Responses and Realtime API (including Azure)
- ⚡ Claude Messages
- ⚡ Google Gemini
- 🔄 Rerank Models (Cohere, Jina)

**Intelligent Routing:**
- ⚖️ Channel weighted random
- 🔄 Automatic retry on failure
- 🚦 User-level model rate limiting

**Format Conversion:**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible**
- 🔄 **OpenAI Compatible ⇄ OpenAI Responses** - Supported through the Responses compatibility adapter
- 🔄 **Thinking-to-content functionality**

**Reasoning Effort Support:**

<details>
<summary>View detailed configuration</summary>

**OpenAI series models:**
- `o3-mini-high` - High reasoning effort
- `o3-mini-medium` - Medium reasoning effort
- `o3-mini-low` - Low reasoning effort
- `gpt-5-high` - High reasoning effort
- `gpt-5-medium` - Medium reasoning effort
- `gpt-5-low` - Low reasoning effort

**Claude thinking models:**
- `claude-3-7-sonnet-20250219-thinking` - Enable thinking mode

**Google Gemini series models:**
- `gemini-2.5-flash-thinking` - Enable thinking mode
- `gemini-2.5-flash-nothinking` - Disable thinking mode
- `gemini-2.5-pro-thinking` - Enable thinking mode
- `gemini-2.5-pro-thinking-128` - Enable thinking mode with thinking budget of 128 tokens
- You can also append `-low`, `-medium`, or `-high` to any Gemini model name to request the corresponding reasoning effort (no extra thinking-budget suffix needed).

</details>

---

## 🤖 Model Support

> For details, see the [OpenAPI specifications](./docs/openapi).

| Model Type | Description | Documentation |
|---------|------|------|
| 🤖 OpenAI-Compatible | OpenAI compatible models | [OpenAPI](./docs/openapi/relay.json) |
| 🤖 OpenAI Responses | OpenAI Responses format | [OpenAPI](./docs/openapi/relay.json) |
| 🎨 Midjourney-Proxy | [Midjourney-Proxy(Plus)](https://github.com/novicezk/midjourney-proxy) | [OpenAPI](./docs/openapi/relay.json) |
| 🎵 Suno-API | [Suno API](https://github.com/Suno-API/Suno-API) | [OpenAPI](./docs/openapi/relay.json) |
| 🔄 Rerank | Cohere, Jina | [OpenAPI](./docs/openapi/relay.json) |
| 💬 Claude | Messages format | [OpenAPI](./docs/openapi/relay.json) |
| 🌐 Gemini | Google Gemini format | [OpenAPI](./docs/openapi/relay.json) |
| 🔧 Dify | ChatFlow mode | - |
| 🎯 Custom upstream | Supports configuring legally authorized upstream endpoints | - |

### 📡 Supported Interfaces

<details>
<summary>View complete interface list</summary>

- [Chat Interface (Chat Completions)](./docs/openapi/relay.json)
- [Response Interface (Responses)](./docs/openapi/relay.json)
- [Image Interface (Image)](./docs/openapi/relay.json)
- [Audio Interface (Audio)](./docs/openapi/relay.json)
- [Video Interface (Video)](./docs/openapi/relay.json)
- [Embedding Interface (Embeddings)](./docs/openapi/relay.json)
- [Rerank Interface (Rerank)](./docs/openapi/relay.json)
- [Realtime Conversation (Realtime)](./docs/openapi/relay.json)
- [Claude Chat](./docs/openapi/relay.json)
- [Google Gemini Chat](./docs/openapi/relay.json)

</details>

---

## 🚢 Deployment

> [!TIP]
> **Default Docker image:** `ghcr.io/forcemind/myapi:<version>` (built by GitHub Actions; set `MYAPI_IMAGE` to a published tag to upgrade). Set `MYAPI_BUILD_LOCAL=true` to build locally.

推送新的 `vX.Y.Z` 版本 tag 后，GitHub Actions 会自动构建并推送 GHCR 多架构镜像；
部署端只需将 `MYAPI_IMAGE` 改为新 tag 后运行更新命令。

### 📋 Deployment Requirements

| Component | Requirement |
|------|------|
| **Local database** | SQLite (Docker must mount `/data` directory)|
| **Remote database** | MySQL ≥ 5.7.8 or PostgreSQL ≥ 9.6 |
| **Container engine** | Docker / Docker Compose |
| **System architecture** | 64-bit only (amd64 / arm64); 32-bit systems are not supported |

### ⚙️ Environment Variable Configuration

<details>
<summary>Common environment variable configuration</summary>

| Variable Name | Description | Default Value |
|--------|------|--------|
| `SESSION_SECRET` | Authentication signing secret; must be identical on every node | - |
| `SESSION_COOKIE_SECURE` | `false`/unset disables the refresh/logout OriginGuard for local HTTP dev proxies; `true` enables the Secure cookie and strict Origin checks | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Required with Secure mode: comma-separated exact HTTPS Origins allowed to call refresh/logout; not a relay CORS allowlist | - |
| `TRUSTED_PROXIES` | Unset/blank trusts loopback, RFC 1918 and IPv6 ULA with a startup warning; `none` trusts no proxies; an explicit proxy IP/CIDR list replaces the defaults | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | Maximum active login Sessions per user | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | Maximum Sessions created per user within the issuance window, including revoked Sessions | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Per-user Session issuance window; clamped to the revoked retention period when configured higher | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | Days to retain revoked Session rows for audit and issuance accounting | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | Global Sessions created per hour that triggers an alert only; it never blocks login | `5000` |
| `CRYPTO_SECRET` | HMAC secret for cache keys; nodes sharing Redis must use the same effective value | Defaults to `SESSION_SECRET` |
| `SQL_DSN` | Database connection string | - |
| `REDIS_CONN_STRING` | Redis connection string | - |
| `RELAY_IDLE_CONN_TIMEOUT` | Idle keep-alive timeout for relay HTTP clients, seconds. Defaults to Go standard library behavior; set `0` to disable | `90` |
| `STREAMING_TIMEOUT` | Streaming timeout (seconds) | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | Max per-line buffer (MB) for the stream scanner; increase when upstream sends huge image/base64 payloads | `64` |
| `MAX_REQUEST_BODY_MB` | Max request body size (MB, counted **after decompression**; prevents huge requests/zip bombs from exhausting memory). Exceeding it returns `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure API version | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | Error log switch | `false` |
| `PYROSCOPE_URL` | Pyroscope server address | - |
| `PYROSCOPE_APP_NAME` | Pyroscope application name | `my-api` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope basic auth user | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope basic auth password | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutex sampling rate | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope block sampling rate | `5` |
| `HOSTNAME` | Hostname tag for Pyroscope | `my-api` |

📖 **Complete configuration:** [Deployment configuration](./DEPLOYMENT_CUSTOM.md)

</details>

### 🔧 Deployment Methods

<details>
<summary><strong>Method 1: Docker Compose (Recommended)</strong></summary>

```bash
# Clone the project
git clone https://github.com/ForceMind/MyAPI.git my-api
cd my-api

# Edit configuration
nano docker-compose.yml

# Start service
docker-compose up -d
```

</details>

<details>
<summary><strong>Method 2: Docker Commands</strong></summary>

**Using SQLite:**
```bash
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

**Using MySQL:**
```bash
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

> **💡 Path explanation:**
> - `./data:/data` - Relative path, data saved in the data folder of the current directory
> - You can also use absolute path, e.g.: `/your/custom/path:/data`

</details>

<details>
<summary><strong>Method 3: BaoTa Panel</strong></summary>

1. Install BaoTa Panel (≥ 9.2.0 version)
2. Search for **MyAPI** in the application store
3. One-click installation

📖 [Tutorial with images](./docs/installation/BT.md)

</details>

### ⚠️ Multi-machine Deployment Considerations

> [!WARNING]
> - All nodes must use the same primary database and the same `SESSION_SECRET`; otherwise Access Tokens, refresh sessions, and temporary authentication flows cannot be verified consistently.
> - Nodes connected to the same Redis must also use the same `CRYPTO_SECRET`, or their cache-key digests will differ and shared entries cannot be reused consistently.

The database is authoritative for login Sessions and for the per-user active/issuance limits. Redis Session entries are short-lived caches whose TTL follows `SYNC_FREQUENCY` (60 seconds by default) and never exceeds the Session's remaining lifetime.

| Redis topology | Session propagation | Rate limiting |
| --- | --- | --- |
| Shared Redis | Revocations and version publications normally propagate immediately | Redis limits are shared across nodes |
| Independent Redis per node | Nodes converge from the database within the effective `SYNC_FREQUENCY`; a newly rotated token may receive a temporary 401 on a node with stale cache | Each node has its own allowance, so aggregate capacity can reach roughly the configured limit multiplied by the node count |
| No Redis | Every Session validation reads the database | In-memory limits are independent per node |

A shorter `SYNC_FREQUENCY` reduces the independent-Redis staleness window but causes one additional primary-key Session lookup per active SID, per node, per TTL. These guarantees make Session authentication bounded-stale across the supported topologies; rate limits and other Redis-backed control-plane caches remain topology-dependent.

See [User authentication and login sessions](./docs/authentication.md) for the token, Origin-check and PAT contracts.

### 🔄 Channel Retry and Cache

**Retry configuration:** `Settings → Operation Settings → General Settings → Failure Retry Count`

**Cache configuration:**
- `REDIS_CONN_STRING`: Redis cache (recommended)
- `MEMORY_CACHE_ENABLED`: Memory cache

---

## 🔗 Related Projects

### Upstream Projects

| Project | Description |
|------|------|
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney interface support |

### Supporting Tools

Use the built-in MyAPI administration panel for key quota queries and audit logs.
Third-party tools are intentionally not promoted by the default distribution.

---

## 💬 Help Support

### 📖 Documentation Resources

| Resource | Link |
|------|------|
| 📘 FAQ | [FAQ](https://github.com/ForceMind/MyAPI/discussions) |
| 💬 Community Interaction | [GitHub Discussions](https://github.com/ForceMind/MyAPI/discussions) |
| 🐛 Issue Feedback | [Issue tracker](https://github.com/ForceMind/MyAPI/issues) |
| 📚 Complete Documentation | [Repository documentation](https://github.com/ForceMind/MyAPI/tree/main/docs) |

### 🤝 Contribution Guide

Welcome all forms of contribution!

- 🐛 Report Bugs
- 💡 Propose New Features
- 📝 Improve Documentation
- 🔧 Submit Code

---

## 📜 License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE).

Additional terms under AGPLv3 Section 7 and any required attribution obligations
are documented in `LICENSE` and `NOTICE`. Modified versions must preserve the
required notices in the appropriate legal files and in any prominent About,
legal, footer, or attribution location presented by the user interface. Review
those files before redistributing a modified build.

MyAPI is an independent distribution with compatibility adapters for established data and API contracts.

If your organization's policies do not permit the use of AGPLv3-licensed
software, review the license obligations with your legal team before use.

---

<div align="center">

### 💖 Thank you for using MyAPI

If this project is helpful to you, welcome to give us a ⭐️ Star！

**[Documentation](https://github.com/ForceMind/MyAPI/tree/main/docs)** • **[Issue Feedback](https://github.com/ForceMind/MyAPI/issues)** • **[Latest Release](https://github.com/ForceMind/MyAPI/releases)**

<sub>Built with ❤️ by ForceMind</sub>

</div>
