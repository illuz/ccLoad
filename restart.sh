#!/usr/bin/env bash
set -euo pipefail

APP_NAME="${APP_NAME:-ccload}"
SOURCE_DIR="${SOURCE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
RUNTIME_DIR="${RUNTIME_DIR:-/root/workspace/ccload-runtime}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/ccload}"
ENV_FILE="${ENV_FILE:-$RUNTIME_DIR/.env}"
LOG_DIR="${LOG_DIR:-$RUNTIME_DIR/logs}"
DEBUG_LOG_DIR="${DEBUG_LOG_DIR:-$RUNTIME_DIR/data/debug-logs}"
DEBUG_ANALYSIS_DIR="${DEBUG_ANALYSIS_DIR:-$RUNTIME_DIR/data/debug-analysis}"
ANALYZER_ENABLED="${ANALYZER_ENABLED:-1}"
ANALYZER_NAME="${ANALYZER_NAME:-${APP_NAME}-debug-analyzer}"
ANALYZER_BIN_SOURCE="${ANALYZER_BIN_SOURCE:-$SOURCE_DIR/ccload-debug-analyzer}"
ANALYZER_BIN_PATH="${ANALYZER_BIN_PATH:-/usr/local/bin/ccload-debug-analyzer}"
DB_MAINTENANCE_SCRIPT_SOURCE="${DB_MAINTENANCE_SCRIPT_SOURCE:-$SOURCE_DIR/scripts/maintain_sqlite_offline.py}"
DB_MAINTENANCE_RUNTIME_SCRIPT="${DB_MAINTENANCE_RUNTIME_SCRIPT:-$RUNTIME_DIR/scripts/maintain_sqlite_offline.py}"
DB_MAINTENANCE_ON_RESTART="${DB_MAINTENANCE_ON_RESTART:-0}"
DB_MAINTENANCE_MODE="${DB_MAINTENANCE_MODE:-compact-copy}" # compact-copy | delete-only
DB_MAINTENANCE_BACKUP_DIR="${DB_MAINTENANCE_BACKUP_DIR:-$RUNTIME_DIR/data/backups}"
PYTHON_BIN="${PYTHON_BIN:-$(command -v python3 || true)}"
PM2_BIN="${PM2_BIN:-$(command -v pm2 || true)}"
if [[ -z "$PM2_BIN" && -x "/root/.local/share/fnm/node-versions/v16.20.2/installation/bin/pm2" ]]; then
  PM2_BIN="/root/.local/share/fnm/node-versions/v16.20.2/installation/bin/pm2"
fi
GOTAGS="${GOTAGS:-sonic}"
BACKUP_PATH=""
PM2_EXISTS_BEFORE=0
DEPLOYED_BINARY=0
OLD_BIN_PRESENT=0
APP_START_ATTEMPTED=0

pm2_with_clean_proxy_env() {
  env \
    HTTP_PROXY= \
    HTTPS_PROXY= \
    ALL_PROXY= \
    NO_PROXY= \
    http_proxy= \
    https_proxy= \
    all_proxy= \
    no_proxy= \
    "$PM2_BIN" "$@"
}

fail() {
  echo "ERROR: $*" >&2
  return 1
}

pm2_app_pid() {
  local output
  output="$(pm2_with_clean_proxy_env pid "$APP_NAME" 2>/dev/null || true)"
  awk '/^[0-9]+$/ && $1 > 0 { print $1; exit }' <<<"$output"
}

port_listener_snapshot() {
  ss -H -ltnp "sport = :$PORT_NUMBER" 2>/dev/null || true
}

port_owned_by_pid() {
  local pid="$1"
  local listeners
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  listeners="$(port_listener_snapshot)"
  [[ -n "$listeners" ]] && grep -Eq "pid=${pid}," <<<"$listeners"
}

check_port_before_deploy() {
  local listeners current_pid
  listeners="$(port_listener_snapshot)"
  if [[ -z "$listeners" ]]; then
    echo "==> HTTP port $PORT_NUMBER is currently free"
    return 0
  fi

  current_pid="$(pm2_app_pid)"
  if [[ "$PM2_EXISTS_BEFORE" -eq 1 ]] && port_owned_by_pid "$current_pid"; then
    echo "==> HTTP port $PORT_NUMBER is owned by current PM2 app PID $current_pid"
    return 0
  fi

  echo "ERROR: HTTP port $PORT_NUMBER is occupied by a process other than PM2 app '$APP_NAME'" >&2
  echo "$listeners" >&2
  return 1
}

