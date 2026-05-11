"""Output helpers — Rich tables, error formatting, time formatting."""

from __future__ import annotations

import json as _json
from datetime import datetime, timezone
from typing import Any

import typer
from rich.console import Console
from rich.table import Table

from .client import APIError

console = Console()
err_console = Console(stderr=True)


def die(msg: str, code: int = 1) -> None:
    err_console.print(f"[red]error:[/red] {msg}")
    raise typer.Exit(code=code)


def handle_api_error(err: APIError) -> None:
    msg = err.message
    if err.status == 404:
        die(f"not found: {msg}")
    if err.status in (401, 403):
        die(f"unauthorized: {msg}")
    if err.status == 409:
        die(f"conflict: {msg}")
    if err.status == 503:
        die(f"unavailable: {msg}")
    if err.status >= 500:
        die(f"server error ({err.status}): {msg}")
    die(f"{err.status}: {msg}")


def relative_time(iso: str | None) -> str:
    if not iso:
        return "—"
    try:
        s = iso.replace("Z", "+00:00")
        t = datetime.fromisoformat(s)
        if t.tzinfo is None:
            t = t.replace(tzinfo=timezone.utc)
    except ValueError:
        return iso[:19]
    delta = datetime.now(timezone.utc) - t
    secs = int(delta.total_seconds())
    if secs < 0:
        return "now"
    if secs < 60:
        return "just now"
    mins = secs // 60
    if mins < 60:
        return f"{mins}m ago"
    hours = mins // 60
    if hours < 24:
        return f"{hours}h ago"
    days = hours // 24
    if days < 30:
        return f"{days}d ago"
    return f"{days // 30}mo ago"


def make_table(*columns: str, title: str | None = None) -> Table:
    t = Table(title=title, show_header=True, header_style="bold", box=None, pad_edge=False)
    for c in columns:
        t.add_column(c, overflow="fold")
    return t


def print_kv(d: dict[str, Any], title: str | None = None) -> None:
    t = Table(show_header=False, box=None, pad_edge=False, title=title)
    t.add_column(style="dim")
    t.add_column(overflow="fold")
    for k, v in d.items():
        if isinstance(v, (dict, list)):
            v = _json.dumps(v)
        t.add_row(k, "—" if v is None else str(v))
    console.print(t)


def confirm(msg: str, *, default: bool = False) -> bool:
    return typer.confirm(msg, default=default)


def truncate(s: str | None, n: int) -> str:
    if not s:
        return "—"
    return s if len(s) <= n else s[: n - 1] + "…"


def emit(obj: Any, fmt: str) -> bool:
    """If fmt is json/yaml, print obj and return True (caller should
    return). Otherwise return False so the caller renders a table.
    """
    fmt = (fmt or "").lower()
    if fmt == "json":
        console.print_json(data=obj)
        return True
    if fmt in ("yaml", "yml"):
        try:
            import yaml  # type: ignore

            console.print(yaml.safe_dump(obj, sort_keys=False), end="")
        except ImportError:
            # No PyYAML — fall back to pretty JSON rather than failing.
            console.print_json(data=obj)
        return True
    return False
