# QuizHub

A real-time multiplayer trivia game shipped as a **single Go binary** with an embedded SQLite database, embedded frontend, WebSocket live sync, and a full admin panel.

Zero runtime dependencies. One binary. One command. Done.

---

## Table of Contents

- [Features](#features)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Prerequisites](#prerequisites)
- [Getting Started (Local)](#getting-started-local)
- [Deployment](#deployment)
  - [Option A: Docker (Recommended)](#option-a-docker-recommended)
  - [Option B: Bare Binary on a VPS](#option-b-bare-binary-on-a-vps)
  - [Option C: Docker Compose (with reverse proxy)](#option-c-docker-compose-with-reverse-proxy)
- [Configuration](#configuration)
- [Admin Panel](#admin-panel)
- [API Reference](#api-reference)
- [WebSocket Events](#websocket-events)
- [Running Tests](#running-tests)
- [Troubleshooting](#troubleshooting)
- [License](#license)

---

## Features

**Player Experience**
- Join with a nickname and play live trivia
- Timed questions with countdown bar (configurable 5-120 seconds)
- Speed-based scoring: faster correct answers earn more points (100-1000)
- Instant answer feedback (correct/wrong) with score reveal
- Live leaderboard with gold/silver/bronze rankings
- Real-time updates via WebSocket (no polling)

**Admin Panel** (`/admin.html`)
- PIN-protected access
- Start, advance, and reset games
- Live answer stats (correct/wrong counts in real time)
- Player management with kick functionality
- Full question CRUD (add, edit, delete)
- Timer configuration (5-120 seconds per question)
- 15 built-in questions across 6 categories (geography, science, math, history, literature, technology)
- WebSocket connection status indicator

**Architecture**
- Single binary distribution (14 MB) with everything embedded
- SQLite database created automatically at startup
- No external services needed (no Redis, no Postgres, no message queue)
- Graceful shutdown with signal handling
- Structured logging, CORS, security headers, panic recovery
- 38 Go unit tests with 82% DB and 72% handler coverage

---

## Tech Stack

| Component   | Technology                                                  |
|-------------|-------------------------------------------------------------|
| Language    | Go 1.24+                                                    |
| Database    | SQLite via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO) |
| WebSocket   | [`gorilla/websocket`](https://github.com/gorilla/websocket) |
| Frontend    | Vanilla HTML, CSS, JavaScript (embedded via `go:embed`)      |
| UUID        | [`google/uuid`](https://github.com/google/uuid)             |

---

## Project Structure

```
quizhub/
├── cmd/server/
│   └── main.go                 # Entry point: wires DB, WebSocket hub, routes, server
├── internal/
│   ├── db/
│   │   ├── db.go               # SQLite connection, migrations, seed data, all queries
│   │   └── db_test.go          # 14 database unit tests
│   ├── handlers/
│   │   ├── handlers.go         # All HTTP handlers (public, game, admin)
│   │   └── handlers_test.go    # 24 handler unit tests (incl. admin auth, kick, CRUD)
│   ├── middleware/
│   │   └── middleware.go       # CORS, logging, panic recovery, security headers
│   ├── models/
│   │   └── models.go           # All data structures (Player, Question, GameState, etc.)
│   └── ws/
│       └── hub.go              # WebSocket hub: connection manager, broadcast, events
├── web/
│   ├── embed.go                # go:embed directive for static files
│   └── static/
│       ├── index.html          # Player UI entry point
│       ├── app.css             # Player UI styles
│       ├── app.js              # Player UI logic + WebSocket client
│       ├── admin.html          # Admin panel entry point
│       ├── admin.css           # Admin panel styles
│       └── admin.js            # Admin panel logic + WebSocket client
├── Dockerfile                  # Multi-stage build (golang:1.24 -> alpine:3.21)
├── Makefile                    # build, run, test, fmt, lint, clean
├── go.mod
├── go.sum
├── LICENSE                     # MIT
└── README.md
```

---

## Prerequisites

- **Go 1.24+** (for building from source)
- **Docker** (for containerized deployment) — OR —
- Any Linux/macOS/Windows machine (for running the pre-built binary)

> SQLite is compiled into the binary. No database server needed.

---

## Getting Started (Local)

### 1. Clone the repository

```bash
git clone https://github.com/sakh1l/quizhub.git
cd quizhub
```

### 2. Build

```bash
make build
# or: go build -o quizhub ./cmd/server
```

This produces a single `quizhub` binary in the project root.

### 3. Run

```bash
./quizhub
```

Output:

```
QuizHub v1.0.0 running on http://localhost:8080
```

### 4. Open in browser

| URL                        | What it does         |
|----------------------------|----------------------|
| `http://localhost:8080`    | Player join screen   |
| `http://localhost:8080/admin.html` | Admin panel (PIN: `1234`) |

### 5. Play

1. Open the player URL in 2+ browser tabs
2. Enter nicknames and click **Join Game**
3. Open the admin panel in another tab, enter PIN `1234`
4. Click **Start Game** from the admin dashboard
5. Answer questions — scores update in real time

---

## Deployment

### Option A: Docker (Recommended)

The simplest way to deploy. One command builds and runs everything.

#### Step 1: Build the image

```bash
docker build -t quizhub .
```

This runs a multi-stage build:
- **Stage 1** (`golang:1.24-alpine`): Downloads dependencies, compiles the binary with `-ldflags="-s -w"` (stripped, smaller)
- **Stage 2** (`alpine:3.21`): Copies only the binary into a minimal 14 MB image

#### Step 2: Run the container

```bash
docker run -d \
  --name quizhub \
  -p 8080:8080 \
  -v quizhub-data:/app/data \
  -e QUIZHUB_ADMIN_PIN=your-secret-pin \
  quizhub
```

| Flag | Purpose |
|------|---------|
| `-d` | Run in background |
| `-p 8080:8080` | Map container port 8080 to host port 8080 |
| `-v quizhub-data:/app/data` | Persist SQLite database across container restarts |
| `-e QUIZHUB_ADMIN_PIN=...` | Set admin PIN (default: `1234`) |

#### Step 3: Verify

```bash
curl http://localhost:8080/api/health
# {"status":"ok","version":"1.0.0","player_count":0}
```

Open `http://your-server-ip:8080` in a browser.

#### Step 4: Stop / Remove

```bash
docker stop quizhub
docker rm quizhub
```

---

### Option B: Bare Binary on a VPS

No Docker needed. Download or build the binary, copy it to your server, and run.

#### Step 1: Build for your target OS/architecture

```bash
# Linux AMD64 (most cloud VPS)
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o quizhub ./cmd/server

# Linux ARM64 (AWS Graviton, Raspberry Pi 4)
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o quizhub ./cmd/server

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o quizhub ./cmd/server

# Windows
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o quizhub.exe ./cmd/server
```

#### Step 2: Copy to server

```bash
scp quizhub user@your-server:/opt/quizhub/
```

#### Step 3: Create a systemd service (Linux)

Create `/etc/systemd/system/quizhub.service`:

```ini
[Unit]
Description=QuizHub Trivia Game
After=network.target

[Service]
Type=simple
User=quizhub
WorkingDirectory=/opt/quizhub
ExecStart=/opt/quizhub/quizhub
Restart=on-failure
RestartSec=5

Environment=QUIZHUB_PORT=8080
Environment=QUIZHUB_DB=/opt/quizhub/data/quizhub.db
Environment=QUIZHUB_ADMIN_PIN=your-secret-pin

[Install]
WantedBy=multi-user.target
```

#### Step 4: Enable and start

```bash
sudo mkdir -p /opt/quizhub/data
sudo useradd -r -s /bin/false quizhub
sudo chown -R quizhub:quizhub /opt/quizhub

sudo systemctl daemon-reload
sudo systemctl enable quizhub
sudo systemctl start quizhub
sudo systemctl status quizhub
```

#### Step 5: (Optional) Add Nginx reverse proxy for HTTPS

```nginx
server {
    listen 80;
    server_name quiz.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;

        # WebSocket support
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Then use Certbot for HTTPS:

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d quiz.yourdomain.com
```

---

### Option C: Docker Compose (with reverse proxy)

For production setups with HTTPS via Caddy.

Create `docker-compose.yml`:

```yaml
version: "3.8"

services:
  quizhub:
    build: .
    container_name: quizhub
    restart: unless-stopped
    volumes:
      - quizhub-data:/app/data
    environment:
      - QUIZHUB_PORT=8080
      - QUIZHUB_ADMIN_PIN=change-this-pin
    expose:
      - "8080"

  caddy:
    image: caddy:2-alpine
    container_name: caddy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy-data:/data
      - caddy-config:/config

volumes:
  quizhub-data:
  caddy-data:
  caddy-config:
```

Create `Caddyfile`:

```
quiz.yourdomain.com {
    reverse_proxy quizhub:8080
}
```

Run:

```bash
docker compose up -d
```

Caddy automatically provisions HTTPS via Let's Encrypt.

---

## Configuration

All configuration is via environment variables. No config files needed.

| Variable | Default | Description |
|----------|---------|-------------|
| `QUIZHUB_PORT` | `8080` | Port the server listens on |
| `QUIZHUB_DB` | `quizhub.db` | Path to the SQLite database file |
| `QUIZHUB_ADMIN_PIN` | `1234` | PIN required to access the admin panel |

**Important**: Change `QUIZHUB_ADMIN_PIN` before deploying to production.

---

## Admin Panel

### Accessing the admin panel

1. Navigate to `/admin.html` on your QuizHub server
2. Enter the admin PIN (default: `1234`)
3. The dashboard loads with real-time WebSocket connection

### Admin capabilities

| Feature | Description |
|---------|-------------|
| **Start Game** | Shuffles all questions and begins the quiz |
| **Next Question** | Advances to the next question (or ends game) |
| **Reset Game** | Clears all players, scores, and returns to lobby |
| **Set Timer** | Change per-question time limit (5-120 seconds) |
| **Kick Player** | Remove a player from the game (disconnects their WebSocket) |
| **Add Question** | Create a new question with text, 4 options, correct answer, category |
| **Edit Question** | Modify any existing question |
| **Delete Question** | Remove a question from the bank |
| **Live Stats** | See real-time answer counts (total, correct, wrong) per question |
| **Leaderboard** | Live-updating player rankings |

---

## API Reference

All endpoints are prefixed with `/api`. JSON request/response bodies.

### Public Endpoints

| Method | Path | Description | Request Body | Response |
|--------|------|-------------|-------------|----------|
| `GET` | `/api/health` | Server health check | — | `{"status":"ok","version":"1.0.0","player_count":N}` |
| `POST` | `/api/join` | Join the quiz | `{"nickname":"..."}` | `{"player_id":"...","nickname":"...","score":0}` |
| `GET` | `/api/players` | List all players | — | `[{"player_id":"...","nickname":"...","score":N}]` |
| `GET` | `/api/game/state` | Current game state | — | `{"status":"lobby\|question\|finished",...}` |
| `POST` | `/api/answer` | Submit an answer | `{"player_id":"...","question_id":N,"answer":N}` | `{"correct":bool,"score_earned":N,"total_score":N}` |
| `GET` | `/api/leaderboard` | Sorted rankings | — | `[{"rank":N,"player_id":"...","nickname":"...","score":N}]` |
| `GET` | `/api/categories` | Question categories | — | `["geography","science",...]` |
| `GET` | `/api/questions` | All questions (with answers) | — | `[{"id":N,"text":"...","options":[...],"answer":N}]` |

### Game Control Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/game/start` | Start a new game (shuffles questions) |
| `POST` | `/api/game/next` | Advance to next question or finish game |
| `POST` | `/api/game/reset` | Reset to lobby (clears players and scores) |
| `POST` | `/api/game/start-with-categories` | Start with filtered categories: `{"categories":["math","science"]}` |

### Admin Endpoints (require `X-Admin-Token` header)

First authenticate to get a token:

```bash
curl -X POST /api/admin/auth \
  -H "Content-Type: application/json" \
  -d '{"pin":"1234"}'
# Returns: {"token":"abc123..."}
```

Then use the token in subsequent requests:

| Method | Path | Description | Request Body |
|--------|------|-------------|-------------|
| `POST` | `/api/admin/auth` | Authenticate with PIN | `{"pin":"..."}` |
| `GET` | `/api/admin/config` | Get timer + categories | — |
| `POST` | `/api/admin/timer` | Set timer (5-120 sec) | `{"time_limit":20}` |
| `POST` | `/api/admin/kick` | Kick a player | `{"player_id":"..."}` |
| `POST` | `/api/questions/add` | Add a question | `{"text":"...","options":["A","B","C","D"],"answer":1,"category":"math"}` |
| `POST` | `/api/questions/edit` | Edit a question | `{"id":1,"text":"...","options":[...],"answer":N,"category":"..."}` |
| `POST` | `/api/questions/delete` | Delete a question | `{"id":1}` |

### WebSocket

Connect to `/api/ws` with query parameters:

```
ws://localhost:8080/api/ws?role=player&player_id=abc-123
ws://localhost:8080/api/ws?role=admin
```

---

## WebSocket Events

Events are JSON messages: `{"event":"event_name","data":{...}}`

| Event | Direction | Description | Sent To |
|-------|-----------|-------------|---------|
| `player_joined` | Server -> Client | New player joined | All |
| `players_update` | Server -> Client | Full player list refresh | All |
| `game_started` | Server -> Client | Game started with first question | All |
| `new_question` | Server -> Client | New question loaded | All |
| `player_answered` | Server -> Client | Answer stats updated | Admin only |
| `leaderboard_update` | Server -> Client | Leaderboard changed | All |
| `game_finished` | Server -> Client | All questions answered | All |
| `game_reset` | Server -> Client | Game returned to lobby | All |
| `player_kicked` | Server -> Client | Player was removed | Kicked player only |

---

## Running Tests

### Go unit tests (38 tests)

```bash
make test
# or: go test ./... -v -count=1
```

### Test coverage report

```bash
make cover
# Opens HTML coverage report in browser
```

### Quick smoke test (curl)

```bash
# Health
curl -s http://localhost:8080/api/health

# Join
curl -s -X POST http://localhost:8080/api/join \
  -H "Content-Type: application/json" \
  -d '{"nickname":"Test"}'

# Admin auth
curl -s -X POST http://localhost:8080/api/admin/auth \
  -H "Content-Type: application/json" \
  -d '{"pin":"1234"}'
```

---

## Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| `bind: address already in use` | Port 8080 is taken | Set `QUIZHUB_PORT=3000` or kill the other process |
| `Failed to open database` | Directory doesn't exist | Create the data directory: `mkdir -p /app/data` |
| WebSocket not connecting | Reverse proxy not forwarding `Upgrade` headers | Add `proxy_set_header Upgrade $http_upgrade` to Nginx config |
| Admin returns 401 | Wrong PIN or missing token | Verify PIN matches `QUIZHUB_ADMIN_PIN` env; include `X-Admin-Token` header |
| Questions not loading | Database was deleted | Restart the server — seed data is recreated automatically |
| Binary won't run on server | Wrong architecture | Rebuild with correct `GOOS`/`GOARCH` (see [Option B](#option-b-bare-binary-on-a-vps)) |

---

## License

MIT License. See [LICENSE](LICENSE) for full text.

---

Built with Go, SQLite, and zero JavaScript frameworks.
