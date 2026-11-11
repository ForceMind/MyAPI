#!/usr/bin/env bash
# Read-only service preview and local package verification.
set -euo pipefail
export LC_ALL=C

fail() {
  printf '%s\n' "$1" >&2
  exit 2
}

print_help() {
  printf '%s\n' \
    'MyAPI Linux service preview / MyAPI Linux 原生服务预览' \
    'Usage / 用法:' \
    '  bash deploy/install-native.sh --help' \
    '  bash deploy/install-native.sh plan --scope system|user' \
    '  bash deploy/install-native.sh render-unit --scope system|user' \
    '  bash deploy/install-native.sh verify-package --scope system|user --edition full|lite --tag TAG' \
    '    --manifest FILE --bundle FILE --package FILE --cosign FILE --root FILE' \
    '' \
    'Read-only; no installation, configuration, or service state is inspected.' \
    '仅预览，不检查现有安装、配置或服务状态，不可据此接管或安装。' \
    'Use plan to review target folders; render-unit prints a service file for review.' \
    'plan 查看目标目录；render-unit 输出服务文件，供审阅。' \
    'Install, upgrade, rollback, and uninstall are unavailable in B3a.' \
    'verify-package 核验本地包。安装、升级、回滚和卸载暂未开放。'
}

validated_base() {
  local label="$1"
  local value="$2"
  local c1_controls=$'\302[\200-\237]'
  if [[ -z "$value" || "$value" != /* || "$value" =~ [[:cntrl:]] || "$value" =~ $c1_controls ]]; then
    fail "$label must be a non-empty absolute path without control characters / 路径须非空、绝对且不含控制字符。"
  fi
  case "/$value/" in
    */./*|*/../*) fail "$label must not contain dot path segments / 路径不得含 . 或 .. 段。" ;;
  esac
  while [[ "$value" != / && "$value" == */ ]]; do
    value="${value%/}"
  done
  printf '%s' "$value"
}

# systemd has its own quoting, specifier and ExecStart variable expansion.
# These values are path data, never shell commands or evaluated expressions.
unit_quote() {
  local value="$1"
  local context="${2:-value}"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//%/%%}"
  if [[ "$context" == exec ]]; then
    value="${value//\$/\$\$}"
  fi
  printf '"%s"' "$value"
}

# These path fields keep the complete value, without word unquoting. Only
# EnvironmentFile subsequently passes it through glob(), so escape that layer.
unit_path() {
  local value="$1"
  local field="$2"
  case "$field" in
    directory) ;;
    env-file)
      value="${value//\\/\\\\}"
      value="${value//\*/\\*}"
      value="${value//\?/\\?}"
      value="${value//\[/\\[}"
      value="${value//\]/\\]}"
      ;;
    *) fail 'Unknown internal unit path field / 未知的内部服务路径字段。' ;;
  esac
  value="${value//\%/%%}"
  printf '%s' "$value"
}

if [[ "$#" -eq 1 && "$1" == --help ]]; then
  print_help
  exit 0
fi

[[ "$#" -ge 1 ]] || fail 'A command and explicit --scope are required / 必须指定命令和 --scope。'
command_name="$1"
shift
case "$command_name" in
  verify-package)
    script_directory="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
    exec node "$script_directory/../cli/lib/native-package-verification.mjs" "$@"
    ;;
  plan|render-unit) ;;
  install|upgrade|rollback|uninstall)
    fail 'Unavailable in B3a: no installation or lifecycle executor is connected; nothing was changed / 本阶段未接线安装或生命周期执行器，未作任何更改。'
    ;;
  *) fail 'Unknown command; use --help / 未知命令，请使用 --help。' ;;
