"""agents list / get / create / delete / trigger / follow-env / unfollow-env / webhook / test-auth."""

from __future__ import annotations

import typer

from .. import client
from ..utils import confirm, console, die, emit, make_table, print_kv, relative_time, truncate

app = typer.Typer(no_args_is_help=True)


@app.command("list")
def list_agents(output: str = typer.Option("table", "--output", "-o", help="table | json | yaml")) -> None:
    """List all agents."""
    agents = client.get("/api/agents") or []
    if emit(agents, output):
        return
    if not agents:
        console.print("[dim]no agents yet.[/dim]")
        return
    t = make_table("AGENT ID", "NAME", "REPO", "PAT", "WEBHOOK", "ENVS", "UPDATED")
    for a in agents:
        t.add_row(
            a["id"],
            a["name"],
            truncate(a.get("repoUrl"), 44),
            "yes" if a.get("hasPat") else "no",
            "ok" if a.get("webhookInstalled") else "—",
            ", ".join(a.get("environments") or []) or "—",
            relative_time(a.get("updatedAt")),
        )
    console.print(t)


@app.command("get")
def get_agent(
    agent_id: str = typer.Argument(..., help="Agent ID."),
    output: str = typer.Option("table", "--output", "-o", help="table | json | yaml"),
) -> None:
    """Show an agent: repo, auth, webhook, followed environments, credentials."""
    a = client.get(f"/api/agents/{agent_id}")
    if emit(a, output):
        return
    print_kv(
        {
            "id": a["id"],
            "name": a["name"],
            "repo": a.get("repoUrl"),
            "ref": a.get("ref"),
            "pat_set": a.get("hasPat", False),
            "auth": a.get("authStatus") or "untested",
            "auth_checked": a.get("authCheckedAt"),
            "webhook_installed": a.get("webhookInstalled", False),
            "webhook_url": a.get("webhookUrl"),
            "environments": ", ".join(a.get("environments") or []) or "—",
            "created": a.get("createdAt"),
            "updated": a.get("updatedAt"),
        },
        title=f"agent · {a['name']}",
    )
    creds = a.get("credentials") or []
    if creds:
        t = make_table("CRED NAME", "TYPE", "DETAIL", title="agent credential overrides")
        for c in creds:
            if c["type"] == "aws":
                detail = f"{c.get('awsAccountId','')} / {c.get('awsRegion','')}"
            elif c["type"] == "gcp":
                detail = f"project={c.get('gcpProjectId','')}"
            else:
                detail = ",".join(c.get("kvKeys") or [])
            t.add_row(c["name"], c["type"], detail)
        console.print(t)


@app.command("create")
def create_agent(
    repo: str = typer.Option(..., "--repo", "-r", help="Git repository URL."),
    pat: str = typer.Option("", "--pat", help="Personal access token (for private repos)."),
    name: str = typer.Option("", "--name", "-n", help="Override the auto-derived name."),
    ref: str = typer.Option("", "--ref", help="Default git ref (branch). Defaults to main."),
) -> None:
    """Register a new agent repository."""
    body: dict = {"repoUrl": repo}
    if pat:
        body["pat"] = pat
    if name:
        body["name"] = name
    if ref:
        body["ref"] = ref
    a = client.post("/api/agents", json=body)
    console.print(f"[green]✓[/green] created agent [bold]{a['name']}[/bold] · id [cyan]{a['id']}[/cyan]")


@app.command("delete")
def delete_agent(
    agent_id: str = typer.Argument(...),
    yes: bool = typer.Option(False, "--yes", "-y", help="Skip confirmation."),
) -> None:
    """Delete an agent (best-effort uninstalls its webhook first)."""
    if not yes and not confirm(f"delete agent {agent_id}?", default=False):
        die("aborted", code=2)
    client.delete(f"/api/agents/{agent_id}")
    console.print(f"[green]✓[/green] deleted {agent_id}")


@app.command("trigger")
def trigger_agent(
    agent_id: str = typer.Argument(...),
    output: str = typer.Option("table", "--output", "-o", help="table | json"),
) -> None:
    """Dispatch a run across the agent's followed environments' pipelines."""
    res = client.post(f"/api/agents/{agent_id}/trigger")
    if emit(res, output):
        return
    ids = res.get("executionIds") or []
    fails = res.get("failures") or []
    for eid in ids:
        console.print(f"[green]✓[/green] run [cyan]{eid}[/cyan]  (langship runs logs {eid})")
    for f in fails:
        where = "/".join(x for x in (f.get("environment"), f.get("pipelineId")) if x) or "?"
        console.print(f"[yellow]·[/yellow] skipped {where}: {f.get('reason')}" + (f" — {f['error']}" if f.get("error") else ""))
    if not ids and not fails:
        console.print("[dim]nothing dispatched.[/dim]")


@app.command("follow-env")
def follow_env(
    agent_id: str = typer.Argument(...),
    env: str = typer.Argument(..., help="Environment name."),
) -> None:
    """Subscribe the agent to an environment."""
    client.post(f"/api/agents/{agent_id}/environments/{env}")
    console.print(f"[green]✓[/green] {agent_id} now follows env [bold]{env}[/bold]")


@app.command("unfollow-env")
def unfollow_env(
    agent_id: str = typer.Argument(...),
    env: str = typer.Argument(..., help="Environment name."),
) -> None:
    """Unsubscribe the agent from an environment."""
    client.delete(f"/api/agents/{agent_id}/environments/{env}")
    console.print(f"[green]✓[/green] {agent_id} no longer follows env [bold]{env}[/bold]")


@app.command("test-auth")
def test_auth(agent_id: str = typer.Argument(...)) -> None:
    """Probe the agent's PAT against its repo."""
    res = client.post(f"/api/agents/{agent_id}/test-auth")
    status = res.get("authStatus") if isinstance(res, dict) else "?"
    if status == "ok":
        console.print(f"[green]✓[/green] auth ok")
    else:
        console.print(f"[red]✗[/red] auth {status}" + (f" — {res.get('error')}" if isinstance(res, dict) and res.get("error") else ""))


webhook_app = typer.Typer(no_args_is_help=True, help="Install / uninstall the GitHub webhook.")
app.add_typer(webhook_app, name="webhook")


@webhook_app.command("install")
def webhook_install(agent_id: str = typer.Argument(...)) -> None:
    """Install the GitHub push webhook for this agent (server must have FLOW_PUBLIC_URL set)."""
    a = client.post(f"/api/agents/{agent_id}/webhook")
    console.print(f"[green]✓[/green] webhook installed: {a.get('webhookUrl')}")


@webhook_app.command("uninstall")
def webhook_uninstall(agent_id: str = typer.Argument(...)) -> None:
    """Remove the GitHub webhook for this agent."""
    client.delete(f"/api/agents/{agent_id}/webhook")
    console.print(f"[green]✓[/green] webhook uninstalled")
