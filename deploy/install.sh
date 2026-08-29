#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "$script_dir/.." && pwd)
env_file="$script_dir/.env"

if [[ ! -f "$env_file" ]]; then
  cp "$script_dir/.env.example" "$env_file"
  echo "Created $env_file. Set SESSION_SECRET and MYAPI_PUBLIC_URL, then run this script again." >&2
  exit 1
fi

set -a
source "$env_file"
set +a

legacy_keys=()
if [[ -z "${MYAPI_IMAGE:-}" && -n "${NEW_API_IMAGE:-}" ]]; then
  MYAPI_IMAGE="$NEW_API_IMAGE"
  legacy_keys+=(NEW_API_IMAGE)
fi
if [[ -z "${MYAPI_PORT:-}" && -n "${NEW_API_PORT:-}" ]]; then
  MYAPI_PORT="$NEW_API_PORT"
  legacy_keys+=(NEW_API_PORT)
fi
if [[ -z "${MYAPI_PUBLIC_URL:-}" && -n "${NEW_API_PUBLIC_URL:-}" ]]; then
  MYAPI_PUBLIC_URL="$NEW_API_PUBLIC_URL"
  legacy_keys+=(NEW_API_PUBLIC_URL)
fi
if [[ -z "${MYAPI_DATA_DIR:-}" && -n "${NEW_API_DATA_DIR:-}" ]]; then
  MYAPI_DATA_DIR="$NEW_API_DATA_DIR"
  legacy_keys+=(NEW_API_DATA_DIR)
fi
if [[ -z "${MYAPI_LOGS_DIR:-}" && -n "${NEW_API_LOGS_DIR:-}" ]]; then
  MYAPI_LOGS_DIR="$NEW_API_LOGS_DIR"
  legacy_keys+=(NEW_API_LOGS_DIR)
fi

if (( ${#legacy_keys[@]} > 0 )); then
  echo "Compatibility fallback for ${legacy_keys[*]}; run 'myapi migrate --project-dir $repo_dir' to write MYAPI_* settings." >&2
fi

MYAPI_IMAGE="${MYAPI_IMAGE:-local/my-api:custom-rc25}"
MYAPI_PORT="${MYAPI_PORT:-3000}"
MYAPI_PUBLIC_URL="${MYAPI_PUBLIC_URL:-}"
MYAPI_DATA_DIR="${MYAPI_DATA_DIR:-./data}"
MYAPI_LOGS_DIR="${MYAPI_LOGS_DIR:-./logs}"
export MYAPI_IMAGE MYAPI_PORT MYAPI_PUBLIC_URL MYAPI_DATA_DIR MYAPI_LOGS_DIR

if [[ -z "${SESSION_SECRET:-}" || "$SESSION_SECRET" == "replace-with-a-long-random-secret" ]]; then
  echo "Set a strong SESSION_SECRET in $env_file before deployment." >&2
  exit 1
fi

if [[ -z "${MYAPI_PUBLIC_URL:-}" || "$MYAPI_PUBLIC_URL" == "https://my-api.example.com" || "$MYAPI_PUBLIC_URL" == "https://new-api.example.com" ]]; then
  echo "Set MYAPI_PUBLIC_URL in $env_file before deployment." >&2
  exit 1
fi

resolve_deploy_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$script_dir" "$1" ;;
  esac
}

mkdir -p "$(resolve_deploy_path "$MYAPI_DATA_DIR")" "$(resolve_deploy_path "$MYAPI_LOGS_DIR")"

cd "$repo_dir"
docker build \
  --build-arg "MYAPI_BRAND_NAME=${MYAPI_BRAND_NAME:-MyAPI}" \
  --build-arg "MYAPI_BRAND_LOGO=${MYAPI_BRAND_LOGO:-/myapi-logo-v1.png}" \
  -t "$MYAPI_IMAGE" .
docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" up -d --force-recreate
docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" ps
