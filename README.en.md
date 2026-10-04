# My API

A self-hosted AI API gateway for your own accounts, models, and applications.

[English](README.md) · [简体中文](README.zh_CN.md) · [繁體中文](README.zh_TW.md) · [Français](README.fr.md) · [日本語](README.ja.md)

My API brings upstream connections, downstream API keys, access controls, and usage records into one web console. It is designed for personal use first, with controlled sharing to a small team. New installations keep commercial modules off: using your own upstream accounts does not require topping up an internal wallet. Users, permissions, key limits, and usage tracking still apply.

## What you can do

- Connect supported upstream providers and configure the models each channel exposes. Existing adapters include OpenAI-compatible APIs, Responses, Claude Messages, Gemini, and Codex; endpoint and feature support depend on the selected adapter and upstream account.
- Give each application or user a separate downstream API key, with its own permitted models, access profile, and applicable usage limits. Keep upstream credentials on the server.
- Inspect requests, errors, usage, and supported provider quota observations. Account charts retain missing, failed, and reset states instead of presenting them as zero consumption.
- Test a channel with an explicit model, endpoint, and streaming mode; reuse the most recent successful test options where applicable.
- Run with SQLite, or configure MySQL/PostgreSQL for a suitable deployment. These are alternatives, not three required services.

The console supports English, Simplified Chinese, Traditional Chinese, French, Japanese, Russian, and Vietnamese. See the [relay API specification](docs/openapi/relay.json) for endpoint shapes; a listed endpoint is not a promise that every provider implements it.

### Important limits

Strict Token/USD budgets currently cover only the qualified official native Responses text-only paths. USD limits also require an applicable frozen price and supported service tier. Model aliases, protocol conversion, tools, or multimedia do not automatically qualify.

Codex percentages are remaining-account/window safety thresholds, not a per-key ledger of shared subscription consumption. Subscription API-equivalent cost is a reference estimate, not the provider's actual bill. Missing or estimated usage is not actual zero; unresolved requests require evidence-based review rather than an assumed refund.

## Choose a version

Status as of 2026-10-04:

- **Published prerelease: `v0.2.0-beta.3`.** Full and legacy LAN images are available for Linux amd64/arm64. Release artifacts, source revision, image digests, signatures, and verification limits are recorded in the [beta.3 release record](docs/RELEASE_BETA_3.md).
- **beta.4: verified development source, not a release.** Its limited model-discovery, explicit mapping, routing-preview, dispatch, and log workflow has automated verification. It is not included in the beta.3 images.
- **beta.5: local development candidate only.** Account scheduling, temporary cooldown, bounded failover, and attempt explanations are not published features. This candidate still lacks its own remote three-database and Chromium verification.

The prerelease is not a production-readiness claim. Real-account OAuth, quota reset/429 behavior, provider-bill reconciliation, and target-server HTTPS acceptance remain limited or pending. A successful build or container health check does not establish those results. Deployment defaults continue to point to beta.3; cloning development source does not change the image you run.

## Install the published prerelease

The recommended starting point is **Full on Linux, using the pinned Docker image and the repository's installer**. This path uses SQLite and binds the application to loopback behind your HTTPS reverse proxy.

### Prerequisites

- Linux amd64 or arm64, Git, Bash, and a working Docker daemon
- Docker Compose v2 with support for `up --wait --wait-timeout`
- An HTTPS origin you control and a reverse proxy forwarding it to `http://127.0.0.1:3000`
- Repository read access and network access to GitHub/GHCR; authorized upstream credentials for later use
- Persistent storage for the database, logs, and backups; OpenSSL for the secret-generation examples below

Go, Bun, Node.js, Redis, and a separate database server are **not required for this image-based installation**. The template caps the container at 2 CPUs and 2 GiB of memory; these are resource limits, not measured minimum hardware requirements. Allow additional host and storage capacity for your workload.

### 1. Get the matching deployment files

For a **new installation in a new directory**:

