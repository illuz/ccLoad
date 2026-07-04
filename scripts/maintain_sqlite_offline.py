#!/usr/bin/env python3
"""Offline maintenance for ccLoad's SQLite database.

This script is intentionally designed for *offline* use: stop ccLoad and the
debug analyzer first, then run it from a shell/nohup/tmux.  It can delete
expired debug_logs in small batches and optionally compact the database via
VACUUM INTO + atomic file swap.

Default mode is dry-run.  Add --apply to change data.
"""
from __future__ import annotations

import argparse
import datetime as dt
import os
import shutil
import sqlite3
import subprocess
import sys
import time
from pathlib import Path


DEFAULT_DB = os.environ.get("SQLITE_PATH", "data/ccload.db")
APP_PATTERNS = (
    "/usr/local/bin/ccload",
    "analyze_debug_logs.py",
)


def ts() -> str:
    return dt.datetime.now().strftime("%Y%m%d%H%M%S")


def log(message: str) -> None:
    print(f"{dt.datetime.now().isoformat(timespec='seconds')} {message}", flush=True)


def sql_quote(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def human_bytes(n: int | float) -> str:
    n = float(n)
    for unit in ("B", "KiB", "MiB", "GiB", "TiB"):
        if abs(n) < 1024 or unit == "TiB":
            return f"{n:.2f} {unit}" if unit != "B" else f"{int(n)} B"
        n /= 1024
    return f"{n:.2f} TiB"


def find_running_ccload_processes() -> list[str]:
    try:
        proc = subprocess.run(
            ["ps", "-eo", "pid=,cmd="],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            check=False,
        )
    except Exception:
        return []
    current_pid = os.getpid()
    out: list[str] = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        parts = line.split(None, 1)
        try:
            pid = int(parts[0])
        except Exception:
            continue
        if pid == current_pid:
            continue
        cmd = parts[1] if len(parts) > 1 else ""
        if any(p in cmd for p in APP_PATTERNS):
            out.append(line)
    return out


def read_retention_minutes(conn: sqlite3.Connection, default: int) -> int:
    try:
        row = conn.execute(
            "SELECT value FROM system_settings WHERE key = 'debug_log_retention_minutes'"
        ).fetchone()
    except sqlite3.Error:
        return default
    if not row:
        return default
    try:
        value = int(str(row[0]).strip())
        return value if value > 0 else default
    except Exception:
        return default


def checkpoint_truncate(conn: sqlite3.Connection, label: str) -> None:
    try:
        rows = conn.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchall()
        log(f"{label}: wal_checkpoint(TRUNCATE)={rows}")
    except sqlite3.Error as exc:
        log(f"{label}: wal_checkpoint(TRUNCATE) failed: {exc!r}")


def delete_expired_debug_logs(
    conn: sqlite3.Connection,
    cutoff: int,
    batch_size: int,
    sleep_seconds: float,
) -> int:
    deleted = 0
    started = time.time()
    while True:
        conn.execute(
            """
            DELETE FROM debug_logs
            WHERE log_id IN (
                SELECT log_id FROM debug_logs
                WHERE created_at < ?
                ORDER BY created_at
                LIMIT ?
            )
            """,
            (cutoff, batch_size),
        )
        n = int(conn.execute("SELECT changes()").fetchone()[0])
        if n <= 0:
            break
        deleted += n
        if deleted % max(batch_size * 20, 1) == 0 or n < batch_size:
            freelist = conn.execute("PRAGMA freelist_count").fetchone()[0]
            page_size = conn.execute("PRAGMA page_size").fetchone()[0]
            log(
                "delete progress: "
                f"deleted={deleted}, elapsed={time.time() - started:.1f}s, "
                f"freelist={freelist} pages ({human_bytes(freelist * page_size)})"
            )
        if n < batch_size:
            break
        if sleep_seconds > 0:
            time.sleep(sleep_seconds)
    return deleted


def quick_check(path: Path) -> None:
    uri = f"file:{path.resolve()}?mode=ro"
    conn = sqlite3.connect(uri, uri=True)
    try:
        result = conn.execute("PRAGMA quick_check").fetchone()[0]
    finally:
        conn.close()
    if result != "ok":
        raise RuntimeError(f"quick_check failed for {path}: {result}")


def move_if_exists(src: Path, dst: Path) -> None:
    if src.exists():
        os.replace(src, dst)


def compact_with_vacuum_into(conn: sqlite3.Connection, db_path: Path, backup_dir: Path) -> Path:
    backup_dir.mkdir(parents=True, exist_ok=True)
    stamp = ts()
    tmp_path = db_path.with_name(f"{db_path.name}.compact.{stamp}.tmp")
    backup_db = backup_dir / f"{db_path.name}.bak.{stamp}"
    backup_wal = backup_dir / f"{db_path.name}-wal.bak.{stamp}"
    backup_shm = backup_dir / f"{db_path.name}-shm.bak.{stamp}"

    if tmp_path.exists():
        tmp_path.unlink()

    original_stat = db_path.stat()

    log(f"VACUUM INTO start: {tmp_path}")
    last_progress = 0.0

    def progress() -> int:
        nonlocal last_progress
        now = time.time()
        if now - last_progress >= 10:
            size = tmp_path.stat().st_size if tmp_path.exists() else 0
            log(f"VACUUM INTO progress: tmp_size={human_bytes(size)}")
            last_progress = now
        return 0

    conn.set_progress_handler(progress, 200_000)
    try:
        conn.execute(f"VACUUM INTO {sql_quote(str(tmp_path))}")
    finally:
        conn.set_progress_handler(None, 0)
    log(f"VACUUM INTO done: tmp_size={human_bytes(tmp_path.stat().st_size)}")

    log("quick_check compacted database")
    quick_check(tmp_path)

    log(f"swap start: backup_dir={backup_dir}")
    conn.close()
    move_if_exists(db_path.with_name(db_path.name + "-wal"), backup_wal)
    move_if_exists(db_path.with_name(db_path.name + "-shm"), backup_shm)
    os.replace(db_path, backup_db)
    os.replace(tmp_path, db_path)
    os.chmod(db_path, original_stat.st_mode)
    try:
        os.chown(db_path, original_stat.st_uid, original_stat.st_gid)
    except PermissionError:
        pass
    log(f"swap done: backup_db={backup_db}")
    return backup_db


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Offline SQLite maintenance for ccLoad")
    parser.add_argument("--db", default=DEFAULT_DB, help="SQLite DB path")
    parser.add_argument(
        "--retention-minutes",
        type=int,
        help="debug_logs retention. Defaults to DB setting debug_log_retention_minutes, fallback 1440",
    )
    parser.add_argument("--batch-size", type=int, default=100, help="Rows per delete batch")
    parser.add_argument("--sleep", type=float, default=0.02, help="Sleep seconds between delete batches")
    parser.add_argument(
        "--mode",
        choices=("delete-only", "compact-copy"),
        default="compact-copy",
        help="delete-only leaves free pages in DB; compact-copy uses VACUUM INTO then swaps files",
    )
    parser.add_argument("--backup-dir", help="Backup directory; default is <db-dir>/backups")
    parser.add_argument("--apply", action="store_true", help="Actually modify the DB. Omit for dry-run")
    parser.add_argument(
        "--allow-online",
        action="store_true",
        help="Allow running while ccload/analyzer processes are detected (not recommended)",
    )
    args = parser.parse_args(argv)

    db_path = Path(args.db).resolve()
    if not db_path.exists():
        raise SystemExit(f"DB not found: {db_path}")
    backup_dir = Path(args.backup_dir).resolve() if args.backup_dir else db_path.parent / "backups"

    running = find_running_ccload_processes()
    if running and not args.allow_online:
        log("Refusing to run while ccLoad-related processes are active:")
        for line in running:
            log(f"  {line}")
        log("Stop ccload and ccload-debug-analyzer first, or pass --allow-online if you know what you are doing.")
        return 2

    log(f"db={db_path}")
    log(f"db_size={human_bytes(db_path.stat().st_size)}")
    for suffix in ("-wal", "-shm"):
        p = db_path.with_name(db_path.name + suffix)
        if p.exists():
            log(f"{p.name}_size={human_bytes(p.stat().st_size)}")

    conn = sqlite3.connect(str(db_path), timeout=60, isolation_level=None)
    conn.execute("PRAGMA busy_timeout=60000")
    try:
        page_size = int(conn.execute("PRAGMA page_size").fetchone()[0])
        page_count = int(conn.execute("PRAGMA page_count").fetchone()[0])
        freelist = int(conn.execute("PRAGMA freelist_count").fetchone()[0])
        log(
            f"pages: page_size={page_size}, page_count={page_count}, "
            f"freelist={freelist} ({human_bytes(freelist * page_size)})"
        )

        retention = args.retention_minutes or read_retention_minutes(conn, 1440)
        cutoff = int(time.time() - retention * 60)
        cutoff_text = dt.datetime.fromtimestamp(cutoff).isoformat(timespec="seconds")
        expired = conn.execute(
            "SELECT count(*), min(created_at), max(created_at) FROM debug_logs WHERE created_at < ?",
            (cutoff,),
        ).fetchone()
        total = conn.execute(
            "SELECT count(*), min(created_at), max(created_at) FROM debug_logs"
        ).fetchone()
        log(f"retention_minutes={retention}, cutoff={cutoff_text}")
        log(f"debug_logs total={tuple(total)}, expired={tuple(expired)}")

        if not args.apply:
            log("dry-run only. Re-run with --apply to modify the database.")
            return 0

        checkpoint_truncate(conn, "before delete")
        deleted = delete_expired_debug_logs(conn, cutoff, args.batch_size, args.sleep)
        log(f"delete done: deleted={deleted}")
        checkpoint_truncate(conn, "after delete")

        freelist = int(conn.execute("PRAGMA freelist_count").fetchone()[0])
        log(f"post-delete freelist={freelist} ({human_bytes(freelist * page_size)})")

        if args.mode == "delete-only":
            log("mode=delete-only; skip compaction")
            return 0

        # VACUUM INTO needs enough free space for the compact copy.  Use current
        # DB size as a conservative floor; abort early if the filesystem is tight.
        usage = shutil.disk_usage(db_path.parent)
        min_free = max(5 * 1024**3, db_path.stat().st_size // 3)
        if usage.free < min_free:
            raise RuntimeError(
                f"not enough free disk space for compact-copy: free={human_bytes(usage.free)}, "
                f"required_at_least={human_bytes(min_free)}"
            )

        compact_with_vacuum_into(conn, db_path, backup_dir)
        log(f"final_db_size={human_bytes(db_path.stat().st_size)}")
        return 0
    finally:
        try:
            conn.close()
        except Exception:
            pass


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
