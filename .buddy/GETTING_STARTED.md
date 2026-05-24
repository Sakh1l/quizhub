🏠 [Home](./README_FOR_HUMANS.md) · [Getting Started](./GETTING_STARTED.md) · [Architecture](./ARCHITECTURE.md) · [Tech](./TECH_STACK.md) · [Integrations](./INTEGRATIONS.md) · [Repo Map](./MAP/repo_map.md) · [Links](./LINKS.md)

---

# Getting Started

Short and exact steps to run the project locally.

## Prerequisites

- Go 1.24+ (required to build the binary)
- Docker (optional, recommended for container runs)
- (Node is only used for a lightweight frontend dev server; not required to run the Go binary)

## Clone & build

```bash
git clone https://github.com/sakh1l/quizhub.git
cd quizhub
make build    # builds the quizhub binary (uses: go build -o quizhub ./cmd/server)
```

## Run locally

```bash
# run the built binary
./quizhub
# or with Makefile shortcut
make run
```

Open in browser:
- Players: http://localhost:8080
- Admin:   http://localhost:8080/admin.html  (PIN: 1234)

## Run tests

```bash
make test        # runs `go test ./... -v -count=1`
make cover       # coverage report (go tool cover -html=coverage.out)
```

## Docker (quick)

```bash
docker build -t quizhub .
docker run -d -p 8080:8080 -v quizhub-data:/app/data -e QUIZHUB_ADMIN_PIN=your-pin quizhub
```

## Common env vars (where to set them)

- QUIZHUB_PORT (default: 8080) — used in cmd/server/main.go
- QUIZHUB_DB (default: quizhub.db) — SQLite file path
- QUIZHUB_ADMIN_PIN (default: 1234) — admin PIN
- QUIZHUB_ALLOWED_ORIGINS — comma list for WebSocket origin checks
- QUIZHUB_TRUST_PROXY — set to true if behind a trusted proxy

## Troubleshooting quick fixes

- bind: address already in use → change port: `QUIZHUB_PORT=3000 ./quizhub`
- WebSocket not connecting → ensure your reverse proxy forwards Upgrade headers (see README.md)

Where to look in code:
- entry: cmd/server/main.go
- DB layer: internal/db/db.go
- HTTP handlers: internal/handlers/handlers.go
- WebSocket: internal/ws/hub.go
- Frontend files: web/static/*.html, web/static/*.js