```bash
git clone --branch v0.2.0-beta.3 --single-branch https://github.com/ForceMind/MyAPI.git my-api
cd my-api
umask 077
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

For an existing installation, preserve its configuration and read the upgrade section first. Do not overwrite an existing `.env` with the example.

### 2. Configure the instance

Edit `deploy/.env`. Keep these values and replace the example origin with your own exact HTTPS origin, without an API path:

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

The installer rejects placeholder `example.com` origins. Generate a new random session secret and put the result in `SESSION_SECRET` using your private editor:

```bash
openssl rand -hex 32
```

The installer requires at least 48 characters. For multi-account quota sampling, generate a **separate** identity key:

```bash
openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'
```

Set `CHANNEL_QUOTA_IDENTITY_KEYS` to `active:v1:` followed by that result. Keep the complete keyring with your database backups; all instances sharing that database must use it. An empty keyring disables identity-dependent quota sampling. Do not regenerate either secret when upgrading an existing database, paste them into support messages, or commit them to Git. The installer reads literal `KEY=VALUE` entries; do not put shell substitutions in `.env`.

With unchanged paths, persistent data and logs live in `deploy/data/` and `deploy/logs/`, mounted as `/data` and `/app/logs`. Keep these directories outside disposable container storage.

### 3. Start and check

```bash
bash deploy/install.sh
```

The script validates Compose configuration, pulls the pinned image, starts the service, and waits up to 120 seconds for container health. It does not install Docker, obtain TLS certificates, configure your proxy, or open firewall ports.

Open your configured **HTTPS origin**, complete the initialization page, and create the administrator account. Full uses Secure cookies: plain `http://localhost:3000` is not its recommended login URL. Confirm login and the runtime version/revision in System Information before adding real accounts.

For local or private-network use instead, follow the [legacy LAN guide](docs/LAN_LITE.md), using the published `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.3` image. LAN sharing is opt-in. The planned unified Lite/Desktop installer and updater are not yet delivered; desktop build artifacts do not establish real-device acceptance.

## Make your first request

1. **Add an upstream channel.** In Channels (`/channels`), choose the actual provider type and enter its authorized endpoint and credentials. For Codex in a container, use the supported browser-login flow; the container cannot read your host's login files.
2. **Enable a model.** Fetch models where supported or enter an exact upstream model ID. Confirm the channel's enabled models, group/access compatibility, and any explicit mapping. Discovery alone does not grant access or prove a model works. The enhanced beta.4 discovery/routing workflow requires its development source, not the beta.3 image.
3. **Run one small channel test.** Select the model, supported endpoint, and streaming mode explicitly. Tests contact the upstream and can consume quota or incur charges. Start with non-sensitive input.
4. **Create a downstream API key.** Use a separate key for your application, grant only the needed models/access profile, and choose only limits supported by that channel. Do not distribute the upstream key or an administrator login token.
5. **Configure your client.** For an OpenAI-compatible client, use your HTTPS origin followed by `/v1`, the downstream key, and the exact enabled public model name. Other protocols must use their documented endpoints. Send one short request before enabling a workload.
6. **Inspect the result.** Check Usage Logs (`/usage-logs/common`) for status, model, usage evidence, and applicable cost information. Administrators can inspect additional details. In development builds with routing evidence, also verify the selected channel and mapped target; a routing preview does not guarantee the next random choice or a particular key's admission.

If a test fails, check provider type, endpoint, credentials, enabled model, key permissions, and upstream quota before increasing retry counts. Keep an unresolved usage record unresolved until you have reliable evidence.

## Configuration and operations

[Deployment details](DEPLOYMENT_CUSTOM.md), the [deployment template](deploy/.env.example), and the [runtime environment reference](.env.example) cover advanced configuration. The deployment file and runtime environment are different layers: adding an arbitrary runtime variable to `deploy/.env` does not automatically pass it into the container. Review [the Compose file](deploy/docker-compose.yml) when configuring an external database or Redis.

SQLite is the default for the recommended path. MySQL ≥ 5.7.8 and PostgreSQL ≥ 9.6 are compatibility baselines, not recommendations to operate unsupported database versions. Use an appropriately maintained version and rehearse migrations. Redis is optional; multi-node deployments require deliberate session/cache/rate-limit configuration. See [authentication and session behavior](docs/authentication.md).

The source CLI requires Node.js ≥ 20. This guide uses the CLI from the checked-out repository and does not depend on installing a package from NPM:

