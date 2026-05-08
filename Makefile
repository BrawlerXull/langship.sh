.PHONY: build run serve test vet tidy clean web web-dev dev watch

BINARY := bin/flow

# --- env defaults for `make watch` ----------------------------------------
# These match the host-side ports published by docker-compose.yml. Override
# any value at the command line, e.g. `make watch MINIO_ENDPOINT=...`.
#
# Anything you `export` separately in your shell still takes precedence over
# these, since the Go binary only reads its env via os.Getenv.
export FLOW_ADDR             ?= :8090
export FLOW_CORS_ORIGINS     ?= http://localhost:3000
export MONGO_URI             ?= mongodb://localhost:27017
export MONGO_DB              ?= flow
export RESTATE_INGRESS_URL   ?= http://localhost:8081
export RESTATE_ADMIN_URL     ?= http://localhost:9070
export RESTATE_DEPLOYMENT_URI ?= http://host.docker.internal:9080
export BUILDKIT_HOST         ?= tcp://127.0.0.1:1234
export MINIO_ENDPOINT        ?= 127.0.0.1:9000
export MINIO_ACCESS_KEY      ?= minio
export MINIO_SECRET_KEY      ?= minio12345
export MINIO_BUCKET          ?= flow-logs
export MINIO_USE_SSL         ?= false

build: web
	go build -o $(BINARY) ./cmd/flow

# Build only the Go binary, expecting web/dist to already exist.
build-go:
	go build -o $(BINARY) ./cmd/flow

web:
	cd web && (test -d node_modules || npm install --no-fund --no-audit --loglevel=error) && npm run build

web-dev:
	# NEXT_PUBLIC_FLOW_API_URL points the SSE EventSource straight at the
	# Go API so streams skip Next's trailingSlash 308 redirect (which
	# EventSource doesn't follow). Override at the command line if your
	# Go API runs elsewhere.
	cd web && (test -d node_modules || npm install --no-fund --no-audit --loglevel=error) \
	  && NEXT_PUBLIC_FLOW_API_URL=$${NEXT_PUBLIC_FLOW_API_URL:-http://localhost:8090} npm run dev

# `make dev` runs the Next.js dev server (auto-installs deps).
# In another terminal run `make watch` to hot-reload the Go API on :8090;
# the Next dev rewrite forwards /api/* requests to it.
dev: web-dev

serve: build-go
	./$(BINARY) serve

# Hot-reload the Go server with air. Re-run on every *.go change.
# Picks up MINIO_*, MONGO_*, RESTATE_*, BUILDKIT_HOST, FLOW_* defaults from
# this Makefile so a fresh shell can `make watch` without exporting anything.
# First time: `go install github.com/air-verse/air@latest`
watch:
	@command -v air >/dev/null 2>&1 || { \
	  echo "installing air..."; \
	  go install github.com/air-verse/air@latest; \
	}
	@echo "flow env:"
	@echo "  FLOW_ADDR           = $(FLOW_ADDR)"
	@echo "  MONGO_URI           = $(MONGO_URI)"
	@echo "  RESTATE_INGRESS_URL = $(RESTATE_INGRESS_URL)"
	@echo "  BUILDKIT_HOST       = $(BUILDKIT_HOST)"
	@echo "  MINIO_ENDPOINT      = $(MINIO_ENDPOINT)"
	air

run: build
	./$(BINARY)

test:
	go test ./... -count=1 -race

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/ web/dist web/node_modules *.db *.db-shm *.db-wal coverage.out
