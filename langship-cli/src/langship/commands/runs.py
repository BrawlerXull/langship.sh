"""runs list / get / logs.

`logs -f` streams the execution's SSE event feed (/api/executions/{id}/stream)
until the run is done. Without -f it dumps the archived per-node logs from
/api/executions/{id}/logs/{node} for whatever nodes the run record knows about.
"""

from __future__ import annotations

import typer

from .. import client
from ..utils import console, die, emit, make_table, print_kv, relative_time, truncate

app = typer.Typer(no_args_is_help=True)

_LEVEL_COLOR = {
    "error": "red",
    "stderr": "yellow",
    "warn": "yellow",
    "cmd": "cyan",
    "status": "magenta",
}


def _status_mark(s: str | None) -> str:
    s = (s or "").lower()
    if s in ("success", "completed"):
        return "[green]✓[/green]"
    if s in ("failed", "error", "partial_error"):
        return "[red]✗[/red]"
    if s in ("running", "pending"):
        return "[cyan]◌[/cyan]"
    if s in ("paused",):
        return "[yellow]⏸[/yellow]"
    if s in ("cancelled",):
        return "[dim]∅[/dim]"
    return "·"


@app.command("list")
def list_runs(
    limit: int = typer.Option(20, "--limit", "-l"),
    pipeline: str = typer.Option("", "--pipeline", "-p", help="Filter by pipeline id."),
    output: str = typer.Option("table", "--output", "-o", help="table | json | yaml"),
) -> None:
    """List recent runs."""
    params: dict = {"limit": str(limit)}
    if pipeline:
        params["pipeline_id"] = pipeline
    runs = client.get("/api/executions", params=params) or []
    if emit(runs, output):
        return
    if not runs:
        console.print("[dim]no runs yet.[/dim]")
        return
    t = make_table("", "EXECUTION ID", "PIPELINE", "STATUS", "STARTED")
    for r in runs:
        t.add_row(
            _status_mark(r.get("status")),
            r["id"],
            truncate(r.get("pipelineName") or r.get("pipelineId"), 32),
            r.get("status") or "—",
            relative_time(r.get("startedAt")),
        )
    console.print(t)


@app.command("get")
def get_run(
    execution_id: str = typer.Argument(...),
    output: str = typer.Option("table", "--output", "-o", help="table | json | yaml"),
) -> None:
    """Show a run: status + per-node outcomes (from the orchestrator)."""
    r = client.get(f"/api/executions/{execution_id}")
    if emit(r, output):
        return
    if not isinstance(r, dict):
        die("unexpected response")
    print_kv(
        {"execution_id": r.get("execution_id") or execution_id, "status": r.get("status")},
        title=f"run · {execution_id}",
    )
    node_outputs = r.get("node_outputs") or {}
    if node_outputs:
        t = make_table("NODE", "OUTPUTS (keys)", title="node outputs")
        for node, outs in node_outputs.items():
            keys = set()
            for items in (outs or {}).values():
                for it in items or []:
                    if isinstance(it, dict):
                        keys.update(it.keys())
            t.add_row(node, ", ".join(sorted(keys)) or "—")
        console.print(t)
    errs = r.get("errors") or []
    for e in errs:
        console.print(f"[red]error:[/red] {e}")


@app.command("logs")
def logs(
    execution_id: str = typer.Argument(...),
    follow: bool = typer.Option(False, "--follow", "-f", help="Stream the SSE event feed until the run is done."),
) -> None:
    """Print logs for a run. -f streams live; otherwise dumps archived node logs."""
    if follow:
        _stream(execution_id)
        return
    # Non-follow: dump archived per-node logs for whatever nodes we know.
    try:
        r = client.get(f"/api/executions/{execution_id}")
        node_outputs = (r or {}).get("node_outputs") or {}
        nodes = list(node_outputs.keys())
    except Exception:
        nodes = []
    if not nodes:
        console.print("[dim]no node logs available — try `-f` while the run is in progress.[/dim]")
        return
    for node in nodes:
        try:
            text = client.get(f"/api/executions/{execution_id}/logs/{node}")
        except Exception:
            continue
        if not text:
            continue
        console.print(f"[bold]── {node} ──[/bold]")
        # The node-log endpoint returns the raw archived text/JSON; print as-is.
        if isinstance(text, str):
            console.print(text, end="" if text.endswith("\n") else "\n")
        else:
            console.print_json(data=text)


def _stream(execution_id: str) -> None:
    last_status: str | None = None
    for ev in client.stream_sse(f"/api/executions/{execution_id}/stream"):
        if "raw" in ev:
            console.print(ev["raw"])
            continue
        et = ev.get("type")
        node = ev.get("node")
        if et == "node_started":
            console.print(f"[cyan]▶[/cyan] [bold]{node}[/bold] [dim]({ev.get('node_type','')})[/dim]")
        elif et == "node_log":
            line = ev.get("content") or ""
            color = _LEVEL_COLOR.get((ev.get("status") or "").lower(), "white")
            console.print(f"  [bold]{node}[/bold] [{color}]{line}[/{color}]")
        elif et == "node_completed":
            dur = ev.get("duration_ms")
            console.print(f"[green]✓[/green] [bold]{node}[/bold] {ev.get('status','')}" + (f" [dim]({dur}ms)[/dim]" if dur else ""))
        elif et == "node_error":
            console.print(f"[red]✗[/red] [bold]{node}[/bold] {ev.get('error','')}")
        elif et == "done":
            s = ev.get("status")
            if s != last_status:
                console.print(f"[bold]·[/bold] run {s}")
            return
        else:
            # Unknown event type — show it raw so nothing's silently dropped.
            console.print(f"[dim]{ev}[/dim]")
