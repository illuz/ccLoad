#!/usr/bin/env bash
set -euo pipefail

# Stop ccLoad, run SQLite maintenance offline, then start ccLoad again.
#
# Safe default: dry-run.  Use APPLY=1 to actually delete/compact:
#   nohup env APPLY=1 MODE=compact-copy scripts/offline_db_maintenance.sh \
#     > /root/workspace/ccload-runtime/logs/offline-db-maintenance.log 2>&1 &

APP_NAME="${APP_NAME:-ccload}"
ANALYZER_NAME="${ANALYZER_NAME:-${APP_NAME}-debug-analyzer}"
RUNTIME_DIR="${RUNTIME_DIR:-/root/workspace/ccload-runtime}"
DB_PATH="${SQLITE_PATH:-$RUNTIME_DIR/data/ccload.db}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAINTAIN_SCRIPT="${MAINTAIN_SCRIPT:-$SCRIPT_DIR/maintain_sqlite_offline.py}"
PYTHON_BIN="${PYTHON_BIN:-$(command -v python3)}"
PM2_BIN="${PM2_BIN:-$(command -v pm2)}"
MODE="${MODE:-compact-copy}" # compact-copy | delete-only
APPLY="${APPLY:-0}"
RETENTION_MINUTES="${RETENTION_MINUTES:-}"
BACKUP_DIR="${BACKUP_DIR:-$RUNTIME_DIR/data/backups}"

pm2_clean() {
  env \
    HTTP_PROXY= HTTPS_PROXY= ALL_PROXY= NO_PROXY= \
    http_proxy= https_proxy= all_proxy= no_proxy= \
    "$PM2_BIN" "$@"
}

was_online() {
  local name="$1"
  pm2_clean describe "$name" 2>/dev/null | grep -q "status.*online"
}

APP_WAS_ONLINE=0
ANALYZER_WAS_ONLINE=0
if was_online "$APP_NAME"; then APP_WAS_ONLINE=1; fi
if was_online "$ANALYZER_NAME"; then ANALYZER_WAS_ONLINE=1; fi

restart_services() {
  set +e
  if [[ "$APP_WAS_ONLINE" == "1" ]]; then
    echo "==> Starting $APP_NAME"
    pm2_clean restart "$APP_NAME" --update-env
  fi
  if [[ "$ANALYZER_WAS_ONLINE" == "1" ]]; then
    echo "==> Starting $ANALYZER_NAME"
    pm2_clean restart "$ANALYZER_NAME" --update-env
  fi
}
trap restart_services EXIT

echo "==> Stopping analyzer first: $ANALYZER_NAME"
pm2_clean stop "$ANALYZER_NAME" >/dev/null 2>&1 || true
echo "==> Stopping app: $APP_NAME"
pm2_clean stop "$APP_NAME" >/dev/null 2>&1 || true
sleep 2

args=(
  "$MAINTAIN_SCRIPT"
  --db "$DB_PATH"
  --mode "$MODE"
  --backup-dir "$BACKUP_DIR"
)
if [[ "$APPLY" == "1" ]]; then
  args+=(--apply)
else
  echo "==> DRY-RUN mode. Set APPLY=1 to modify the database."
fi
if [[ -n "$RETENTION_MINUTES" ]]; then
  args+=(--retention-minutes "$RETENTION_MINUTES")
fi

echo "==> Running offline maintenance: ${args[*]}"
"$PYTHON_BIN" "${args[@]}"
echo "==> Offline maintenance finished"