```bash
node cli/myapi.mjs help
node cli/myapi.mjs doctor --project-dir .
node cli/myapi.mjs status --project-dir .
node cli/myapi.mjs logs --project-dir .
```

`doctor` checks configuration and tool availability; it is not live upstream acceptance. Logs may contain sensitive data. The CLI does not provide the planned `install`, `switch`, or `rollback` commands.

### Upgrade, backup, and recovery

1. Record the running image tag/digest, source revision, database type, data paths, and private configuration. Use an actually published target version, never a planned beta label or `latest`.
2. Create a consistent database backup using an appropriate database procedure. Preserve SQLite WAL state when applicable, configuration, session secret, complete quota identity keyring, and required logs. Verify restoration on an isolated copy before upgrading the live instance.
3. Rehearse the target version on that copy, including login, channels, keys, a controlled request, logs, and recovery. The CLI supports a read-only preflight, for example when rehearsing an older instance's upgrade to the published beta.3:

   ```bash
   node cli/myapi.mjs upgrade --project-dir ./upgrade-copy --version v0.2.0-beta.3 --dry-run --json
   ```

4. Follow the [upgrade rehearsal and recovery guide](docs/UPGRADE_REHEARSAL.md) for the actual change. The CLI's `.env` backup is **not a database backup**. Once target startup has been attempted, it does not automatically restart the old image: the database may already have migrated. Restore a verified pre-upgrade database before starting an incompatible older binary.

Do not delete data volumes, reset unresolved accounting records, or point an older binary at a migrated database as a shortcut to recovery. Existing databases and access settings must be preserved when adopting or moving an installation.

## Security and responsible use

- Keep the initial loopback binding; expose only the intended HTTPS proxy. Review trusted proxies, registration, user roles, and API-key access before sharing the instance.
- Use only accounts and APIs you are authorized to access, in accordance with provider terms and applicable law. Public services and resale may require additional compliance measures; installing this project does not satisfy those obligations.
- The shipped deployment template enables full-content logging. The example above deliberately disables it. If you enable it, review [content logging](docs/FULL_CONTENT_LOGGING_CUSTOM.md), access permissions, retention, and backups. Redaction cannot guarantee that prompts or outputs contain no personal or confidential data.
- Quota sampling may contact upstream services in the background. Review monitoring settings and supported-provider boundaries before adding real credentials.
- Keep secrets, OAuth files, databases, and private logs out of Git, screenshots, issue reports, and shared archives. Redact diagnostics before reporting a problem.

## Documentation and help

- [Published beta.3 artifacts and evidence](docs/RELEASE_BETA_3.md)
- [Deployment configuration](DEPLOYMENT_CUSTOM.md) · [Legacy LAN](docs/LAN_LITE.md)
- [Upgrade and recovery](docs/UPGRADE_REHEARSAL.md) · [Installation acceptance record](docs/R1_INSTALLATION_CHECK.md)
- [Relay API](docs/openapi/relay.json) · [Management API](docs/openapi/api.json)
- [Usage and quota analytics](docs/QUOTA_ANALYTICS.md) · [Claude organization usage limits](docs/CLAUDE_USAGE_REPORT.md)
- [Authentication](docs/authentication.md) · [Content logging](docs/FULL_CONTENT_LOGGING_CUSTOM.md)
- [Issue tracker](https://github.com/ForceMind/MyAPI/issues): include version/revision, deployment type, sanitized reproduction steps, and expected/actual behavior

Contributors: see the [development plan and implementation records](docs/MYAPI_MASTER_PLAN.md). Historical development notes are separate from these installation instructions.

## License and credits

My API is a modified distribution of upstream open-source work, with distribution changes by ForceMind. It is licensed under [GNU AGPLv3](LICENSE); see [NOTICE](NOTICE) for upstream authorship and the additional Section 7 attribution requirements, including the required visible frontend credit and original-project link in modified user interfaces. Preserve those notices and mark modifications; branding changes do not remove these obligations. Network use of modified versions may also trigger corresponding-source obligations under the license.

[Third-party licenses](THIRD-PARTY-LICENSES.md) document dependency credits. Preserve applicable notices in redistributed images, binaries, frontend bundles, and desktop packages, including Electron/Chromium notices where relevant. Review the full license terms before use or redistribution.
