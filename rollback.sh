#!/usr/bin/env bash
set -euo pipefail

APP_NAME="${APP_NAME:-ccload}"
RUNTIME_DIR="${RUNTIME_DIR:-/root/workspace/ccload-runtime}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/ccload}"
ENV_FILE="${ENV_FILE:-$RUNTIME_DIR/.env}"
LOG_DIR="${LOG_DIR:-$RUNTIME_DIR/logs}"
PM2_BIN="${PM2_BIN:-$(command -v pm2 || true)}"
if [[ -z "$PM2_BIN" && -x "/root/.local/share/fnm/node-versions/v16.20.2/installation/bin/pm2" ]]; then
  PM2_BIN="/root/.local/share/fnm/node-versions/v16.20.2/installation/bin/pm2"
fi

if [[ -z "$PM2_BIN" ]]; then
  echo "ERROR: pm2 not found in PATH. Set PM2_BIN=/path/to/pm2 or install pm2 first." >&2
  exit 1
fi

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

shopt -s nullglob
backups=( "$BIN_PATH".bak.* )
shopt -u nullglob

if [[ ${#backups[@]} -eq 0 ]]; then
  echo "ERROR: no backup binaries found for $BIN_PATH" >&2
  exit 1
fi

IFS=$'\n' sorted=( $(printf '%s\n' "${backups[@]}" | sort) )
unset IFS
LATEST_BACKUP="${sorted[-1]}"

echo "==> Rolling back binary from: $LATEST_BACKUP"
cp -a "$LATEST_BACKUP" "$BIN_PATH"
chmod 0755 "$BIN_PATH"

if "$PM2_BIN" describe "$APP_NAME" >/dev/null 2>&1; then
  echo "==> Restarting existing PM2 app: $APP_NAME"
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

echo "==> Saving PM2 process list"
"$PM2_BIN" save

echo "==> PM2 status"
"$PM2_BIN" status "$APP_NAME"