install_binary_atomically() {
  local src="$1"
  local dst="$2"
  local tmp="${dst}.tmp.$$"
  install -m 0755 "$src" "$tmp"
  mv -f "$tmp" "$dst"
}

rollback() {
  set +e

  if [[ "$DEPLOYED_BINARY" -eq 1 ]]; then
    if [[ -n "$BACKUP_PATH" && -f "$BACKUP_PATH" ]]; then
      install_binary_atomically "$BACKUP_PATH" "$BIN_PATH"
      echo "==> Rolled back binary from backup"
    elif [[ "$OLD_BIN_PRESENT" -eq 0 && -f "$BIN_PATH" ]]; then
      rm -f "$BIN_PATH"
    fi
  fi

  if [[ "$APP_START_ATTEMPTED" -eq 1 && "$PM2_EXISTS_BEFORE" -eq 1 ]]; then
    echo "==> Restoring previous PM2 process"
    if pm2_with_clean_proxy_env restart "$APP_NAME" --update-env >/dev/null 2>&1 \
      || pm2_with_clean_proxy_env start "$BIN_PATH" \
        --name "$APP_NAME" \
        --cwd "$RUNTIME_DIR" \
        --interpreter none \
        --time \
        --output "$LOG_DIR/ccload.log" \
        --error "$LOG_DIR/ccload.error.log" >/dev/null 2>&1; then
      if healthcheck; then
        echo "==> Rollback health check passed"
      else
        echo "ERROR: previous binary was restored, but rollback health check failed" >&2
      fi
    else
      echo "ERROR: previous binary was restored, but PM2 could not restart it" >&2
    fi
  elif [[ "$APP_START_ATTEMPTED" -eq 1 ]]; then
    echo "==> Removing failed new PM2 process"
    pm2_with_clean_proxy_env delete "$APP_NAME" >/dev/null 2>&1 || true
  fi
}

healthcheck() {
  local attempt app_pid stable_pid
  for ((attempt = 1; attempt <= HEALTHCHECK_RETRIES; attempt++)); do
    app_pid="$(pm2_app_pid)"
    if port_owned_by_pid "$app_pid" \
      && curl --silent --show-error --fail --noproxy '*' \
        --max-time "$HEALTHCHECK_TIMEOUT" \
        "$HEALTHCHECK_URL" >/dev/null; then
      if [[ "$HEALTHCHECK_STABLE_SECONDS" != "0" ]]; then
        sleep "$HEALTHCHECK_STABLE_SECONDS"
      fi
      stable_pid="$(pm2_app_pid)"
      if [[ "$stable_pid" == "$app_pid" ]] \
        && port_owned_by_pid "$stable_pid" \
        && curl --silent --show-error --fail --noproxy '*' \
          --max-time "$HEALTHCHECK_TIMEOUT" \
          "$HEALTHCHECK_URL" >/dev/null; then
        echo "==> Health check passed: $HEALTHCHECK_URL (PM2 PID $stable_pid owns port $PORT_NUMBER)"
        return 0
      fi
    fi

    if [[ "$attempt" -lt "$HEALTHCHECK_RETRIES" ]]; then
      echo "==> Health/port check failed (attempt $attempt/$HEALTHCHECK_RETRIES, PM2 PID ${app_pid:-none}), retrying in ${HEALTHCHECK_INTERVAL}s..."
      sleep "$HEALTHCHECK_INTERVAL"
    fi
  done

  echo "ERROR: health/port check failed after $HEALTHCHECK_RETRIES attempts: $HEALTHCHECK_URL" >&2
  port_listener_snapshot >&2
  return 1
}

on_error() {
  local line="$1"
  local code="$2"
  echo "ERROR: deployment failed at line $line (exit $code), rolling back..." >&2
  rollback
  exit "$code"
}

trap 'on_error $LINENO $?' ERR

