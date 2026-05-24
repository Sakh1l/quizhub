🏠 [Home](../README_FOR_HUMANS.md) · [Getting Started](../GETTING_STARTED.md) · [Architecture](../ARCHITECTURE.md) · [Tech](../TECH_STACK.md) · [Integrations](../INTEGRATIONS.md) · [Repo Map](./repo_map.md) · [Links](../LINKS.md)

---

# Repo Map

A quick tour of the main folders and where to begin reading code.

## Top-level folders

| Folder | Purpose | Start here? |
|---|---|---|
| cmd/ | Application entry points | cmd/server/main.go |
| internal/ | Core server code (db, handlers, middleware, ws, models) | internal/handlers/handlers.go |
| web/ | Embedded static assets and embed.go (go:embed) | web/static/ and web/embed.go |
| frontend/ | Optional frontend dev helpers / package.json (serve) | frontend/package.json |
| .github/ | CI workflows and repo automation | .github/workflows/go.yml |
| .buddy/ | Buddy docs (this folder) | .buddy/README_FOR_HUMANS.md |
| Dockerfile, Makefile | Build and deploy helpers | See Makefile for build/test commands |

## Where to start reading code

1. cmd/server/main.go — program entry; wires everything.
2. internal/handlers/handlers.go — HTTP routes and business logic.
3. internal/db/db.go — migrations and data access.
4. internal/ws/hub.go — WebSocket hub and message broadcasting.
5. web/static — frontend HTML/CSS/JS (player/admin UIs).

If you want a small change, pick a handler or a DB function and run `make test` locally.
