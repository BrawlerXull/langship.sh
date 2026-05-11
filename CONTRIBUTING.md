# Contributing to Langship

Thanks for your interest. This guide covers the dev setup, what to run
before opening a PR, and the conventions we follow.

By contributing, you agree your contributions are licensed under the
[Apache License 2.0](./LICENSE) (the project's license). No CLA or DCO
sign-off is required.

## Code of conduct

Participation is governed by [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).
Report concerns to <khush@lyzr.ai>.

## Repo layout

| Path | What |
|---|---|
| `cmd/flow/` | the `flow` server binary (API + Restate worker entry point) |
| `pkg/api/` | REST + SSE handlers |
| `pkg/orchestrator/`, `pkg/engine/` | DAG walk, execution context, events |
| `pkg/executors/` | node implementations (Build, Push, SAST, Approval, Promote, Deploy, …) |
| `pkg/awsdeploy/` | AWS Bedrock AgentCore adapter (STS, ECR, IAM, control-plane SigV4) |
| `pkg/storage/` | Mongo-backed stores (pipelines, runs, agents, credentials, environments) |
| `pkg/secrets/` | AES-GCM seal/open keyed off `FLOW_SECRET_KEY` |
| `web/` | Next.js UI (static export) |
| `langship-cli/` | the `langship` Python CLI |

## Dev environment

Backing services (Mongo, Restate, BuildKit, MinIO, registry) come up via
docker-compose; the Go API runs on the host. See the **Quickstart** and
**Dev (hot reload)** sections in the [README](./README.md) for the exact
commands. In short:

```bash
make services        # docker-compose: mongo, restate, buildkitd, registry, minio
make watch           # Go API with air (rebuilds on .go change)   — or `make serve` for a stable binary
cd web && pnpm dev   # Next dev server, /api proxies to :8090
```

Set `FLOW_SECRET_KEY` in your environment before working on anything that
touches credentials/environments — the API refuses credential writes
without it.

CLI:

```bash
pip install -e ./langship-cli
langship login --api-url http://localhost:8090
```

## Before you open a PR

- **Go**: `go build ./...` and `go test ./...` must pass. Run
  `gofmt`/`go vet` on changed files.
- **Web**: `cd web && pnpm tsc --noEmit` must pass (and `pnpm build` if you
  changed anything that affects the static export).
- **CLI**: `pip install -e ./langship-cli && langship --help` should work;
  smoke-test any command you touched against a local server.
- Keep changes focused — one logical change per PR.
- Update the README / CLI README / `aude.md` if you changed behavior they
  describe.

## Conventions

- **Commits**: imperative subject, ≤ ~72 chars (`feat:`/`fix:`/`refactor:`
  prefixes are welcome but not required). Squash noise before pushing.
- **Branches**: `main` is the trunk; branch off it. Don't push directly to
  `main` — open a PR.
- **Code style**: match the surrounding code. Comments explain *why*, not
  *what*. Executors emit `__<node>` summary objects on output items;
  follow that pattern.
- **Errors**: wrap with context (`fmt.Errorf("deploy: %w", err)`); the API
  layer maps `storage.ErrNotFound` → 404 and `storage.ErrAlreadyExists` →
  409.
- **No secrets in code or commits.** Anything sensitive goes through
  `pkg/secrets` and is never returned by the API.

## Adding a node executor

1. Implement `executors.NodeExecutor` in `pkg/executors/<name>.go`. Read
   the trigger payload via `firstItem(inputs)` for `agentId` /
   `environment` / `fromBranch`. Emit each input item with a `__<name>`
   summary attached.
2. Register it in `pkg/executors/registry.go` (pass any stores it needs
   via `RegistryDeps`).
3. Add a catalog entry in `web/lib/node-catalog.ts` and a `<Name>Form` in
   `web/components/canvas/node-form.tsx`.
4. If it pauses (like Approval), special-case it in
   `pkg/orchestrator/walk.go` so it isn't wrapped in `restate.Run` (it
   calls Restate context methods directly).

## Reporting bugs

Open a GitHub issue with: what you did, what you expected, what happened,
and the relevant logs (`flow` server output + the run's node logs from
`langship runs logs <id>` or `/api/executions/{id}/logs/{node}`).

## Security

Vulnerabilities go to <khush@lyzr.ai> privately — see
[SECURITY.md](./SECURITY.md). Do not open a public issue.
