# flow

A lightweight, durable, n8n-compatible workflow engine in Go.

- Single Go binary, single SQLite file — no external services required for the default install
- n8n DSL compatible: paste exported workflow JSON and run it
- Durable execution with crash-safe journal
- First-class human-in-the-loop approvals
- Postgres backend opt-in for multi-instance deployments
- Embeddable as a Go library (`import "github.com/lyzrai/flow/pkg/engine"`)

## Status

Pre-v0.1. Workflow engine extraction in progress. Not yet usable.

## Quickstart

```sh
# build
make build

# run an example workflow
./bin/flow run examples/hello.json
```

## License

Apache 2.0
