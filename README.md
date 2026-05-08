# flow

A durable agent-pipeline runtime — the engine + control plane behind
[Langship](./aude.md) (Deployment / Governance / Operations for agent apps).

- **Pipeline canvas**: drag-and-drop CI/CD nodes (Trigger → Build → Test →
  Eval → Policy → Approval → Deploy → Promote → Rollback)
- **n8n-shaped JSON** as the on-disk pipeline format; flow's own DAG +
  executor catalog at runtime
- **Restate-backed durability**: every node wrapped in `restate.Run` —
  crash-safe journaling, replay, awakeable-based human approvals
- **Real BuildKit OCI builds** with private-repo PAT support; pushes to
  GHCR or any registry
- **GitHub webhook receiver** with HMAC verification, agent ↔ pipeline
  attachments, branch filtering
- **Live execution view**: SSE-streamed per-node status + per-node log
  lines (BuildKit progress, shell stdout, stub events), canvas-overlay
  status rings

## Status

Pre-v0.1. Working but moving fast — APIs and node types may change.

## Architecture

```
┌────────────────┐    ┌────────────────┐
│  web (nginx)   │    │  flow API (Go) │      ┌──────────────┐
│  Next static   │◄──►│   :8090        │◄────►│   Mongo      │
│  :3000         │    │  /api/* CRUD   │      │   pipelines  │
└────────────────┘    │  /api/.../stream      │   runs       │
                      │  SSE (live)    │      │   agents     │
                      └────────┬───────┘      └──────────────┘
                               │
                               │ ingress.Send  ┌──────────────┐
                               ├──────────────►│   Restate    │
                               │               │   :8081 ing  │
                               │ (executes via │   :9070 admin│
                               │  callback)    └──────┬───────┘
                               │                      │
                               │     ┌────────────────┘
                               ▼     ▼ workflow callback
                      ┌──────────────────────┐
                      │ flow service :9080   │  walkDurable + executors
                      │  Trigger/Build/Test/ │  → BuildKit (tcp:1234) for OCI
                      │  Eval/Policy/Approve │  → Registry (tcp:5000) for push
                      │  /Deploy/Promote/RB  │
                      └──────────────────────┘
```

## Quickstart

The default `docker compose up` brings the **API + UI + backing stores**
(Mongo, Restate). It assumes you have BuildKit + a registry running on
the host already (e.g. via the sibling `langship` stack).

```sh
docker compose up
# UI:        http://localhost:3000
# API:       http://localhost:8090
# Restate:   :8081 ingress, :9070 admin
```

If you **don't** have BuildKit + registry running, layer the standalone
overlay to bring them up too:

```sh
docker compose -f docker-compose.yml -f docker-compose.standalone.yml up
# adds:
#   buildkitd  127.0.0.1:1234   (moby/buildkit:v0.18.2)
#   registry   127.0.0.1:5000   (registry:2)
```

## Dev (hot reload)

Three terminals:

```sh
# 1) backing services
docker compose up -d mongo restate
# (and buildkitd/registry from the standalone overlay or the sibling stack)

# 2) Go API with air (rebuilds on .go changes)
make watch

# 3) Next dev server with HMR; /api proxies to :8090
make dev
```

Open `http://localhost:3000`.

## Env vars

The flow process (`./bin/flow serve` or `make watch`):

| Var | Default | Notes |
|---|---|---|
| `FLOW_ADDR` | `:8090` | API listen address |
| `FLOW_CORS_ORIGINS` | `*` | CSV allowlist |
| `FLOW_PUBLIC_URL` | (empty) | Externally-reachable base URL — used to render webhook callback URLs. Set to your `cloudflared` tunnel for GitHub webhooks. |
| `MONGO_URI` | (required) | e.g. `mongodb://localhost:27017` |
| `MONGO_DB` | `flow` | |
| `RESTATE_INGRESS_URL` | `http://localhost:8081` | |
| `RESTATE_ADMIN_URL` | `http://localhost:9070` | |
| `RESTATE_SERVICE_ADDR` | `:9080` | Service-endpoint listen addr |
| `RESTATE_DEPLOYMENT_URI` | `http://host.docker.internal:9080` | How Restate reaches us. In docker-compose this is overridden to `http://flow:9080`. |
| `BUILDKIT_HOST` | `tcp://127.0.0.1:1234` | BuildKit gRPC. In docker-compose: `tcp://buildkitd:1234` (or `host.docker.internal` when buildkitd is external). |

## Concepts

- **Agent** — a registered git repo (URL + PAT). One-click GitHub webhook
  install; `/webhooks/github/{id}` verifies HMAC and dispatches runs on
  push. Agents attach to pipelines.
- **Pipeline** — a DAG of nodes built on the canvas (n8n-shape JSON
  underneath). Saved to Mongo; loaded fresh per run.
- **Run** — one execution of a pipeline. Restate journals each node
  (`restate.Run("node:<name>", fn)`). Terminal status is written back to
  Mongo's `runs` collection.
- **Live view** — `/executions/view?id=…` subscribes to
  `/api/executions/{id}/stream` (SSE) for `node_started`,
  `node_completed`, `node_error`, **`node_log`**, and `done` events.

## Build node

Two modes:

- `mode: "docker"` — BuildKit solves the Dockerfile against the cloned
  repo and pushes to a registry. Auth: GHCR uses the agent's PAT
  (`write:packages`); `localhost:*` / `registry:*` are anonymous +
  insecure. Streams BuildKit's plain-mode progress as `node_log` events.
- `mode: "shell"` — escape hatch, runs `/bin/sh -c <command>` in the
  cloned repo. Stdout/stderr line-streamed to the log channel.

## License

Apache 2.0
