"""envs list / get / create / update / delete / add-pipeline / remove-pipeline / reorder."""

from __future__ import annotations

import typer

from .. import client
from ..utils import confirm, console, die, emit, make_table, print_kv, relative_time

app = typer.Typer(no_args_is_help=True)


def _pipeline_names() -> dict[str, str]:
    """Map pipeline id -> name (best effort; falls back to id)."""
    try:
        flows = client.get("/api/workflows") or []
        return {f["id"]: f.get("name") or f["id"] for f in flows}
    except Exception:
        return {}


@app.command("list")
def list_envs(output: str = typer.Option("table", "--output", "-o", help="table | json | yaml")) -> None:
    """List all environments."""
    envs = client.get("/api/environments") or []
    if emit(envs, output):
        return
    if not envs:
        console.print("[dim]no environments yet.[/dim]")
        return
    names = _pipeline_names()
    t = make_table("NAME", "DESCRIPTION", "PIPELINES (ORDER)", "UPDATED")
    for e in envs:
        pids = e.get("pipelineIds") or []
        chain = " → ".join(names.get(p, p) for p in pids) or "—"
        t.add_row(e["name"], e.get("description") or "—", chain, relative_time(e.get("updatedAt")))
    console.print(t)


@app.command("get")
def get_env(
    name: str = typer.Argument(..., help="Environment name."),
    output: str = typer.Option("table", "--output", "-o", help="table | json | yaml"),
) -> None:
    """Show an environment and its ordered pipelines."""
    e = client.get(f"/api/environments/{name}")
    if emit(e, output):
        return
    print_kv(
        {"name": e["name"], "description": e.get("description"), "created": e.get("createdAt"), "updated": e.get("updatedAt")},
        title=f"environment · {e['name']}",
    )
    pids = e.get("pipelineIds") or []
    if not pids:
        console.print("[dim]no pipelines in this environment.[/dim]")
        return
    names = _pipeline_names()
    t = make_table("#", "PIPELINE ID", "NAME", title="pipelines (promotion order)")
    for i, pid in enumerate(pids, 1):
        t.add_row(str(i), pid, names.get(pid, pid))
    console.print(t)


@app.command("create")
def create_env(
    name: str = typer.Argument(..., help="Environment name, e.g. dev."),
    description: str = typer.Option("", "--description", "-d"),
) -> None:
    """Create an environment."""
    body: dict = {"name": name}
    if description:
        body["description"] = description
    e = client.post("/api/environments", json=body)
    console.print(f"[green]✓[/green] created environment [bold]{e['name']}[/bold]")


@app.command("update")
def update_env(
    name: str = typer.Argument(...),
    description: str = typer.Option(..., "--description", "-d", help="New description (use \"\" to clear)."),
) -> None:
    """Update an environment's description (name is immutable)."""
    e = client.put(f"/api/environments/{name}", json={"name": name, "description": description})
    console.print(f"[green]✓[/green] updated [bold]{e['name']}[/bold]")


@app.command("delete")
def delete_env(
    name: str = typer.Argument(...),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    """Delete an environment."""
    if not yes and not confirm(f"delete environment {name}? agents following it stop dispatching its pipelines.", default=False):
        die("aborted", code=2)
    client.delete(f"/api/environments/{name}")
    console.print(f"[green]✓[/green] deleted {name}")


@app.command("add-pipeline")
def add_pipeline(
    name: str = typer.Argument(..., help="Environment name."),
    pipeline_id: str = typer.Argument(..., help="Pipeline ID to append."),
) -> None:
    """Bring a pipeline into the environment (appended to the end of the order)."""
    e = client.post(f"/api/environments/{name}/pipelines/{pipeline_id}")
    console.print(f"[green]✓[/green] {pipeline_id} added to [bold]{name}[/bold] ({len(e.get('pipelineIds') or [])} total)")


@app.command("remove-pipeline")
def remove_pipeline(
    name: str = typer.Argument(...),
    pipeline_id: str = typer.Argument(...),
) -> None:
    """Remove a pipeline from the environment."""
    client.delete(f"/api/environments/{name}/pipelines/{pipeline_id}")
    console.print(f"[green]✓[/green] {pipeline_id} removed from [bold]{name}[/bold]")


@app.command("reorder")
def reorder(
    name: str = typer.Argument(...),
    pipeline_ids: list[str] = typer.Argument(..., help="The full pipeline id list in the new order."),
) -> None:
    """Set the promotion order. Must be a permutation of the env's current pipelines."""
    e = client.put(f"/api/environments/{name}/pipelines", json={"pipelineIds": pipeline_ids})
    names = _pipeline_names()
    console.print(f"[green]✓[/green] reordered [bold]{name}[/bold]: " + " → ".join(names.get(p, p) for p in e.get("pipelineIds") or []))
