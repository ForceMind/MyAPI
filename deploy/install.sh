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

# Read KEY=VALUE entries without evaluating the deployment file as shell code.
# Values may be quoted, but command substitutions, functions, and other shell
# syntax are never executed. Only deployment keys are exported to this shell;
# Docker Compose still receives the original file through --env-file below.
# Keeping an allowlist here is important because exporting arbitrary names from
# a user-controlled `.env` could replace PATH/loader settings for the helper
# commands that follow (or alter shell startup behaviour via BASH_ENV).
is_allowed_env_key() {
  case "$1" in
    MYAPI_*|CHANNEL_QUOTA_*|FULL_CONTENT_LOG_*|SESSION_SECRET|TZ|ERROR_LOG_ENABLED|BATCH_UPDATE_ENABLED|TRUSTED_PROXIES)
      return 0
      ;;
    NEW_API_IMAGE|NEW_API_PORT|NEW_API_PUBLIC_URL|NEW_API_DATA_DIR|NEW_API_LOGS_DIR)
      # Legacy deployment aliases remain accepted for one-time migration.
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

load_env_file() {
  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ -z "${line//[[:space:]]/}" || "$line" =~ ^[[:space:]]*# ]] && continue
    if [[ ! "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      echo "Invalid deployment env line; expected KEY=VALUE." >&2
      exit 1
    fi
    key="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    if ! is_allowed_env_key "$key"; then
      echo "Ignoring unknown deployment env key '$key'." >&2
      continue
    fi
    if [[ "$value" == \"*\" && "$value" == *\" ]]; then
      value="${value:1:${#value}-2}"
    elif [[ "$value" == \'*\' && "$value" == *\' ]]; then
      value="${value:1:${#value}-2}"
    fi
    printf -v "$key" '%s' "$value"
    export "$key"
  done < "$env_file"
}
load_env_file

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

distribution_version="$(tr -d '[:space:]' < "$repo_dir/VERSION" 2>/dev/null || true)"
if [[ ! "$distribution_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "VERSION must contain a valid MyAPI release version before installing." >&2
  exit 1
fi
MYAPI_IMAGE="${MYAPI_IMAGE:-ghcr.io/forcemind/myapi:v${distribution_version}}"
MYAPI_EDITION="${MYAPI_EDITION:-full}"
MYAPI_BUILD_LOCAL="${MYAPI_BUILD_LOCAL:-false}"
MYAPI_PORT="${MYAPI_PORT:-3000}"
MYAPI_BIND_ADDRESS="${MYAPI_BIND_ADDRESS:-127.0.0.1}"
MYAPI_ALLOW_LAN="${MYAPI_ALLOW_LAN:-false}"
MYAPI_SESSION_COOKIE_SECURE="${MYAPI_SESSION_COOKIE_SECURE:-}"
MYAPI_PUBLIC_URL="${MYAPI_PUBLIC_URL:-}"
MYAPI_DATA_DIR="${MYAPI_DATA_DIR:-./data}"
MYAPI_LOGS_DIR="${MYAPI_LOGS_DIR:-./logs}"
MYAPI_CPU_LIMIT="${MYAPI_CPU_LIMIT:-2.0}"
MYAPI_MEMORY_LIMIT="${MYAPI_MEMORY_LIMIT:-2g}"
MYAPI_BUILD_PARALLELISM="${MYAPI_BUILD_PARALLELISM:-2}"
if [[ "$MYAPI_EDITION" != "full" && "$MYAPI_EDITION" != "lan" ]]; then
  echo "MYAPI_EDITION must be full or lan." >&2
  exit 1
fi
if [[ "$MYAPI_EDITION" == "lan" && "$MYAPI_IMAGE" == "ghcr.io/forcemind/myapi:"* ]]; then
  MYAPI_IMAGE="${MYAPI_IMAGE/ghcr.io\/forcemind\/myapi:/ghcr.io\/forcemind\/myapi-lan:}"
fi
if [[ "$MYAPI_EDITION" == "lan" ]]; then
  MYAPI_SESSION_COOKIE_SECURE="${MYAPI_SESSION_COOKIE_SECURE:-false}"
else
  MYAPI_SESSION_COOKIE_SECURE="${MYAPI_SESSION_COOKIE_SECURE:-true}"
fi
export MYAPI_IMAGE MYAPI_EDITION MYAPI_BUILD_LOCAL MYAPI_PORT MYAPI_BIND_ADDRESS MYAPI_ALLOW_LAN MYAPI_SESSION_COOKIE_SECURE MYAPI_PUBLIC_URL MYAPI_DATA_DIR MYAPI_LOGS_DIR MYAPI_CPU_LIMIT MYAPI_MEMORY_LIMIT MYAPI_BUILD_PARALLELISM

is_loopback_bind_address() {
  [[ "$1" == "localhost" || "$1" == "127.0.0.1" ]]
}

is_private_ipv4() {
  local address="$1" a b c d extra
  IFS=. read -r a b c d extra <<< "$address"
  [[ -z "${extra:-}" && "$a" =~ ^[0-9]+$ && "$b" =~ ^[0-9]+$ && "$c" =~ ^[0-9]+$ && "$d" =~ ^[0-9]+$ ]] || return 1
  (( a >= 0 && a <= 255 && b >= 0 && b <= 255 && c >= 0 && c <= 255 && d >= 0 && d <= 255 )) || return 1
  (( a == 10 || (a == 172 && b >= 16 && b <= 31) || (a == 192 && b == 168) ))
}

is_valid_lan_origin() {
  local value="$1" host port
  if [[ ! "$value" =~ ^https?://([^/:?#]+)(:([0-9]+))?/?$ ]]; then
    return 1
  fi
  host="${BASH_REMATCH[1],,}"
  port="${BASH_REMATCH[3]:-}"
  if [[ -n "$port" ]] && (( port < 1 || port > 65535 )); then
    return 1
  fi
  is_loopback_bind_address "$host" || is_private_ipv4 "$host"
}

is_valid_public_origin() {
  local value="$1" host port
  if [[ ! "$value" =~ ^https://([^/:?#]+)(:([0-9]+))?/?$ ]]; then
    return 1
  fi
  host="${BASH_REMATCH[1],,}"
  port="${BASH_REMATCH[3]:-}"
  if [[ -n "$port" ]] && (( port < 1 || port > 65535 )); then
    return 1
  fi
  [[ "$host" != "localhost" && "$host" != *.localhost && "$host" != "example.com" && "$host" != *.example.com ]]
}

if [[ "$MYAPI_ALLOW_LAN" != "true" && "$MYAPI_ALLOW_LAN" != "false" ]]; then
  echo "MYAPI_ALLOW_LAN must be true or false." >&2
  exit 1
fi
if ! is_loopback_bind_address "${MYAPI_BIND_ADDRESS,,}" && [[ "$MYAPI_BIND_ADDRESS" != "0.0.0.0" ]] && ! is_private_ipv4 "$MYAPI_BIND_ADDRESS"; then
  echo "MYAPI_BIND_ADDRESS must be localhost, 127.0.0.1, 0.0.0.0, or a private IPv4 address." >&2
  exit 1
fi
if ! is_loopback_bind_address "${MYAPI_BIND_ADDRESS,,}" && [[ "$MYAPI_ALLOW_LAN" != "true" ]]; then
  echo "LAN binding is disabled by default; set MYAPI_ALLOW_LAN=true to share on a private network." >&2
  exit 1
fi

if [[ ! "$MYAPI_BUILD_PARALLELISM" =~ ^[1-9][0-9]*$ || "$MYAPI_BUILD_PARALLELISM" -gt 64 ]]; then
  echo "MYAPI_BUILD_PARALLELISM must be a positive integer no more than 64." >&2
  exit 1
fi
if [[ ! "$MYAPI_CPU_LIMIT" =~ ^[0-9]+([.][0-9]+)?$ ]] || (( $(awk "BEGIN { print ($MYAPI_CPU_LIMIT <= 0 || $MYAPI_CPU_LIMIT > 64) }") )); then
  echo "MYAPI_CPU_LIMIT must be a number greater than 0 and no more than 64." >&2
  exit 1
fi
if [[ ! "$MYAPI_MEMORY_LIMIT" =~ ^[0-9]+([.][0-9]+)?([bBkKmMgGtT][bB]?)$ ]] || [[ "$MYAPI_MEMORY_LIMIT" =~ ^0([.]0+)? ]]; then
  echo "MYAPI_MEMORY_LIMIT must be a positive Docker size such as 512m or 2g." >&2
  exit 1
fi

if [[ -z "${SESSION_SECRET:-}" || "$SESSION_SECRET" == "replace-with-a-long-random-secret" ]]; then
  echo "Set a strong SESSION_SECRET in $env_file before deployment." >&2
  exit 1
fi
if [[ "${#SESSION_SECRET}" -lt 48 ]]; then
  echo "SESSION_SECRET must be at least 48 characters before deployment." >&2
  exit 1
fi

if [[ "$MYAPI_EDITION" == "full" && ( -z "${MYAPI_PUBLIC_URL:-}" || "$MYAPI_PUBLIC_URL" == "https://my-api.example.com" || "$MYAPI_PUBLIC_URL" == "https://new-api.example.com" ) ]]; then
  echo "Set MYAPI_PUBLIC_URL in $env_file before deployment." >&2
  exit 1
fi
if [[ "$MYAPI_EDITION" == "full" ]] && ! is_valid_public_origin "$MYAPI_PUBLIC_URL"; then
  echo "MYAPI_PUBLIC_URL must be an exact non-placeholder HTTPS origin in full edition." >&2
  exit 1
fi
if [[ "$MYAPI_EDITION" == "lan" && ( -z "${MYAPI_PUBLIC_URL:-}" || "$MYAPI_PUBLIC_URL" == "https://my-api.example.com" || "$MYAPI_PUBLIC_URL" == "https://new-api.example.com" ) ]]; then
  MYAPI_PUBLIC_URL="http://localhost:${MYAPI_PORT}"
  export MYAPI_PUBLIC_URL
fi
if [[ "$MYAPI_EDITION" == "lan" ]]; then
  if ! is_valid_lan_origin "$MYAPI_PUBLIC_URL"; then
    echo "MYAPI_PUBLIC_URL must be a localhost, loopback, or private-network origin in LAN edition." >&2
    exit 1
  fi
fi
if [[ "$MYAPI_EDITION" == "lan" && "$MYAPI_PUBLIC_URL" == http://* && "$MYAPI_SESSION_COOKIE_SECURE" == "true" ]]; then
  # The example file targets the HTTPS full edition. LAN's default local
  # origin is HTTP, so avoid issuing cookies browsers will never send.
  MYAPI_SESSION_COOKIE_SECURE=false
  export MYAPI_SESSION_COOKIE_SECURE
fi

resolve_deploy_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$script_dir" "$1" ;;
  esac
}

mkdir -p "$(resolve_deploy_path "$MYAPI_DATA_DIR")" "$(resolve_deploy_path "$MYAPI_LOGS_DIR")"

cd "$repo_dir"
# Validate interpolation (including resource limits and required origins)
# before pulling any remote image or changing a running container.
docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" config --quiet
if [[ "$MYAPI_BUILD_LOCAL" == "true" || "$MYAPI_IMAGE" == local/* ]]; then
  cpu_quota=$(awk "BEGIN { printf \"%d\", ($MYAPI_CPU_LIMIT * 100000) }")
  docker build \
    --cpu-period 100000 \
    --cpu-quota "${cpu_quota}" \
    --memory "${MYAPI_MEMORY_LIMIT}" \
    --memory-swap "${MYAPI_MEMORY_LIMIT}" \
    --build-arg "MYAPI_BRAND_NAME=${MYAPI_BRAND_NAME:-MyAPI}" \
    --build-arg "MYAPI_BRAND_LOGO=${MYAPI_BRAND_LOGO:-/myapi-logo-v1.png}" \
    --build-arg "MYAPI_EDITION=${MYAPI_EDITION}" \
    --build-arg "MYAPI_BUILD_PARALLELISM=${MYAPI_BUILD_PARALLELISM:-2}" \
    -t "$MYAPI_IMAGE" .
else
  echo "Pulling MyAPI image from ${MYAPI_IMAGE}." >&2
  docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" pull my-api
fi
# Wait for the healthcheck before reporting success. Docker Desktop on macOS
# and Windows may take longer to initialize than a native Linux daemon; the
# bounded timeout avoids an indefinite busy wait while still catching a bad
# image/configuration before the operator starts handing out LAN keys.
docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" up -d --force-recreate --wait --wait-timeout 120
docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" ps
