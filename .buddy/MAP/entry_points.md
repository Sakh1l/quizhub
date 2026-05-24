🏠 [Home](../README_FOR_HUMANS.md) · [Getting Started](../GETTING_STARTED.md) · [Architecture](../ARCHITECTURE.md) · [Tech](../TECH_STACK.md) · [Integrations](../INTEGRATIONS.md) · [Repo Map](../MAP/repo_map.md) · [Links](../LINKS.md)

---

# Entry Points

| Entry | File | What it does |
|---|---|---|
| Server (main) | cmd/server/main.go | Builds DB, WebSocket hub, handlers, middleware; serves embedded static files. |
| Frontend preview | frontend/package.json (script: start) | `npx serve public -l 3000` — local preview of static files (dev only). |
| Docker build | Dockerfile | Multi-stage image to produce a small runtime container. |

Short note: run `make build` or `go build -o quizhub ./cmd/server` to build the binary. `make run` runs it.
