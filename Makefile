.PHONY: build run serve test vet tidy clean web web-dev dev

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

# `make dev` runs the Vite dev server (auto-installs deps).
# In another terminal run `make serve` to start the Go API on :8080;
# the Vite proxy forwards /api/* requests to it.
dev: web-dev

serve: build-go
	./$(BINARY) serve

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