esac
[[ "$#" -eq 2 && "$1" == --scope ]] || fail 'Use exactly --scope system or --scope user / 必须明确且仅指定一次 --scope system 或 --scope user。'
scope="$2"
case "$scope" in
  system)
    account=my-api
    binary=/opt/my-api/current/my-api
    releases=/opt/my-api/releases
    config=/etc/my-api/my-api.env
    data=/var/lib/my-api/data
    state=/var/lib/my-api/state
    backups=/var/lib/my-api/backups
    logs=/var/log/my-api
    cache=/var/cache/my-api
    unit=/etc/systemd/system/my-api.service
    ;;
  user)
    native_home="$(validated_base HOME "${HOME-}")"
    native_data_home="$(validated_base XDG_DATA_HOME "${XDG_DATA_HOME-"${native_home%/}/.local/share"}")"
    native_config_home="$(validated_base XDG_CONFIG_HOME "${XDG_CONFIG_HOME-"${native_home%/}/.config"}")"
    native_state_home="$(validated_base XDG_STATE_HOME "${XDG_STATE_HOME-"${native_home%/}/.local/state"}")"
    native_cache_home="$(validated_base XDG_CACHE_HOME "${XDG_CACHE_HOME-"${native_home%/}/.cache"}")"
    account=current-user
    binary="${native_data_home%/}/my-api/current/my-api"
    releases="${native_data_home%/}/my-api/releases"
    config="${native_config_home%/}/my-api/my-api.env"
    data="${native_data_home%/}/my-api/data"
    state="${native_state_home%/}/my-api/state"
    backups="${native_state_home%/}/my-api/backups"
    logs="${native_state_home%/}/my-api/logs"
    cache="${native_cache_home%/}/my-api"
    unit="${native_config_home%/}/systemd/user/my-api.service"
    ;;
  *) fail 'Invalid scope; use system or user / scope 必须为 system 或 user。' ;;
esac

if [[ "$command_name" == plan ]]; then
  printf '%s\n' \
    'MyAPI Linux service preview / MyAPI Linux 原生服务预览' \
    'Preview only. Existing installation, configuration and service state have not been inspected.' \
    '仅预览；尚未检查现有安装、配置或服务状态，不可据此接管或安装。'
  printf 'scope: %s\naccount: %s\nbinary: %s\nreleases: %s\nconfig: %s\ndata: %s\nstate: %s\nbackups: %s\nlogs: %s\ncache: %s\nunit: %s\n' \
    "$scope" "$account" "$binary" "$releases" "$config" "$data" "$state" "$backups" "$logs" "$cache" "$unit"
  printf '%s\n' \
    'Installation unavailable: artifact trust, ownership checks and lifecycle execution are not connected.' \
    '安装不可用：制品来源验证、安装归属检查及生命周期执行尚未接线。' \
    'Automatic installation is not implemented yet. Keep the existing service; do not replace it using this preview.' \
    '尚未实现自动安装。请保留当前服务，不要直接用此预览替换。' \
    'Paths are intended locations, not proof of existence, permissions or service health. Configuration was not read.' \
    '这些是目标路径，不代表目录存在、权限可用或服务健康；未读取配置。' \
    'The required EnvironmentFile may override the default SQLite path. Keep existing data and secrets unchanged.' \
    '必需的配置文件可覆盖默认 SQLite 路径；必须保留既有数据和密钥。' \
    'An unconfirmed stop never permits backup, replacement or restart. No service command was issued.' \
    '停止未确认时不得备份、替换或重新启动；本次未发出服务控制命令。'
  exit 0
fi

script_directory="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
if [[ "$scope" == system ]]; then
  cat "$script_directory/../my-api.service"
  exit 0
fi

while IFS= read -r line || [[ -n "$line" ]]; do
  # Match and emit literal systemd dollar placeholders, never shell expansion.
  # shellcheck disable=SC2016
  case "$line" in
    'WorkingDirectory=@DATA@') printf 'WorkingDirectory=%s\n' "$(unit_path "$data" directory)" ;;
    'Environment=@SQLITE_DEFAULT@') printf 'Environment=%s\n' "$(unit_quote "SQLITE_PATH=$data/my-api.db?_busy_timeout=30000")" ;;
    'EnvironmentFile=@CONFIG@') printf 'EnvironmentFile=%s\n' "$(unit_path "$config" env-file)" ;;
    'ExecStart=/bin/sh -c "exec \"$$1\" --log-dir \"$$2\"" my-api @BINARY@ @LOGS@')
      # Double backslashes here survive printf once and then systemd unquoting.
      # Path values never enter the fixed shell script; they are $1 and $2.
      printf 'ExecStart=/bin/sh -c "exec \\"$$1\\" --log-dir \\"$$2\\"" my-api %s %s\n' "$(unit_quote "$binary" exec)" "$(unit_quote "$logs" exec)"
      ;;
    *) printf '%s\n' "$line" ;;
  esac
done < "$script_directory/my-api-user.service"