if [[ -z "$PM2_BIN" ]]; then
  fail "pm2 not found in PATH. Set PM2_BIN=/path/to/pm2 or install pm2 first."
fi

# Make pm2's node runtime available even when this script is run from a minimal shell.
PM2_REAL="$(readlink -f "$PM2_BIN" 2>/dev/null || echo "$PM2_BIN")"
if [[ "$PM2_REAL" == */lib/node_modules/pm2/bin/pm2 ]]; then
  NODE_HOME="${PM2_REAL%/lib/node_modules/pm2/bin/pm2}"
  export PATH="$NODE_HOME/bin:$PATH"
fi

if [[ ! -d "$RUNTIME_DIR" ]]; then
  fail "runtime dir not found: $RUNTIME_DIR"
fi

if [[ ! -f "$ENV_FILE" ]]; then
  fail "env file not found: $ENV_FILE"
fi

# Load runtime env so PORT/HEALTHCHECK_URL and other shell-compatible values
# are available to this deploy script too.
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

PORT="${PORT:-8080}"
PORT_NUMBER="${PORT#:}"
if [[ ! "$PORT_NUMBER" =~ ^[0-9]+$ ]] || ((10#$PORT_NUMBER < 1 || 10#$PORT_NUMBER > 65535)); then
  fail "invalid PORT value: $PORT"
fi
SQLITE_PATH="${SQLITE_PATH:-$RUNTIME_DIR/data/ccload.db}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-http://127.0.0.1:${PORT_NUMBER}/health}"
HEALTHCHECK_RETRIES="${HEALTHCHECK_RETRIES:-10}"
HEALTHCHECK_INTERVAL="${HEALTHCHECK_INTERVAL:-2}"
HEALTHCHECK_TIMEOUT="${HEALTHCHECK_TIMEOUT:-3}"
HEALTHCHECK_STABLE_SECONDS="${HEALTHCHECK_STABLE_SECONDS:-2}"
DEBUG_LOG_DIR="${CCLOAD_DEBUG_LOG_DIR:-$DEBUG_LOG_DIR}"
DEBUG_ANALYSIS_DIR="${CCLOAD_DEBUG_ANALYSIS_DIR:-$DEBUG_ANALYSIS_DIR}"

command -v curl >/dev/null 2>&1 || fail "curl not found in PATH"
command -v ss >/dev/null 2>&1 || fail "ss not found in PATH (install iproute2)"

# Relative Debug paths are resolved against the PM2 runtime directory.  The
# deploy script may be invoked from any working directory, while PM2 starts
# both processes with --cwd "$RUNTIME_DIR".
if [[ "$DEBUG_LOG_DIR" != /* ]]; then
  DEBUG_LOG_DIR="$RUNTIME_DIR/$DEBUG_LOG_DIR"
fi
if [[ "$DEBUG_ANALYSIS_DIR" != /* ]]; then
  DEBUG_ANALYSIS_DIR="$RUNTIME_DIR/$DEBUG_ANALYSIS_DIR"
fi

mkdir -p "$LOG_DIR" "$DEBUG_LOG_DIR" "$DEBUG_ANALYSIS_DIR" "$(dirname "$DB_MAINTENANCE_RUNTIME_SCRIPT")"
chmod 700 "$DEBUG_LOG_DIR" "$DEBUG_ANALYSIS_DIR"
export CCLOAD_DEBUG_LOG_DIR="$DEBUG_LOG_DIR"
export CCLOAD_DEBUG_ANALYSIS_DIR="$DEBUG_ANALYSIS_DIR"

if pm2_with_clean_proxy_env describe "$APP_NAME" >/dev/null 2>&1; then
  PM2_EXISTS_BEFORE=1
fi
check_port_before_deploy

cd "$SOURCE_DIR"
echo "==> Building ccLoad from $SOURCE_DIR"
GOTAGS="$GOTAGS" make build

echo "==> Installing binary to $BIN_PATH"
if [[ -f "$BIN_PATH" ]]; then
  BACKUP_PATH="${BIN_PATH}.bak.$(date +%Y%m%d%H%M%S)"
  cp -a "$BIN_PATH" "$BACKUP_PATH"
  OLD_BIN_PRESENT=1
  echo "    backup: $BACKUP_PATH"
fi
install_binary_atomically "$SOURCE_DIR/ccload" "$BIN_PATH"
DEPLOYED_BINARY=1

if [[ "$ANALYZER_ENABLED" == "1" ]]; then
  if [[ ! -f "$ANALYZER_BIN_SOURCE" ]]; then
    fail "analyzer binary not found: $ANALYZER_BIN_SOURCE"
  fi
  echo "==> Installing Go debug analyzer to $ANALYZER_BIN_PATH"
  install_binary_atomically "$ANALYZER_BIN_SOURCE" "$ANALYZER_BIN_PATH"
fi

if [[ -f "$DB_MAINTENANCE_SCRIPT_SOURCE" ]]; then
  echo "==> Installing SQLite maintenance script to $DB_MAINTENANCE_RUNTIME_SCRIPT"
  install -m 0755 "$DB_MAINTENANCE_SCRIPT_SOURCE" "$DB_MAINTENANCE_RUNTIME_SCRIPT"
fi

if [[ "$DB_MAINTENANCE_ON_RESTART" == "1" ]]; then
  if [[ -z "$PYTHON_BIN" ]]; then
    fail "python3 not found in PATH. Set PYTHON_BIN=/path/to/python3 or disable DB_MAINTENANCE_ON_RESTART=0."
  fi
  if [[ ! -f "$DB_MAINTENANCE_RUNTIME_SCRIPT" ]]; then
    fail "SQLite maintenance script not found: $DB_MAINTENANCE_RUNTIME_SCRIPT"
  fi

  echo "==> Offline SQLite maintenance requested; stopping PM2 app/analyzer first"
  APP_START_ATTEMPTED=1
  "$PM2_BIN" stop "$ANALYZER_NAME" >/dev/null 2>&1 || true
  "$PM2_BIN" stop "$APP_NAME" >/dev/null 2>&1 || true
  "$PYTHON_BIN" "$DB_MAINTENANCE_RUNTIME_SCRIPT" \
    --db "$SQLITE_PATH" \
    --mode "$DB_MAINTENANCE_MODE" \
    --backup-dir "$DB_MAINTENANCE_BACKUP_DIR" \
    --apply
fi

if [[ "$PM2_EXISTS_BEFORE" -eq 1 ]]; then
  echo "==> Restarting PM2 app: $APP_NAME"
  APP_START_ATTEMPTED=1
  pm2_with_clean_proxy_env restart "$APP_NAME" --update-env
else
  echo "==> Starting PM2 app: $APP_NAME"
  APP_START_ATTEMPTED=1
  pm2_with_clean_proxy_env start "$BIN_PATH" \
    --name "$APP_NAME" \
    --cwd "$RUNTIME_DIR" \
    --interpreter none \
    --time \
    --output "$LOG_DIR/ccload.log" \
    --error "$LOG_DIR/ccload.error.log"
fi

if [[ "$ANALYZER_ENABLED" == "1" ]]; then
  echo "==> Starting PM2 Go debug analyzer: $ANALYZER_NAME"
  "$PM2_BIN" delete "$ANALYZER_NAME" >/dev/null 2>&1 || true
  pm2_with_clean_proxy_env start "$ANALYZER_BIN_PATH" \
    --name "$ANALYZER_NAME" \
    --cwd "$RUNTIME_DIR" \
    --interpreter none \
    --time \
    --output "$LOG_DIR/ccload-debug-analyzer.log" \
    --error "$LOG_DIR/ccload-debug-analyzer.error.log" \
    -- \
    --input-dir "$DEBUG_LOG_DIR" \
    --out-dir "$DEBUG_ANALYSIS_DIR" \
    --follow
else
  "$PM2_BIN" delete "$ANALYZER_NAME" >/dev/null 2>&1 || true
fi

echo "==> Running health check"
healthcheck

echo "==> Saving PM2 process list"
"$PM2_BIN" save

echo "==> PM2 status"
"$PM2_BIN" status "$APP_NAME"
if [[ "$ANALYZER_ENABLED" == "1" ]]; then
  "$PM2_BIN" status "$ANALYZER_NAME"
fi
