"""pipelines list / get / push / delete.

`push` takes a local file containing the pipeline definition (n8n-format
JSON, or YAML if PyYAML is installed). If the file (or the top-level
object) carries an `id`, the pipeline is updated; otherwise a new one is
created and its id printed. The file is the source of truth — GitOps.
"""

from __future__ import annotations

import json
from pathlib import Path

import typer

from .. import client
from ..utils import confirm, console, die, emit, make_table, print_kv, relative_time, truncate

app = typer.Typer(no_args_is_help=True)


def _load_file(path: Path) -> dict:
    text = path.read_text()
    if path.suffix in (".yaml", ".yml"):
        try:
            import yaml  # type: ignore

            return yaml.safe_load(text)
        except ImportError:
            die("PyYAML not installed — install it or pass a .json file")
    return json.loads(text)


@app.command("list")
def list_pipelines(output: str = typer.Option("table", "--output", "-o", help="table | json | yaml")) -> None:
    """List pipeline definitions."""
    flows = client.get("/api/workflows") or []
    if emit(flows, output):
        return
    if not flows:
        console.print("[dim]no pipelines yet.[/dim]")
        return
    t = make_table("PIPELINE ID", "NAME", "NODES", "STATUS", "UPDATED")
    for f in flows:
        t.add_row(f["id"], f.get("name") or "—", str(f.get("nodeCount", 0)), f.get("status") or "draft", relative_time(f.get("updatedAt")))
    console.print(t)


@app.command("get")
def get_pipeline(
    pipeline_id: str = typer.Argument(...),
    output: str = typer.Option("json", "--output", "-o", help="json | yaml | table"),
) -> None:
    """Dump a pipeline. Default output is JSON (the definition is the point)."""
    p = client.get(f"/api/workflows/{pipeline_id}")
    if output.lower() in ("json", "yaml", "yml"):
        emit(p, output)
        return
    # table summary
    defn = p.get("definition") or {}
    nodes = defn.get("nodes") or []
    print_kv(
        {"id": p.get("id"), "name": p.get("name"), "nodes": len(nodes), "status": p.get("status"), "updated": p.get("updatedAt")},
        title=f"pipeline · {p.get('name')}",
    )
    if nodes:
        t = make_table("NODE", "TYPE", title="nodes")
        for n in nodes:
            t.add_row(n.get("name") or "—", n.get("type") or "—")
        console.print(t)


@app.command("push")
def push_pipeline(
    file: Path = typer.Argument(..., exists=True, readable=True, help="JSON/YAML file with the pipeline definition."),
    pipeline_id: str = typer.Option("", "--id", help="Update this pipeline id (overrides any id in the file)."),
    name: str = typer.Option("", "--name", "-n", help="Override the pipeline name."),
) -> None:
    """Create or update a pipeline from a local definition file."""
    doc = _load_file(file)
    if not isinstance(doc, dict):
        die("file must contain a JSON/YAML object")

    # Two accepted shapes: {id?, name?, definition:{...}} or a bare
    # n8n-format definition {name?, nodes:[...], connections:{...}}.
    if "definition" in doc:
        definition = doc["definition"]
        file_id = doc.get("id")
        file_name = doc.get("name")
    else:
        definition = doc
        file_id = doc.get("id")
        file_name = doc.get("name")

    pid = pipeline_id or (file_id or "")
    body: dict = {"definition": definition}
    if name:
        body["name"] = name
    elif file_name:
        body["name"] = file_name

    if pid:
        client.put(f"/api/workflows/{pid}", json=body)
        console.print(f"[green]✓[/green] updated pipeline [cyan]{pid}[/cyan]")
    else:
        res = client.post("/api/workflows", json=body)
        new_id = res.get("id") if isinstance(res, dict) else None
        console.print(f"[green]✓[/green] created pipeline [cyan]{new_id}[/cyan]")
        console.print(f"  tip: add \"id\": \"{new_id}\" to {file} so the next push updates it in place")


@app.command("delete")
def delete_pipeline(
    pipeline_id: str = typer.Argument(...),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    """Delete a pipeline definition."""
    if not yes and not confirm(f"delete pipeline {pipeline_id}? (envs referencing it will lose it)", default=False):
        die("aborted", code=2)
    client.delete(f"/api/workflows/{pipeline_id}")
    console.print(f"[green]✓[/green] deleted {pipeline_id}")
