# MyAPI Electron Desktop App

This directory contains the Electron wrapper for MyAPI, providing a native desktop application with system tray support for Windows, macOS, and Linux.

## Prerequisites

### 1. Go Binary (Required)
The Electron app requires the compiled Go binary to function. You have two options:

**Option A: Use existing binary (without Go installed)**
```bash
# If you have a pre-built binary (e.g., my-api-macos)
cp ../my-api-macos ../my-api
```

**Option B: Build from source (requires Go)**
From the repository root, build the frontend and the platform-specific
backend/package with the bounded helper (the default is two parallel workers):

```bash
cd electron
MYAPI_BUILD_PARALLELISM=2 ./build.sh
```

On Windows, run the same script from Git Bash (or build the Go binary with
`go build -o my-api.exe` and then run `npm run build:win`). The helper never
starts Docker or publishes artifacts; it only writes `my-api[.exe]` and the
installer files under `electron/dist/`.

### 2. Electron Dependencies
```bash
cd electron
npm install
```

## Development

Start the backend, the frontend, and Electron in separate terminals:
```bash
# Repository root
go run main.go

# Repository root
make dev-web

# electron/
npm run dev-app
```

This will:
- Use the Go backend on port 3000
- Use the Rsbuild frontend development server on port 5173
- Open an Electron window with DevTools enabled
- Create a system tray icon (menu bar on macOS)
- Store database in `../data/my-api.db` (existing `new-api.db`/`one-api.db` files are adopted automatically)

## Building for Production

The helper limits Go/npm build parallelism to two workers by default so a
desktop build does not monopolize a shared workstation. Set
`MYAPI_BUILD_PARALLELISM` to a positive integer for a dedicated build host.

### Quick Build
```bash
# From electron/, build the frontend, Go binary, and desktop package
./build.sh

# Or package an existing binary for the current platform
npm run build

# Platform-specific builds
npm run build:mac    # Creates .dmg and .zip
npm run build:win    # Creates .exe installer
npm run build:linux  # Creates .AppImage and .deb
```

### Build Output
- Built applications are in `electron/dist/`
- macOS: `.dmg` (installer) and `.zip` (portable)
- Windows: `.exe` (installer) and portable exe
- Linux: `.AppImage` and `.deb`

Before a platform owner runs a build, the repository-level contract check can
be run without Electron or Go:

```bash
npm run desktop:check
```

The check covers the macOS/Windows targets, bundled native binary and license
resources, versioned tag validation, SHA256 checksum generation, and explicit
release approval gates in GitHub Actions. It is deterministic and does not
start Docker, contact GHCR, or publish artifacts. Installer launch, firewall
prompt, and LAN request acceptance still require real macOS and Windows hosts.

The current workflow produces unsigned desktop artifacts unless the release
environment supplies platform signing credentials. macOS users may see
Gatekeeper warnings (DMG/ZIP), and Windows users may see SmartScreen warnings
for NSIS/portable packages. Apple Developer ID signing/notarization and
Windows code-signing certificates are external release prerequisites; do not
treat a successful build or checksum as proof that an installer is trusted.

## Configuration

### Port
Default port is 3000. Pass `--port 4317` when launching the desktop app to
override it. The process binds to `127.0.0.1` by default.

To share with colleagues, explicitly pass a private bind address and opt in:

```text
MyAPI --allow-lan --bind-address 192.168.1.20 --port 4317
```

Public addresses are rejected, and `--allow-lan` is required for any
non-loopback listener. The tray menu shows the effective endpoint. The desktop
process runs the LAN edition and never imports local Codex, Claude, or other
provider credential files.

Use the tray menu item **LAN status and connection help…** to see the effective
endpoint, whether this process is loopback-only or LAN-enabled, and the
platform-specific firewall hint. This is a read-only status view: the listener
is fixed when Electron starts. To change the bind address, port, or LAN opt-in,
quit MyAPI and relaunch it with the new arguments; the tray view never performs
an unsafe live rebind.

Before a server process is spawned, Electron performs a platform-independent
configuration preflight. Invalid/public addresses fail with a visible error;
the default remains `127.0.0.1:3000`. The checks are covered by
`npm test` in this directory and intentionally do not inspect or import any
local provider credential files.

### Database Location
- **Development**: `../data/my-api.db` (project directory; legacy database names are read for migration)
- **Production**:
  - macOS: `~/Library/Application Support/MyAPI/data/`
  - Windows: `%APPDATA%/MyAPI/data/`
  - Linux: `~/.config/MyAPI/data/`
