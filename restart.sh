#!/usr/bin/env bash
set -euo pipefail

APP_NAME="${APP_NAME:-ccload}"
SOURCE_DIR="${SOURCE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
RUNTIME_DIR="${RUNTIME_DIR:-/root/workspace/ccload-runtime}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/ccload}"
ENV_FILE="${ENV_FILE:-$RUNTIME_DIR/.env}"
LOG_DIR="${LOG_DIR:-$RUNTIME_DIR/logs}"
DEBUG_ANALYSIS_DIR="${DEBUG_ANALYSIS_DIR:-$RUNTIME_DIR/data/debug-analysis}"
ANALYZER_ENABLED="${ANALYZER_ENABLED:-1}"
ANALYZER_NAME="${ANALYZER_NAME:-${APP_NAME}-debug-analyzer}"
ANALYZER_SCRIPT_SOURCE="${ANALYZER_SCRIPT_SOURCE:-$SOURCE_DIR/scripts/analyze_debug_logs.py}"
ANALYZER_RUNTIME_SCRIPT="${ANALYZER_RUNTIME_SCRIPT:-$RUNTIME_DIR/scripts/analyze_debug_logs.py}"
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

  if [[ "$PM2_EXISTS_BEFORE" -eq 1 ]]; then
    echo "==> Restoring previous PM2 process"
    "$PM2_BIN" restart "$APP_NAME" --update-env >/dev/null 2>&1 \
      || "$PM2_BIN" start "$BIN_PATH" \
        --name "$APP_NAME" \
        --cwd "$RUNTIME_DIR" \
        --interpreter none \
        --time \
        --output "$LOG_DIR/ccload.log" \
        --error "$LOG_DIR/ccload.error.log" >/dev/null 2>&1 \
      || true
  fi
}

healthcheck() {
  local attempt
  for ((attempt = 1; attempt <= HEALTHCHECK_RETRIES; attempt++)); do
    if curl --silent --show-error --fail \
      --max-time "$HEALTHCHECK_TIMEOUT" \
      "$HEALTHCHECK_URL" >/dev/null; then
      echo "==> Health check passed: $HEALTHCHECK_URL"
      return 0
    fi

    if [[ "$attempt" -lt "$HEALTHCHECK_RETRIES" ]]; then
      echo "==> Health check failed (attempt $attempt/$HEALTHCHECK_RETRIES), retrying in ${HEALTHCHECK_INTERVAL}s..."
      sleep "$HEALTHCHECK_INTERVAL"
    fi
  done

  echo "ERROR: health check failed after $HEALTHCHECK_RETRIES attempts: $HEALTHCHECK_URL" >&2
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
  echo "ERROR: pm2 not found in PATH. Set PM2_BIN=/path/to/pm2 or install pm2 first." >&2
  exit 1
fi

# Make pm2's node runtime available even when this script is run from a minimal shell.
PM2_REAL="$(readlink -f "$PM2_BIN" 2>/dev/null || echo "$PM2_BIN")"
if [[ "$PM2_REAL" == */lib/node_modules/pm2/bin/pm2 ]]; then
  NODE_HOME="${PM2_REAL%/lib/node_modules/pm2/bin/pm2}"
  export PATH="$NODE_HOME/bin:$PATH"
fi

if [[ ! -d "$RUNTIME_DIR" ]]; then
  echo "ERROR: runtime dir not found: $RUNTIME_DIR" >&2
  exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
  echo "ERROR: env file not found: $ENV_FILE" >&2
  exit 1
fi

# Load runtime env so PORT/HEALTHCHECK_URL and other shell-compatible values
# are available to this deploy script too.
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

PORT="${PORT:-8080}"
SQLITE_PATH="${SQLITE_PATH:-$RUNTIME_DIR/data/ccload.db}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-http://127.0.0.1:${PORT}/health}"
HEALTHCHECK_RETRIES="${HEALTHCHECK_RETRIES:-10}"
HEALTHCHECK_INTERVAL="${HEALTHCHECK_INTERVAL:-2}"
HEALTHCHECK_TIMEOUT="${HEALTHCHECK_TIMEOUT:-3}"

mkdir -p "$LOG_DIR" "$DEBUG_ANALYSIS_DIR" "$(dirname "$ANALYZER_RUNTIME_SCRIPT")"
export CCLOAD_DEBUG_ANALYSIS_DIR="$DEBUG_ANALYSIS_DIR"

cd "$SOURCE_DIR"
echo "==> Building ccLoad from $SOURCE_DIR"
GOTAGS="$GOTAGS" make build

if "$PM2_BIN" describe "$APP_NAME" >/dev/null 2>&1; then
  PM2_EXISTS_BEFORE=1
fi

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
  if [[ -z "$PYTHON_BIN" ]]; then
    echo "ERROR: python3 not found in PATH. Set PYTHON_BIN=/path/to/python3 or disable with ANALYZER_ENABLED=0." >&2
    exit 1
  fi
  if [[ ! -f "$ANALYZER_SCRIPT_SOURCE" ]]; then
    echo "ERROR: analyzer script not found: $ANALYZER_SCRIPT_SOURCE" >&2
    exit 1
  fi
  echo "==> Installing debug analyzer to $ANALYZER_RUNTIME_SCRIPT"
  install -m 0755 "$ANALYZER_SCRIPT_SOURCE" "$ANALYZER_RUNTIME_SCRIPT"
fi

if [[ "$PM2_EXISTS_BEFORE" -eq 1 ]]; then
  echo "==> Restarting PM2 app: $APP_NAME"
  "$PM2_BIN" restart "$APP_NAME" --update-env
else
  echo "==> Starting PM2 app: $APP_NAME"
  "$PM2_BIN" start "$BIN_PATH" \
    --name "$APP_NAME" \
    --cwd "$RUNTIME_DIR" \
    --interpreter none \
    --time \
    --output "$LOG_DIR/ccload.log" \
    --error "$LOG_DIR/ccload.error.log"
fi

if [[ "$ANALYZER_ENABLED" == "1" ]]; then
  echo "==> Starting PM2 debug analyzer: $ANALYZER_NAME"
  "$PM2_BIN" delete "$ANALYZER_NAME" >/dev/null 2>&1 || true
  "$PM2_BIN" start "$ANALYZER_RUNTIME_SCRIPT" \
    --name "$ANALYZER_NAME" \
    --cwd "$RUNTIME_DIR" \
    --interpreter "$PYTHON_BIN" \
    --time \
    --output "$LOG_DIR/ccload-debug-analyzer.log" \
    --error "$LOG_DIR/ccload-debug-analyzer.error.log" \
    -- \
    --db "$SQLITE_PATH" \
    --out-dir "$DEBUG_ANALYSIS_DIR" \
    --follow
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
