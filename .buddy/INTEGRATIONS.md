🏠 [Home](./README_FOR_HUMANS.md) · [Getting Started](./GETTING_STARTED.md) · [Architecture](./ARCHITECTURE.md) · [Tech](./TECH_STACK.md) · [Integrations](./INTEGRATIONS.md) · [Repo Map](./MAP/repo_map.md) · [Links](./LINKS.md)

---

# External Integrations

This project aims for zero external runtime dependencies. The only runtime integrations are environment-configured behaviors and optional reverse proxies.

| Service | What it's used for | Where configured |
|---|---|---|
| SQLite (embedded) | Persistent storage for players, questions, answers, game_state | internal/db/db.go (migrations); QUIZHUB_DB env var in cmd/server/main.go |
| Reverse proxy / TLS | Optional: Nginx/Caddy in front to provide HTTPS and proxy WebSocket Upgrade headers | Deployment notes in README.md and Docker compose example (docker-compose.yml / fly.toml) |

## Environment variables (where to set)

- QUIZHUB_PORT — server port (cmd/server/main.go)
- QUIZHUB_DB — SQLite file path (cmd/server/main.go)
- QUIZHUB_ADMIN_PIN — admin PIN (handlers.New)
- QUIZHUB_ADMIN_TOKEN_TTL_MIN — admin token TTL in minutes (handlers.New)
- QUIZHUB_ALLOWED_ORIGINS — comma-separated origins for WebSocket (internal/ws/isAllowedOrigin)
- QUIZHUB_TRUST_PROXY — if true, use X-Forwarded-For for client IP (handlers.New)

Notes:
- No external secrets store is required. If you deploy behind a reverse proxy, configure HTTPS there and ensure Upgrade headers are forwarded for WebSockets.
