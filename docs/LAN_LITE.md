# MyAPI LAN Lite

LAN Lite runs MyAPI as a small, private-network API gateway on a macOS or Windows workstation. Colleagues call the workstation with their own MyAPI downstream API Keys; upstream credentials remain configured in MyAPI and are never read by the CLI or printed to the terminal.

## What it is (and is not)

LAN Lite is a single-process, SQLite-first deployment for a trusted local network. It is not a public hosting mode, a credential-file importer, or an automatic firewall configurator. The default listener is loopback-only.

The CLI never scans `~/.codex`, Claude credential files, Keychain, Windows Credential Manager, or any other local credential store. Configure an upstream channel explicitly in the MyAPI administrator interface or point it at an already-authorized local API endpoint.

## Docker Desktop (recommended on macOS and Windows)

Install Docker Desktop, then run the CLI from the MyAPI source distribution:

```bash
myapi lan init ./myapi-lan
myapi lan start --project-dir ./myapi-lan
```

The release image is in the private MyAPI GHCR package. If Docker Desktop is
not already authenticated, sign in once with a GitHub token that can read the
package (`docker login ghcr.io`) before starting the service.

The initializer creates a private `deploy/.env`, a random session secret, SQLite/data directories, and the LAN image configuration. It does not start Docker or change host firewall rules.

On Windows PowerShell the same commands work with a Windows path, for example
`myapi lan init "$env:LOCALAPPDATA\\MyAPI\\lan-project"`.

To share deliberately on a private IPv4 network, opt in and bind to the workstation's private address:

```bash
myapi lan start \
  --project-dir ./myapi-lan \
  --bind-address 192.168.1.20 \
  --port 3000 \
  --allow-lan
```

`--allow-lan` is required whenever the listener is not loopback. Public addresses are rejected. `0.0.0.0` is accepted only with `--allow-lan`; prefer the workstation's concrete private address when possible.

Check the configured endpoint without exposing secrets:

```bash
myapi lan status --project-dir ./myapi-lan
myapi lan stop --project-dir ./myapi-lan
```

The status output only shows the listener and deployment state. It never prints `SESSION_SECRET`, upstream keys, downstream keys, OAuth JSON, cookies, or JWTs.

## Platform data locations

Relative `./data` and `./logs` paths are portable and are the default for a source workspace. If you choose platform-specific host paths, use a directory owned by the current user:

- macOS: `~/Library/Application Support/MyAPI/lan`
- Windows: `%LOCALAPPDATA%\\MyAPI\\lan`

Docker Desktop must be allowed to share the selected directory. Do not mount an entire home directory or a credential directory into the container.

## Colleague access

1. Open the administrator UI at the displayed local URL.
2. Create one downstream API Key per colleague.
3. Give each person only the models, rate limit, and quota they need.
4. Revoke or rotate a key independently when access ends.

Colleagues receive the LAN URL and their own MyAPI key. They do not receive the upstream provider key, administrator password, local files, or another colleague's logs. Do not paste keys into chat, source control, screenshots, or issue reports.

## Stop, update, and recovery

`myapi lan stop` stops the LAN compose project without removing its data volumes. An update is explicit:

```bash
myapi lan stop --project-dir ./myapi-lan
# update the pinned MYAPI_IMAGE in deploy/.env when you are ready
myapi lan start --project-dir ./myapi-lan
```

GitHub Actions builds and signs the `ghcr.io/forcemind/myapi-lan:<version>` image, but does not restart your workstation. Keep the image pinned to a version and retain the previous image until the new health check succeeds.

## Security checklist

- Keep the listener on loopback unless a private-network share is intentional.
- Prefer a concrete private IP over `0.0.0.0`.
- Use a separate downstream key for every colleague.
- Restrict model access, quota, concurrency, and rate limits.
- Keep Docker Desktop and the host OS updated.
- Use a VPN/Tailscale or an authenticated reverse proxy for networks you do not fully trust.
- Never expose the LAN listener directly to the public Internet.
- Never put `deploy/.env`, SQLite files, logs, or API keys in Git.
