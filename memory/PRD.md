# QuizHub - PRD

## Original Problem Statement
1. Read the codebase of this QuizHub app, check UI unit testing, and create a TODO list
2. Maintain Go tech stack, make it a single binary combining SQLite for DB
3. Run deployment health check and make deployment-ready (Option 2: keep SQLite, fix quick wins)

## Architecture
- **Language**: Go 1.24+
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Frontend**: Vanilla HTML/CSS/JS served via `npx serve` on port 3000
- **Backend**: FastAPI reverse proxy (port 8001) → Go binary (port 8002)
- **Output**: Single self-contained Go binary (14MB) with embedded static assets

### Deployment Architecture (Emergent)
```
Kubernetes Ingress
  ├── /api/*  → Backend (port 8001) → FastAPI proxy → Go binary (port 8002) → SQLite
  └── /*      → Frontend (port 3000) → npx serve → static HTML/CSS/JS
```

### Package Structure
```
cmd/server/main.go        → Entry point, server wiring, graceful shutdown
internal/db/db.go          → SQLite operations, migrations, seed data
internal/handlers/          → HTTP handlers with dependency injection
internal/middleware/        → CORS, logging, recovery, security headers
internal/models/            → Data structures
web/embed.go               → go:embed directive for static files
web/static/                 → HTML, CSS, JS frontend
backend/server.py          → FastAPI reverse proxy for Emergent deployment
backend/quizhub            → Compiled Go binary
frontend/public/           → Static files for frontend service
```

## What's Been Implemented
- [Jan 2026] **Phase 1**: Full codebase audit → `TODO.md` with 100 items
- [Jan 2026] **Phase 2**: Complete refactor to single binary architecture
  - SQLite DB with 15 seeded questions, full game flow
  - 5-package Go architecture, embedded frontend
  - 35 unit tests (82% DB, 72% handler coverage)
  - Modern dark UI with animations, timer, responsive design
- [Jan 2026] **Phase 3**: Deployment readiness
  - FastAPI reverse proxy for Emergent compatibility
  - Port configuration (8001 backend, 3000 frontend)
  - Frontend served via npx serve
  - All 17 tests passing (10 backend + 7 frontend)

## Deployment Notes
- SQLite data is **ephemeral** — resets on pod restart. Acceptable for short-lived quiz games.
- For persistent data, migrate to MongoDB (Option 1 deferred)

## Prioritized Backlog
- **P1**: MongoDB migration (if persistence needed), WebSockets, Admin view, Room system
- **P2**: Sound effects, dark/light toggle, avatars, confetti, Dockerfile, CI/CD

## Next Tasks
1. MongoDB migration for persistent data (if needed)
2. WebSocket for real-time sync
3. Admin/Host control panel
4. Dockerfile for self-hosted deployment
