# --- web build stage ---
FROM node:22-alpine AS web
WORKDIR /app
COPY web/package.json web/package-lock.json* ./web/
RUN cd web && (test -f package-lock.json && npm ci --no-fund --no-audit || npm install --no-fund --no-audit)
COPY web ./web
RUN cd web && npm run build

# --- go build stage ---
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/out ./web/out
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/flow ./cmd/flow

# --- runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/flow /usr/local/bin/flow
EXPOSE 8080 9080
ENTRYPOINT ["/usr/local/bin/flow"]
CMD ["serve"]
