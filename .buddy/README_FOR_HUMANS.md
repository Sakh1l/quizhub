# Welcome! 👋

Hi — I'm Buddy. This is the friendly home page for this repo.
I write short notes so a new contributor can get started fast.

---

## What is this project?

QuizHub is a small real-time multiplayer trivia game. It runs as one Go binary.
The server embeds the frontend and uses an embedded SQLite DB. Players join by a room code. An admin creates quizzes via an admin page.

- What it does: run quiz games for players and a host.
- Who uses it: demos, small teams, learning projects.
- Why it exists: simple, self-contained live quiz app you can run anywhere.

## Big-picture ideas

- Single Go binary with frontend embedded (go:embed).
- Real-time sync via WebSockets (hub in internal/ws).
- Data stored in an embedded SQLite DB (no external DB).

## Quick actions (what to do first)

1. Open the detailed setup: .buddy/GETTING_STARTED.md
2. Build and run locally: `make build` then `./quizhub` (see README.md)
3. See the admin UI: http://localhost:8080/admin.html (PIN: 1234)
4. Read code entry: cmd/server/main.go → internal/handlers/handlers.go → internal/ws/hub.go

## Where to look in the code (start here)

- Entry point: cmd/server/main.go
- HTTP handlers: internal/handlers/handlers.go
- DB layer: internal/db/db.go
- WebSocket hub: internal/ws/hub.go
- Embedded frontend: web/static/*.html, web/static/*.js, web/embed.go
- Build & CI: Makefile, Dockerfile, .github/workflows/go.yml

## Next steps

- Run `make build` and `./quizhub` to see it live.
- Open `.buddy/GETTING_STARTED.md` for exact commands.
- If something in these notes looks wrong, run: `buddy scan` or ask "Update buddy".

---

If you want, I can now update the other Buddy files (GETTING_STARTED, ARCHITECTURE, MAPs, TECH_STACK) with details from the repo. Say "Yes — update the rest" to proceed.
