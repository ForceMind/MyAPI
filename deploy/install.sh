#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "$script_dir/.." && pwd)
env_file="$script_dir/.env"

if [[ ! -f "$env_file" ]]; then
  cp "$script_dir/.env.example" "$env_file"
  echo "Created $env_file. Set SESSION_SECRET and NEW_API_PUBLIC_URL, then run this script again." >&2
  exit 1
fi

set -a
source "$env_file"
set +a

if [[ -z "${SESSION_SECRET:-}" || "$SESSION_SECRET" == "replace-with-a-long-random-secret" ]]; then
  echo "Set a strong SESSION_SECRET in $env_file before deployment." >&2
  exit 1
fi

if [[ -z "${NEW_API_PUBLIC_URL:-}" || "$NEW_API_PUBLIC_URL" == "https://new-api.example.com" ]]; then
  echo "Set NEW_API_PUBLIC_URL in $env_file before deployment." >&2
  exit 1
fi

mkdir -p "$script_dir/data" "$script_dir/logs"

cd "$repo_dir"
docker build \
  --build-arg "MYAPI_BRAND_NAME=${MYAPI_BRAND_NAME:-MyAPI}" \
  --build-arg "MYAPI_BRAND_LOGO=${MYAPI_BRAND_LOGO:-/myapi-logo-v1.png}" \
  -t "${NEW_API_IMAGE:-local/new-api:custom-rc25}" .
docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" up -d --force-recreate
docker compose --env-file "$env_file" -f "$script_dir/docker-compose.yml" ps
