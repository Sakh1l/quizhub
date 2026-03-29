# QuizHub - PRD

## Original Problem Statement
1. Read the codebase of this QuizHub app, check UI unit testing, and create a TODO list
2. Maintain Go tech stack, make it a single binary combining SQLite for DB

## Architecture
- **Language**: Go 1.24+
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Frontend**: Vanilla HTML/CSS/JS embedded via `go:embed`
- **Output**: Single self-contained binary (14MB)

### Package Structure
```
cmd/server/main.go        → Entry point, server wiring, graceful shutdown
internal/db/db.go          → SQLite operations, migrations, seed data
internal/handlers/          → HTTP handlers with dependency injection
internal/middleware/        → CORS, logging, recovery, security headers
internal/models/            → Data structures
web/embed.go               → go:embed directive for static files
web/static/                 → HTML, CSS, JS frontend
```

## What's Been Implemented
- [Jan 2026] **Phase 1**: Full codebase audit → `TODO.md` with 100 items
- [Jan 2026] **Phase 2**: Complete refactor to single binary architecture
  - Replaced in-memory maps with SQLite (15 seeded questions)
  - Split monolithic `main.go` into 5 packages
  - Embedded frontend with `go:embed`
  - Full game flow: join → answer → leaderboard → play again
  - Modern dark UI with animations, timer, responsive design
  - 35 unit tests (14 DB + 21 handler), 82% DB coverage, 72% handler coverage
  - Added `.gitignore`, `Makefile`, `README.md`, `LICENSE`
  - Middleware: CORS, logging, recovery, security headers, graceful shutdown

## Core Requirements
- Single binary distribution (no external dependencies at runtime)
- SQLite embedded database
- Real-time multiplayer quiz game
- Speed-based scoring

## Prioritized Backlog
- **P1**: WebSockets for real-time sync, Admin/Host view, Room system, Docker, CI/CD
- **P2**: Sound effects, dark/light toggle, avatars, confetti, question categories, share results

## Next Tasks
1. Add middleware tests for full coverage
2. Add Playwright e2e tests
3. Implement WebSocket for real-time game sync
4. Build admin/host control panel
5. Dockerfile for containerized deployment
