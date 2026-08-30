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

echo "Building MyAPI Electron App..."

echo "Step 1: Building frontend..."
cd ../web
bun install --frozen-lockfile
DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(git describe --tags --always) bun run build
cd ../electron

echo "Step 2: Building Go backend..."
cd ..

if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Building for macOS..."
    CGO_ENABLED=1 go build -p "$BUILD_PARALLELISM" -ldflags="-s -w" -o my-api
    cd electron
    npm install
    npm run build:mac
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Building for Linux..."
    CGO_ENABLED=1 go build -p "$BUILD_PARALLELISM" -ldflags="-s -w" -o my-api
    cd electron
    npm install
    npm run build:linux
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
    echo "Building for Windows..."
    CGO_ENABLED=1 go build -p "$BUILD_PARALLELISM" -ldflags="-s -w" -o my-api.exe
    cd electron
    npm install
    npm run build:win
else
    echo "Unknown OS, building for current platform..."
    CGO_ENABLED=1 go build -p "$BUILD_PARALLELISM" -ldflags="-s -w" -o my-api
    cd electron
    npm install
    npm run build
fi

echo "Build complete! Check electron/dist/ for output."
