🏠 [Home](./README_FOR_HUMANS.md) · [Getting Started](./GETTING_STARTED.md) · [Architecture](./ARCHITECTURE.md) · [Tech](./TECH_STACK.md) · [Integrations](./INTEGRATIONS.md) · [Repo Map](./MAP/repo_map.md) · [Links](./LINKS.md)

---

# Architecture

This is a small single-binary web app that embeds its frontend and uses an embedded SQLite DB. It runs an HTTP server and a WebSocket hub to keep players in sync in real time.

## Major components

| Component | Where in code | Responsibility |
|---|---|---|
| Server entry | cmd/server/main.go | Wire up DB, WebSocket hub, handlers, middleware, and serve embedded static files. |
| Database layer | internal/db/db.go | SQLite migrations, queries, players/questions/answers, game state. |
| HTTP handlers | internal/handlers/handlers.go | API endpoints, admin auth, game lifecycle, timers. |
| WebSocket hub | internal/ws/hub.go | Manage connections, broadcast events to players/admin. |
| Embedded frontend | web/embed.go + web/static/* | Player and admin UI served from the binary (go:embed). |

## Request / data flow (common scenarios)

### Player joining
1. Player opens `/` → static page served.
2. Frontend posts to `POST /api/join` with room_code and nickname.
3. handlers.Join validates room, creates player via DB.CreatePlayer, broadcasts `player_joined` via hub.
4. Frontend opens WebSocket `/api/ws?role=player&player_id=...` to receive live events.

### Admin creating a room
1. Admin hits `/admin.html` and authenticates with PIN via `POST /api/admin/auth`.
2. Admin requests `POST /api/room/create` → server generates a room code and sets game_state in DB.
3. Hub broadcasts `room_created` to clients.

### Game start and question flow
1. Admin selects questions and calls `POST /api/game/start`.
2. DB.StartGame sets game state and handlers.startQuestion broadcasts `new_question`.
3. Players submit answers to `POST /api/answer`; DB records answers and updates scores.
4. After time limit, handlers.auto-reveal triggers `time_up` and admin advances with `POST /api/game/next`.

## Key boundaries

- No external DB or services required—the app embeds SQLite.
- WebSocket is the live sync channel; REST API is used for state changes and queries.
- Admin actions require `X-Admin-Token` header or admin_token query param for WS.

Where to dig deeper: start at cmd/server/main.go, then handlers, then db and ws packages.
