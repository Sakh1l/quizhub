# QuizHub - PRD

## Original Problem Statement
1. Read the codebase, check UI testing, create TODO list
2. Maintain Go stack, single binary with SQLite
3. Deployment readiness (Option 2: keep SQLite, quick fixes)
4. WebSocket real-time sync, Admin/Host panel with PIN, Dockerfile

## Architecture
- **Go binary** (14MB): API + embedded frontend + SQLite
- **WebSocket hub**: Real-time events for players AND admin
- **Admin panel**: PIN-protected (/admin.html) with full game/question management
- **Deployment**: FastAPI proxy for Emergent K8s; standalone Docker for self-hosted

### API Endpoints (18 total)
- Public: health, join, players, game/state, answer, leaderboard, categories, questions
- Game control: game/start, game/next, game/reset, game/start-with-categories
- Admin (PIN-protected): admin/auth, admin/kick, admin/timer, admin/config, questions/add, questions/edit, questions/delete
- WebSocket: /api/ws (role=player|admin)

## What's Been Implemented
- [Jan 2026] Phase 1: Codebase audit, TODO.md
- [Jan 2026] Phase 2: Full refactor to single binary, 35 Go unit tests
- [Jan 2026] Phase 3: Deployment readiness (FastAPI proxy, port config)
- [Jan 2026] Phase 4: WebSocket + Admin + Dockerfile
  - WebSocket hub with gorilla/websocket, broadcasts: player_joined, game_started, new_question, player_answered, timer_tick, game_finished, game_reset, player_kicked, leaderboard_update, players_update
  - Admin panel: PIN auth ("1234" default), game controls, timer config, player kick, question CRUD, live answer stats, live leaderboard
  - Dockerfile: multi-stage (golang:1.24-alpine -> alpine:3.21), single binary, /app/data volume
  - 38 Go unit tests (14 DB + 24 handler) all passing
  - E2E tests: 100% pass (18 backend + 14 frontend + WS)

## Admin Credentials
- Default PIN: `1234` (configurable via `QUIZHUB_ADMIN_PIN` env)
- Admin URL: `/admin.html`

## Deployment Notes
- SQLite data is ephemeral on Emergent (resets on pod restart)
- For Docker self-hosted: `docker build -t quizhub . && docker run -p 8080:8080 -v ./data:/app/data quizhub`

## Backlog
- P1: MongoDB migration, room system
- P2: Sound effects, dark/light toggle, avatars, confetti, streak bonuses

## Documentation
- [Jan 2026] Comprehensive README.md (544 lines) covering: features, tech stack, project structure, local setup, 3 deployment options (Docker, bare binary/VPS/systemd, Docker Compose + Caddy HTTPS), configuration, admin panel guide, full API reference (18 endpoints), WebSocket events, testing, troubleshooting
