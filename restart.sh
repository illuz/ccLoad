#!/usr/bin/env bash
set -euo pipefail

APP_NAME="${APP_NAME:-ccload}"
SOURCE_DIR="${SOURCE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
RUNTIME_DIR="${RUNTIME_DIR:-/root/workspace/ccload-runtime}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/ccload}"
ENV_FILE="${ENV_FILE:-$RUNTIME_DIR/.env}"
LOG_DIR="${LOG_DIR:-$RUNTIME_DIR/logs}"
PM2_BIN="${PM2_BIN:-$(command -v pm2 || true)}"
if [[ -z "$PM2_BIN" && -x "/root/.local/share/fnm/node-versions/v16.20.2/installation/bin/pm2" ]]; then
  PM2_BIN="/root/.local/share/fnm/node-versions/v16.20.2/installation/bin/pm2"
fi
GOTAGS="${GOTAGS:-sonic}"

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

mkdir -p "$LOG_DIR"

cd "$SOURCE_DIR"
echo "==> Building ccLoad from $SOURCE_DIR"
GOTAGS="$GOTAGS" make build

echo "==> Installing binary to $BIN_PATH"
if [[ -f "$BIN_PATH" ]]; then
  BACKUP_PATH="${BIN_PATH}.bak.$(date +%Y%m%d%H%M%S)"
  cp -a "$BIN_PATH" "$BACKUP_PATH"
  echo "    backup: $BACKUP_PATH"
fi
install -m 0755 "$SOURCE_DIR/ccload" "$BIN_PATH"

echo "==> Starting PM2 app: $APP_NAME"
if "$PM2_BIN" describe "$APP_NAME" >/dev/null 2>&1; then
  "$PM2_BIN" delete "$APP_NAME" >/dev/null 2>&1 || true
fi
"$PM2_BIN" start "$BIN_PATH" \
  --name "$APP_NAME" \
  --cwd "$RUNTIME_DIR" \
  --interpreter none \
  --time \
  --output "$LOG_DIR/ccload.log" \
  --error "$LOG_DIR/ccload.error.log"

echo "==> Saving PM2 process list"
"$PM2_BIN" save

echo "==> PM2 status"
"$PM2_BIN" status "$APP_NAME"
