"""creds list / get / create / delete — the global credential pool.

Note: the server refuses credential writes unless FLOW_SECRET_KEY is set
(secrets are AES-GCM sealed at rest). Secret values you pass here travel
to the server over HTTP and are never returned by the API afterwards.
"""

from __future__ import annotations

from pathlib import Path

import typer

from .. import client
from ..utils import confirm, console, die, emit, make_table, print_kv, relative_time

app = typer.Typer(no_args_is_help=True)


@app.command("list")
def list_creds(output: str = typer.Option("table", "--output", "-o", help="table | json | yaml")) -> None:
    """List credentials in the global pool."""
    creds = client.get("/api/credentials") or []
    if emit(creds, output):
        return
    if not creds:
        console.print("[dim]no credentials yet.[/dim]")
        return
    t = make_table("NAME", "TYPE", "DETAIL", "UPDATED")
    for c in creds:
        if c["type"] == "aws":
            detail = f"{c.get('awsAccountId','')} / {c.get('awsRegion','')}  role={c.get('awsCrossAccountRoleArn','')}"
        elif c["type"] == "gcp":
            detail = f"project={c.get('gcpProjectId','')} loc={c.get('gcpLocation','') or '—'} sa={'set' if c.get('hasServiceAccount') else 'unset'}"
        else:
            detail = "keys: " + (", ".join(c.get("kvKeys") or []) or "—")
        t.add_row(c["name"], c["type"], detail, relative_time(c.get("updatedAt")))
    console.print(t)


@app.command("get")
def get_cred(
    name: str = typer.Argument(...),
    output: str = typer.Option("table", "--output", "-o", help="table | json | yaml"),
) -> None:
    """Show one credential (non-secret fields + flags only)."""
    c = client.get(f"/api/credentials/{name}")
    if emit(c, output):
        return
    print_kv(c, title=f"credential · {c['name']}")


@app.command("create")
def create_cred(
    name: str = typer.Argument(..., help="Credential name, e.g. prod-aws."),
    type_: str = typer.Option(..., "--type", "-t", help="aws | gcp | kv"),
    # AWS
    aws_region: str = typer.Option("", "--aws-region"),
    aws_account: str = typer.Option("", "--aws-account", help="12-digit account id"),
    aws_role_arn: str = typer.Option("", "--aws-role-arn", help="cross-account role ARN to assume"),
    # GCP
    gcp_project: str = typer.Option("", "--gcp-project"),
    gcp_location: str = typer.Option("", "--gcp-location"),
    gcp_sa_key_file: Path = typer.Option(None, "--gcp-sa-key-file", help="path to service-account JSON"),
    # KV
    kv: list[str] = typer.Option(None, "--kv", help="KEY=value (repeatable)"),
) -> None:
    """Create a credential in the global pool."""
    t = type_.lower()
    body: dict = {"name": name, "type": t}
    if t == "aws":
        if not (aws_region and aws_account and aws_role_arn):
            die("aws credential needs --aws-region, --aws-account, and --aws-role-arn")
        body["awsRegion"] = aws_region
        body["awsAccountId"] = aws_account
        body["awsCrossAccountRoleArn"] = aws_role_arn
    elif t == "gcp":
        if not gcp_project:
            die("gcp credential needs --gcp-project")
        body["gcpProjectId"] = gcp_project
        if gcp_location:
            body["gcpLocation"] = gcp_location
        if gcp_sa_key_file:
            body["gcpServiceAccountJson"] = Path(gcp_sa_key_file).read_text()
    elif t == "kv":
        if not kv:
            die("kv credential needs at least one --kv KEY=value")
        m: dict[str, str] = {}
        for pair in kv:
            if "=" not in pair:
                die(f"--kv must be KEY=value, got {pair!r}")
            k, v = pair.split("=", 1)
            m[k.strip()] = v
        body["kv"] = m
    else:
        die("--type must be aws, gcp, or kv")
    c = client.post("/api/credentials", json=body)
    console.print(f"[green]✓[/green] created credential [bold]{c['name']}[/bold] ({c['type']})")


@app.command("delete")
def delete_cred(
    name: str = typer.Argument(...),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    """Delete a credential from the global pool."""
    if not yes and not confirm(f"delete credential {name}? pipelines referencing it will fail until replaced.", default=False):
        die("aborted", code=2)
    client.delete(f"/api/credentials/{name}")
    console.print(f"[green]✓[/green] deleted {name}")
