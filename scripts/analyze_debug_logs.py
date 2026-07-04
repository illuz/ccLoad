#!/usr/bin/env python3
"""Analyze ccLoad debug_logs and emit per-log JSON analysis files.

This tool is intentionally independent from the Go server. It opens the
SQLite database read-only, extracts user prompts and file-related AI tool
calls from captured upstream request/response bodies, and writes JSON files
under data/debug-analysis by default.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import re
import sqlite3
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

FILE_ARG_KEYS = {
    "file_path", "filepath", "path", "absolute_path", "relative_path",
    "notebook_path", "target_file", "target_path", "pattern", "glob",
}
TOOL_NAME_HINTS = (
    "read", "write", "edit", "multiedit", "glob", "grep", "ls", "list", "find",
    "view", "open", "cat", "sed", "apply_patch",
)
PATH_RE = re.compile(r"(?:^|[\s'\"`])((?:[A-Za-z0-9_.-]+/)+[A-Za-z0-9_.@()+,=:\\-]+)(?=$|[\s'\"`,})\]])")


def decode_blob(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, bytes):
        try:
            return value.decode("utf-8")
        except UnicodeDecodeError:
            return base64.b64encode(value).decode("ascii")
    return str(value)


def parse_json_maybe(text: str) -> Any | None:
    if not text:
        return None
    try:
        return json.loads(text)
    except Exception:
        return None


def parse_sse_json_events(text: str) -> list[Any]:
    events: list[Any] = []
    if not text or "data:" not in text:
        return events
    for block in text.split("\n\n"):
        data_lines: list[str] = []
        for line in block.splitlines():
            if line.startswith("data:"):
                data_lines.append(line[5:].lstrip())
        if not data_lines:
            continue
        data = "\n".join(data_lines).strip()
        if not data or data == "[DONE]":
            continue
        parsed = parse_json_maybe(data)
        if parsed is not None:
            events.append(parsed)
    return events


def iter_messages(obj: Any):
    if isinstance(obj, dict):
        # Chat Completions style: {messages:[...]}
        msgs = obj.get("messages")
        if isinstance(msgs, list):
            for m in msgs:
                if isinstance(m, dict):
                    yield m
        # Responses API style: {input:[{type:"message", role:"user", ...}, ...]}
        inputs = obj.get("input")
        if isinstance(inputs, list):
            for item in inputs:
                if isinstance(item, dict) and item.get("type") == "message":
                    yield item
        # A message object may appear directly in response.completed output.
        if obj.get("type") == "message" and isinstance(obj.get("role"), str):
            yield obj
        for v in obj.values():
            if isinstance(v, (dict, list)):
                yield from iter_messages(v)
    elif isinstance(obj, list):
        for item in obj:
            yield from iter_messages(item)


def text_from_content(content: Any) -> str:
    parts: list[str] = []
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        for item in content:
            if isinstance(item, str):
                parts.append(item)
            elif isinstance(item, dict):
                if isinstance(item.get("text"), str):
                    parts.append(item["text"])
                elif isinstance(item.get("type"), str) and isinstance(item.get("text"), str):
                    parts.append(item["text"])
                elif isinstance(item.get("content"), str):
                    parts.append(item["content"])
    elif isinstance(content, dict):
        if isinstance(content.get("text"), str):
            return content["text"]
        if isinstance(content.get("content"), str):
            return content["content"]
    return "\n".join(p for p in parts if p)


def extract_user_questions(req_obj: Any) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for m in iter_messages(req_obj):
        if m.get("role") != "user":
            continue
        text = text_from_content(m.get("content"))
        if text.strip():
            out.append({"index": len(out), "role": "user", "content": text})
    return out


def safe_json_arg(value: Any) -> Any:
    if isinstance(value, str):
        parsed = parse_json_maybe(value)
        return parsed if parsed is not None else value
    return value



def append_unique_text(out: list[dict[str, Any]], text: str, source: str) -> None:
    text = (text or "").strip()
    if not text:
        return
    if any(item.get("content") == text for item in out):
        return
    out.append({"index": len(out), "source": source, "content": text})


def extract_ai_texts(obj: Any, out: list[dict[str, Any]] | None = None) -> list[dict[str, Any]]:
    """Extract assistant-visible text from non-stream JSON response objects.

    The analyzer intentionally records text only, not code edits as separate
    structured content. If the model's final answer contains code fences they
    remain part of the text because that is what the user saw.
    """
    if out is None:
        out = []
    if isinstance(obj, dict):
        # Chat Completions full response: choices[].message.content
        choices = obj.get("choices")
        if isinstance(choices, list):
            for choice in choices:
                if not isinstance(choice, dict):
                    continue
                msg = choice.get("message")
                if isinstance(msg, dict) and msg.get("role") == "assistant":
                    append_unique_text(out, text_from_content(msg.get("content")), "choices.message")

        obj_type = obj.get("type")
        # Responses API message object: {type:"message", role:"assistant", content:[...]}
        if obj_type == "message" and obj.get("role") == "assistant":
            append_unique_text(out, text_from_content(obj.get("content")), "response.message")

        # Responses content part: {type:"output_text", text:"..."}
        if obj_type == "output_text" and isinstance(obj.get("text"), str):
            append_unique_text(out, obj["text"], "output_text")

        # Streaming Responses done event: {type:"response.output_text.done", text:"..."}
        if obj_type == "response.output_text.done" and isinstance(obj.get("text"), str):
            append_unique_text(out, obj["text"], "response.output_text.done")

        # Streaming output item wrapper.
        if obj_type == "response.output_item.done" and isinstance(obj.get("item"), dict):
            extract_ai_texts(obj["item"], out)

        for v in obj.values():
            if isinstance(v, (dict, list)):
                extract_ai_texts(v, out)
    elif isinstance(obj, list):
        for item in obj:
            extract_ai_texts(item, out)
    return out


def extract_ai_texts_from_events(events: list[Any]) -> list[dict[str, Any]]:
    out = extract_ai_texts(events)

    # Fallback for Responses API streams that only include deltas and no done/message object.
    response_buffers: dict[str, list[str]] = {}
    response_order: list[str] = []

    # Fallback for Chat Completions streams:
    # data: {"object":"chat.completion.chunk","choices":[{"delta":{"content":"..."}}]}
    chat_buffers: dict[str, list[str]] = {}
    chat_order: list[str] = []

    for event in events:
        if not isinstance(event, dict):
            continue

        if event.get("type") == "response.output_text.delta":
            delta = event.get("delta")
            if isinstance(delta, str):
                item_id = str(event.get("item_id") or event.get("output_index") or "default")
                if item_id not in response_buffers:
                    response_buffers[item_id] = []
                    response_order.append(item_id)
                response_buffers[item_id].append(delta)

        choices = event.get("choices")
        if isinstance(choices, list):
            for choice in choices:
                if not isinstance(choice, dict):
                    continue
                delta_obj = choice.get("delta")
                if not isinstance(delta_obj, dict):
                    continue
                content = delta_obj.get("content")
                if not isinstance(content, str):
                    continue
                choice_id = str(choice.get("index", 0))
                if choice_id not in chat_buffers:
                    chat_buffers[choice_id] = []
                    chat_order.append(choice_id)
                chat_buffers[choice_id].append(content)

    for item_id in response_order:
        append_unique_text(out, "".join(response_buffers[item_id]), f"response.output_text.delta:{item_id}")
    for choice_id in chat_order:
        append_unique_text(out, "".join(chat_buffers[choice_id]), f"chat.completion.delta:{choice_id}")
    return out


def collect_tool_calls(obj: Any, out: list[dict[str, Any]] | None = None) -> list[dict[str, Any]]:
    if out is None:
        out = []
    if isinstance(obj, dict):
        # OpenAI-style tool_calls
        tcs = obj.get("tool_calls")
        if isinstance(tcs, list):
            for tc in tcs:
                if not isinstance(tc, dict):
                    continue
                fn = tc.get("function") if isinstance(tc.get("function"), dict) else {}
                name = fn.get("name") or tc.get("name") or tc.get("type")
                args = safe_json_arg(fn.get("arguments") or tc.get("arguments") or tc.get("input") or {})
                out.append({"name": name or "", "arguments": args})
        # OpenAI Responses API function_call item
        obj_type = obj.get("type")
        if isinstance(obj_type, str) and obj_type == "function_call":
            out.append({
                "name": obj.get("name") or "function_call",
                "arguments": safe_json_arg(obj.get("arguments") or {}),
            })
        # Streaming Responses API output_item.done wrapper
        if isinstance(obj_type, str) and obj_type == "response.output_item.done" and isinstance(obj.get("item"), dict):
            item = obj["item"]
            if item.get("type") == "function_call":
                out.append({
                    "name": item.get("name") or "function_call",
                    "arguments": safe_json_arg(item.get("arguments") or {}),
                })
        # Anthropic-style tool_use blocks
        if isinstance(obj_type, str) and obj_type in {"tool_use", "server_tool_use"}:
            out.append({"name": obj.get("name") or obj_type or "", "arguments": obj.get("input") or {}})
        # Codex/Claude result-ish blocks may include name/input directly
        if isinstance(obj.get("name"), str) and isinstance(obj.get("input"), dict):
            lname = obj["name"].lower()
            if any(h in lname for h in TOOL_NAME_HINTS):
                out.append({"name": obj["name"], "arguments": obj.get("input") or {}})
        for v in obj.values():
            if isinstance(v, (dict, list)):
                collect_tool_calls(v, out)
    elif isinstance(obj, list):
        for item in obj:
            collect_tool_calls(item, out)
    return out


def normalize_path(p: str) -> str | None:
    p = p.strip().strip("'\"` ,;:()[]{}")
    if not p or "://" in p:
        return None
    if p.startswith("/"):
        return p
    if "/" not in p:
        return None
    if any(part in {"..", ""} for part in p.split("/")):
        # Keep absolute paths, but avoid odd relative traversals/noise.
        if not p.startswith("/"):
            return None
    return p


def paths_from_value(value: Any) -> list[str]:
    paths: list[str] = []
    if isinstance(value, str):
        # Treat the whole string as a path only when it looks like a single path.
        # Shell commands often contain slashes in regexes/arguments; parsing the
        # entire command as one path creates noisy pseudo directories.
        if not any(ch.isspace() for ch in value) and not any(ch in value for ch in '|;&<>'):
            maybe = normalize_path(value)
            if maybe:
                paths.append(maybe)
        for match in PATH_RE.findall(value):
            maybe = normalize_path(match)
            if maybe:
                paths.append(maybe)
    elif isinstance(value, list):
        for item in value:
            paths.extend(paths_from_value(item))
    elif isinstance(value, dict):
        for k, v in value.items():
            if k in FILE_ARG_KEYS or isinstance(v, (dict, list)):
                paths.extend(paths_from_value(v))
            elif isinstance(v, str):
                paths.extend(paths_from_value(v))
    return paths


def infer_type(path: str) -> str:
    base = path.rstrip("/").split("/")[-1]
    if path.endswith("/") or "." not in base:
        return "directory"
    return "file"


def build_tree(paths: list[str]) -> str:
    root: dict[str, Any] = {}
    for p in paths:
        clean = p.lstrip("/") or p
        cur = root
        for part in clean.split("/"):
            cur = cur.setdefault(part, {})
    lines: list[str] = []
    def walk(node: dict[str, Any], indent: int = 0):
        for name in sorted(node):
            lines.append("  " * indent + name)
            walk(node[name], indent + 1)
    walk(root)
    return "\n".join(lines)


def analyze_row(row: sqlite3.Row, db_path: str) -> dict[str, Any]:
    req_text = decode_blob(row["req_body"])
    resp_text = decode_blob(row["resp_body"])
    req_obj = parse_json_maybe(req_text)
    resp_obj = parse_json_maybe(resp_text)
    resp_events: list[Any] = []
    errors: list[str] = []
    if req_obj is None and req_text:
        errors.append("req_body is not valid JSON")
    if resp_obj is None and resp_text:
        resp_events = parse_sse_json_events(resp_text)
        if not resp_events:
            errors.append("resp_body is not valid JSON")

    user_questions = extract_user_questions(req_obj) if req_obj is not None else []
    tool_calls = []
    if req_obj is not None:
        tool_calls.extend(collect_tool_calls(req_obj))
    if resp_obj is not None:
        tool_calls.extend(collect_tool_calls(resp_obj))
    if resp_events:
        tool_calls.extend(collect_tool_calls(resp_events))

    ai_texts: list[dict[str, Any]] = []
    if resp_obj is not None:
        ai_texts = extract_ai_texts(resp_obj)
    if resp_events:
        ai_texts = extract_ai_texts_from_events(resp_events)
    final_ai_text = ai_texts[-1]["content"] if ai_texts else ""

    seen_paths: dict[str, dict[str, Any]] = {}
    for tc in tool_calls:
        for p in paths_from_value(tc.get("arguments")):
            seen_paths.setdefault(p, {"path": p, "type": infer_type(p), "source": "tool_call"})
    paths = sorted(seen_paths)

    return {
        "log_id": row["log_id"],
        "created_at": row["created_at"],
        "analyzed_at": datetime.now(timezone.utc).isoformat(),
        "source": {"db_path": db_path, "debug_table": "debug_logs"},
        "user_questions": user_questions,
        "tool_file_tree": {
            "summary": "由 AI tool 调用参数推断出的文件/目录结构",
            "paths": [seen_paths[p] for p in paths],
            "tree_text": build_tree(paths),
        },
        "ai_texts": ai_texts,
        "final_ai_text": final_ai_text,
        "tool_calls": tool_calls,
        "errors": errors,
    }


def connect_ro(db_path: str) -> sqlite3.Connection:
    uri = f"file:{Path(db_path).resolve()}?mode=ro"
    conn = sqlite3.connect(uri, uri=True, timeout=5)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA query_only = ON")
    conn.execute("PRAGMA busy_timeout = 5000")
    return conn


def rows_to_analyze(
    conn: sqlite3.Connection,
    log_id: int | None,
    since_log_id: int,
    limit: int,
    min_created_at: int | None,
):
    if log_id:
        return conn.execute(
            "SELECT log_id, created_at, req_body, resp_body FROM debug_logs WHERE log_id = ?",
            (log_id,),
        ).fetchall()
    if min_created_at:
        return conn.execute(
            """
            SELECT log_id, created_at, req_body, resp_body
            FROM debug_logs
            WHERE log_id > ? AND created_at >= ?
            ORDER BY log_id
            LIMIT ?
            """,
            (since_log_id, min_created_at, limit),
        ).fetchall()
    return conn.execute(
        """
        SELECT log_id, created_at, req_body, resp_body
        FROM debug_logs
        WHERE log_id > ?
        ORDER BY log_id
        LIMIT ?
        """,
        (since_log_id, limit),
    ).fetchall()


def cleanup_old_outputs(out_dir: Path, retention_days: float) -> int:
    if retention_days <= 0:
        return 0
    cutoff = time.time() - retention_days * 24 * 60 * 60
    removed = 0
    for path in out_dir.glob("*.json"):
        try:
            if not path.is_file():
                continue
            if path.stat().st_mtime < cutoff:
                path.unlink()
                removed += 1
        except OSError:
            continue
    if removed:
        print(f"cleaned {removed} old analysis file(s), output={out_dir}")
    return removed


def run_once(args) -> int:
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    min_created_at = None
    if args.retention_days > 0 and not args.log_id:
        min_created_at = int(time.time() - args.retention_days * 24 * 60 * 60)
    conn = connect_ro(args.db)
    try:
        rows = rows_to_analyze(conn, args.log_id, args.since_log_id, args.limit, min_created_at)
    finally:
        conn.close()
    count = 0
    for row in rows:
        out_file = out_dir / f"{row['log_id']}.json"
        if out_file.exists() and not args.force:
            continue
        result = analyze_row(row, args.db)
        tmp = out_file.with_suffix(".json.tmp")
        tmp.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        os.replace(tmp, out_file)
        count += 1
    print(f"analyzed {count} log(s), output={out_dir}")
    return count


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Analyze ccLoad SQLite debug_logs into JSON files")
    parser.add_argument("--db", default=os.environ.get("SQLITE_PATH", "data/ccload.db"), help="SQLite DB path")
    parser.add_argument("--out-dir", default="data/debug-analysis", help="JSON output directory")
    parser.add_argument("--log-id", type=int, help="Analyze one log_id")
    parser.add_argument("--since-log-id", type=int, default=0, help="Analyze log_id greater than this value")
    parser.add_argument("--limit", type=int, default=100, help="Max rows per run")
    parser.add_argument("--force", action="store_true", help="Overwrite existing JSON files")
    parser.add_argument("--watch", action="store_true", help="Poll continuously")
    parser.add_argument("--follow", action="store_true", help="Poll continuously (alias that avoids PM2 --watch handling)")
    parser.add_argument("--interval", type=float, default=5.0, help="Watch poll interval seconds")
    parser.add_argument(
        "--retention-days",
        type=float,
        default=float(os.environ.get("DEBUG_ANALYSIS_RETENTION_DAYS", "5")),
        help="Delete analysis JSON files older than this many days and skip older logs; <=0 disables cleanup",
    )
    parser.add_argument(
        "--cleanup-interval",
        type=float,
        default=float(os.environ.get("DEBUG_ANALYSIS_CLEANUP_INTERVAL", "3600")),
        help="Seconds between output retention cleanup runs in watch/follow mode",
    )
    args = parser.parse_args(argv)

    args.watch = bool(args.watch or args.follow)
    if args.watch and args.log_id:
        parser.error("--watch/--follow cannot be combined with --log-id")

    last = args.since_log_id
    last_cleanup = 0.0
    while True:
        now = time.time()
        if args.retention_days > 0 and (
            last_cleanup == 0.0 or not args.watch or now - last_cleanup >= args.cleanup_interval
        ):
            cleanup_old_outputs(Path(args.out_dir), args.retention_days)
            last_cleanup = now
        args.since_log_id = last
        run_once(args)
        # Advance by output filenames so skipped existing files do not block watch mode.
        out_dir = Path(args.out_dir)
        ids = [int(p.stem) for p in out_dir.glob("*.json") if p.stem.isdigit()]
        if ids:
            last = max(last, max(ids))
        if not args.watch:
            break
        time.sleep(args.interval)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
