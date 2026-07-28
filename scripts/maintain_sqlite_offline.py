#!/usr/bin/env python3
"""Offline maintenance for ccLoad's SQLite database.

This script is intentionally designed for *offline* use: stop ccLoad and the
debug analyzer first, then run it from a shell/nohup/tmux. It drops the obsolete
debug_logs BLOB table and compacts the database via VACUUM INTO + atomic file
swap. New Debug logs are compressed files and never enter SQLite.

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
    "ccload-debug-analyzer",
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


def checkpoint_truncate(conn: sqlite3.Connection, label: str) -> None:
    try:
        rows = conn.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchall()
        log(f"{label}: wal_checkpoint(TRUNCATE)={rows}")
    except sqlite3.Error as exc:
        log(f"{label}: wal_checkpoint(TRUNCATE) failed: {exc!r}")


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

        has_debug_logs = bool(conn.execute(
            "SELECT 1 FROM sqlite_master WHERE type='table' AND name='debug_logs'"
        ).fetchone())
        if has_debug_logs:
            total = conn.execute(
                "SELECT count(*) FROM debug_logs"
            ).fetchone()
            log(f"obsolete debug_logs rows={int(total[0])}; table will be dropped")
        else:
            log("obsolete debug_logs table not found")

        if not args.apply:
            log("dry-run only. Re-run with --apply to modify the database.")
            return 0

        checkpoint_truncate(conn, "before drop")
        if has_debug_logs:
            conn.execute("DROP TABLE debug_logs")
            log("obsolete debug_logs table dropped")
        checkpoint_truncate(conn, "after drop")

        freelist = int(conn.execute("PRAGMA freelist_count").fetchone()[0])
        log(f"post-drop freelist={freelist} ({human_bytes(freelist * page_size)})")

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
