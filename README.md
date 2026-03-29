# QuizHub

A real-time multiplayer trivia game built as a **single Go binary** with embedded SQLite and static frontend.

## Features

- Join with a nickname and play live trivia
- 15 built-in questions across geography, science, math, history, literature, and technology
- Speed-based scoring: faster correct answers earn more points
- Live leaderboard with rankings
- Add custom questions via API
- Zero external dependencies at runtime: single binary, embedded database, embedded frontend
- Graceful shutdown, structured logging, CORS, security headers

## Quick Start

```bash
# Build
go build -o quizhub ./cmd/server

# Run (default port 8080)
./quizhub

# Custom port
PORT=3000 ./quizhub

# Custom database path
QUIZHUB_DB=/data/quiz.db ./quizhub
```

Open `http://localhost:8080` in your browser.

## Project Structure

```
.
├── cmd/server/          # Application entry point
│   └── main.go
├── internal/
│   ├── db/              # SQLite database layer
│   │   ├── db.go
│   │   └── db_test.go
│   ├── handlers/        # HTTP request handlers
│   │   ├── handlers.go
│   │   └── handlers_test.go
│   ├── middleware/       # HTTP middleware (CORS, logging, recovery)
│   │   └── middleware.go
│   └── models/          # Data structures
│       └── models.go
├── web/
│   ├── embed.go         # go:embed directive
│   └── static/          # Frontend (HTML, CSS, JS)
├── go.mod
├── Makefile
├── LICENSE
└── README.md
```

## API

All endpoints are prefixed with `/api`.

| Method | Path               | Description                    |
|--------|--------------------|--------------------------------|
| GET    | `/api/health`      | Server health & player count   |
| POST   | `/api/join`        | Join the quiz                  |
| GET    | `/api/players`     | List all players               |
| POST   | `/api/game/start`  | Start a new game               |
| POST   | `/api/game/next`   | Advance to next question       |
| GET    | `/api/game/state`  | Get current game state         |
| POST   | `/api/answer`      | Submit an answer               |
| GET    | `/api/leaderboard` | Get sorted leaderboard         |
| POST   | `/api/game/reset`  | Reset game to lobby            |
| GET    | `/api/questions`   | List all questions (admin)     |
| POST   | `/api/questions/add`| Add a custom question         |

## Tech Stack

- **Go** (standard library + `modernc.org/sqlite`)
- **SQLite** (embedded, pure Go, no CGO)
- **Vanilla JS/CSS** (no frameworks, embedded in binary)

## Development

```bash
# Run tests
make test

# Format code
make fmt

# Build and run
make run
```

## License

MIT
