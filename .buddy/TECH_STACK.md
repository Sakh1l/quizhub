🏠 [Home](./README_FOR_HUMANS.md) · [Getting Started](./GETTING_STARTED.md) · [Architecture](./ARCHITECTURE.md) · [Tech](./TECH_STACK.md) · [Integrations](./INTEGRATIONS.md) · [Repo Map](./MAP/repo_map.md) · [Links](./LINKS.md)

---

# Tech Stack

Collected from go.mod, frontend package.json, Makefile and Dockerfile.

## Languages

- Go 1.24+ (main application and server)
- JavaScript (small frontend, vanilla)

## Libraries / notable deps

- modernc.org/sqlite — pure-Go SQLite driver
- github.com/gorilla/websocket — WebSocket handling
- github.com/google/uuid — UUIDs
- Frontend: `serve` (npm) used only for local static preview (package.json)

## Build & packaging

- Makefile targets: build (go build), run, test, cover
- Dockerfile: multi-stage build (golang:1.24 → alpine)

## Tests

- Go unit tests: `go test ./...` (Makefile: test)

## CI

- GitHub Actions workflow at .github/workflows/go.yml (runs tests and build)

Notes: No heavy JS framework or bundler. The frontend is embedded in the Go binary via go:embed.
