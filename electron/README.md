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
TODO

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
