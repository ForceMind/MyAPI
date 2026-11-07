#!/bin/bash

set -e

# Keep local desktop builds friendly to shared workstations. Override with a
# positive integer when a dedicated build host has more capacity.
BUILD_PARALLELISM="${MYAPI_BUILD_PARALLELISM:-2}"
if [[ ! "$BUILD_PARALLELISM" =~ ^[1-9][0-9]*$ ]]; then
    echo "MYAPI_BUILD_PARALLELISM must be a positive integer" >&2
    exit 1
fi
export npm_config_jobs="$BUILD_PARALLELISM"
export GOMAXPROCS="$BUILD_PARALLELISM"

echo "Building MyAPI Electron App..."

DESKTOP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DESKTOP_DIR/.."
MYAPI_DESKTOP_VERSION="$(tr -d '[:space:]' < VERSION)"
MYAPI_DESKTOP_BUILD_ID="$(git -c core.fileMode=false rev-parse HEAD)"
ROOT_PACKAGE_VERSION="$(node -p 'JSON.parse(require("fs").readFileSync("package.json", "utf8")).version')"
if [[ "$MYAPI_DESKTOP_VERSION" != "$ROOT_PACKAGE_VERSION" || ! "$MYAPI_DESKTOP_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
    echo "Desktop VERSION and root package.json must match a valid version" >&2
    exit 1
fi

echo "Step 1: Building frontend..."
cd "$DESKTOP_DIR/../web"
bun install --frozen-lockfile
DISABLE_ESLINT_PLUGIN='true' VITE_SELF_USE_MINIMAL='true' VITE_BRAND_NAME='MyAPI' VITE_BRAND_LOGO='/myapi-logo-v1.png' VITE_REACT_APP_VERSION="$MYAPI_DESKTOP_VERSION" VITE_BUILD_ID="$MYAPI_DESKTOP_BUILD_ID" bun run build
cd "$DESKTOP_DIR"

echo "Step 2: Building Go backend..."
cd ..
MODULE_PATH="$(go list -m)"
[[ "$MODULE_PATH" == "github.com/ForceMind/MyAPI" ]] || { echo "unexpected Go module path" >&2; exit 1; }

if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Building for macOS..."
    CGO_ENABLED=1 GOMAXPROCS="$BUILD_PARALLELISM" go build -p "$BUILD_PARALLELISM" -ldflags="-s -w -X '${MODULE_PATH}/common.Version=${MYAPI_DESKTOP_VERSION}'" -o my-api
    cd electron
    npm ci
    npm run build:mac -- --config.extraMetadata.version="$MYAPI_DESKTOP_VERSION"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Building for Linux..."
    CGO_ENABLED=1 GOMAXPROCS="$BUILD_PARALLELISM" go build -p "$BUILD_PARALLELISM" -ldflags="-s -w -X '${MODULE_PATH}/common.Version=${MYAPI_DESKTOP_VERSION}'" -o my-api
    cd electron
    npm ci
    npm run build:linux -- --config.extraMetadata.version="$MYAPI_DESKTOP_VERSION"
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
    echo "Building for Windows..."
    CGO_ENABLED=1 GOMAXPROCS="$BUILD_PARALLELISM" go build -p "$BUILD_PARALLELISM" -ldflags="-s -w -X '${MODULE_PATH}/common.Version=${MYAPI_DESKTOP_VERSION}'" -o my-api.exe
    cd electron
    npm ci
    npm run build:win -- --config.extraMetadata.version="$MYAPI_DESKTOP_VERSION"
else
    echo "Unknown OS, building for current platform..."
    CGO_ENABLED=1 GOMAXPROCS="$BUILD_PARALLELISM" go build -p "$BUILD_PARALLELISM" -ldflags="-s -w -X '${MODULE_PATH}/common.Version=${MYAPI_DESKTOP_VERSION}'" -o my-api
    cd electron
    npm ci
    npm run build -- --config.extraMetadata.version="$MYAPI_DESKTOP_VERSION"
fi

echo "Build complete! Check electron/dist/ for output."
