.PHONY: build run serve test vet tidy clean web web-dev dev watch

BINARY := bin/flow

build: web
	go build -o $(BINARY) ./cmd/flow

# Build only the Go binary, expecting web/dist to already exist.
build-go:
	go build -o $(BINARY) ./cmd/flow

web:
	cd web && (test -d node_modules || npm install --no-fund --no-audit --loglevel=error) && npm run build

web-dev:
	cd web && (test -d node_modules || npm install --no-fund --no-audit --loglevel=error) && npm run dev

# `make dev` runs the Next.js dev server (auto-installs deps).
# In another terminal run `make watch` to hot-reload the Go API on :8080;
# the Next dev rewrite forwards /api/* requests to it.
dev: web-dev

serve: build-go
	./$(BINARY) serve

# Hot-reload the Go server with air. Re-run on every *.go change.
# First time: `go install github.com/air-verse/air@latest`
watch:
	@command -v air >/dev/null 2>&1 || { \
	  echo "installing air..."; \
	  go install github.com/air-verse/air@latest; \
	}
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
