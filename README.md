# QuizHub

A real-time multiplayer trivia game shipped as a **single Go binary** with an embedded SQLite database, embedded frontend, WebSocket live sync, room codes, and a full admin panel.

Zero runtime dependencies. One binary. One command. Done.

---

## Table of Contents

- [How It Works](#how-it-works)
- [Features](#features)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Prerequisites](#prerequisites)
- [Getting Started (Local)](#getting-started-local)
- [Deployment](#deployment)
  - [Option A: Docker (Recommended)](#option-a-docker-recommended)
  - [Option B: Bare Binary on a VPS](#option-b-bare-binary-on-a-vps)
  - [Option C: Docker Compose (with HTTPS)](#option-c-docker-compose-with-https)
- [Configuration](#configuration)
- [Game Flow](#game-flow)
  - [Admin Flow](#admin-flow)
  - [Player Flow](#player-flow)
  - [Scoring](#scoring)
- [API Reference](#api-reference)
- [WebSocket Events](#websocket-events)
- [Running Tests](#running-tests)
- [Troubleshooting](#troubleshooting)
- [License](#license)

---

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│  ADMIN (/admin.html)            PLAYERS (/)                 │
│                                                             │
│  1. Enter PIN ──────────┐                                   │
│  2. Add questions       │                                   │
│  3. Create Quiz Room ───┼──→ Room Code: A3X7K2              │
│     (get code + link)   │    Link: yoursite.com/?room=A3X7K2│
│                         │                                   │
│  4. Share code ─────────┼──→ 5. Enter code + name ──→ Join  │
│                         │                                   │
│  6. See players join    │    7. See lobby + other players    │
│  8. Click Start Game ───┼──→ 9. 10-sec countdown            │
│                         │                                   │
│  10. See question +     │    11. See question + options      │
│      timer + leaderboard│        Pick answer (locked in)    │
│      + answer stats     │                                   │
│                         │                                   │
│  12. Timer expires ─────┼──→ 13. Correct answer revealed    │
│      (auto-reveal)      │        Score shown                │
│                         │                                   │
│  14. Click Next Question┼──→ 15. Next question appears      │
│      ...repeat...       │        ...repeat...               │
│                         │                                   │
│  16. Game Over ─────────┼──→ 17. See personal rank (#1, #2) │
│      Full leaderboard   │                                   │
│  18. Create New Quiz    │                                   │
└─────────────────────────────────────────────────────────────┘
```

---

## Features

**Room System**
- Admin creates a quiz room with a unique 6-character code (e.g., `A3X7K2`)
- Shareable join link: `yoursite.com/?room=A3X7K2` — pre-fills the code for players
- One room at a time — simple, no conflicts

**Admin Panel** (`/admin.html`)
- PIN-protected access (default: `1234`)
- Add custom questions from scratch for each quiz (text, 4 options, correct answer)
- Set question timer (5–120 seconds)
- Create quiz room → get room code + shareable link
- See players join in real time
- Start game → 10-second "Get Ready" countdown
- During game: see current question, countdown timer, live leaderboard, answer stats (correct/wrong count)
- Advance to next question manually (admin controls pacing)
- Game over: see full final leaderboard
- Create new quiz (resets everything for a fresh start)

**Player Experience** (`/`)
- Enter room code + nickname to join
- No control buttons — admin runs the entire game
- See questions + answer options + countdown timer
- Pick an answer → "Answer locked!" confirmation → wait for timer
- Correct answer revealed to everyone when timer expires
- See personal result (correct/wrong + score earned)
- Game over: see personal rank and total score

**Scoring**
- Millisecond precision — faster correct answers earn more points
- Score range: 0–1000 per question
- Formula: `score = 1000 × (time_remaining / total_time)`
- Example: 15-second timer, answer in 0.35 seconds → score ≈ 977
- Wrong answers = 0 points
- Leaderboard ranks by total cumulative score

**Architecture**
- Single binary (14 MB) with everything embedded
- SQLite database created automatically at startup
- Real-time sync via WebSocket (gorilla/websocket)
- Server-side timers for countdown and question time limits
- No external services needed

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
│   │   ├── db.go               # SQLite: migrations, room code, players, questions, answers
│   │   └── db_test.go          # Database unit tests
│   ├── handlers/
│   │   ├── handlers.go         # HTTP handlers: room, join, game, admin, questions
│   │   └── handlers_test.go    # Handler unit tests
│   ├── middleware/
│   │   └── middleware.go       # CORS, logging, recovery, security headers, WebSocket support
│   ├── models/
│   │   └── models.go           # Data structures
│   └── ws/
│       └── hub.go              # WebSocket hub: connections, broadcast, per-player messaging
├── web/
│   ├── embed.go                # go:embed directive
│   └── static/
│       ├── index.html          # Player page (room code + name entry)
│       ├── app.css             # Player styles
│       ├── app.js              # Player logic + WebSocket client
│       ├── admin.html          # Admin panel
│       ├── admin.css           # Admin styles
│       └── admin.js            # Admin logic + WebSocket client
├── Dockerfile                  # Multi-stage build (golang:1.24 → alpine:3.21)
├── Makefile                    # build, run, test, fmt, lint, clean
├── go.mod / go.sum
├── LICENSE                     # MIT
└── README.md
```

---

## Prerequisites

- **Go 1.24+** (for building from source)
- **Docker** (for containerized deployment) — OR —
- Any Linux/macOS/Windows machine (for running the binary)

> SQLite is compiled into the binary. No database server needed.

---

## Getting Started (Local)

### 1. Clone and build

```bash
git clone https://github.com/sakh1l/quizhub.git
cd quizhub
make build    # or: go build -o quizhub ./cmd/server
```

### 2. Run

```bash
./quizhub
```

Output: `QuizHub v1.0.0 running on http://localhost:8080`

### 3. Open in browser

| URL                              | Who          |
|----------------------------------|--------------|
| `http://localhost:8080`          | Players      |
| `http://localhost:8080/admin.html` | Quiz host  |

### 4. Run a quiz

1. **Admin**: Open `/admin.html` → enter PIN `1234`
2. **Admin**: Add a few questions using the form
3. **Admin**: Click **Create Quiz Room** → copy the room code
4. **Players**: Open `/` → enter room code + nickname → click **Join Room**
5. **Admin**: Click **Start Game** when everyone's in
6. **Everyone**: Answer questions, watch scores, have fun!

---

## Deployment

### Option A: Docker (Recommended)

```bash
# Build
docker build -t quizhub .

# Run
docker run -d \
  --name quizhub \
  -p 8080:8080 \
  -v quizhub-data:/app/data \
  -e QUIZHUB_ADMIN_PIN=your-secret-pin \
  quizhub
```

Verify: `curl http://localhost:8080/api/health`

Open `http://your-server-ip:8080` in a browser.

### Option B: Bare Binary on a VPS

```bash
# Build for your target
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o quizhub ./cmd/server

# Copy to server
scp quizhub user@your-server:/opt/quizhub/

# Create systemd service
sudo tee /etc/systemd/system/quizhub.service << 'EOF'
[Unit]
Description=QuizHub Trivia Game
After=network.target

[Service]
Type=simple
User=quizhub
WorkingDirectory=/opt/quizhub
ExecStart=/opt/quizhub/quizhub
Restart=on-failure
Environment=QUIZHUB_PORT=8080
Environment=QUIZHUB_DB=/opt/quizhub/data/quizhub.db
Environment=QUIZHUB_ADMIN_PIN=your-secret-pin

[Install]
WantedBy=multi-user.target
EOF

# Start
sudo systemctl daemon-reload
sudo systemctl enable --now quizhub
```

**Add Nginx + HTTPS** (important for WebSocket):

```nginx
server {
    listen 80;
    server_name quiz.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Then: `sudo certbot --nginx -d quiz.yourdomain.com`

### Option C: Docker Compose (with HTTPS)

```yaml
# docker-compose.yml
version: "3.8"
services:
  quizhub:
    build: .
    restart: unless-stopped
    volumes:
      - quizhub-data:/app/data
    environment:
      - QUIZHUB_PORT=8080
      - QUIZHUB_ADMIN_PIN=change-this
    expose:
      - "8080"

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy-data:/data

volumes:
  quizhub-data:
  caddy-data:
```

```
# Caddyfile
quiz.yourdomain.com {
    reverse_proxy quizhub:8080
}
```

```bash
docker compose up -d
```

---

## Configuration

| Variable             | Default      | Description                              |
|----------------------|--------------|------------------------------------------|
| `QUIZHUB_PORT`       | `8080`       | Port the server listens on               |
| `QUIZHUB_DB`         | `quizhub.db` | Path to the SQLite database file         |
| `QUIZHUB_ADMIN_PIN`  | `1234`       | PIN to access the admin panel            |

**Important**: Change `QUIZHUB_ADMIN_PIN` before deploying to production.

---

## Game Flow

### Admin Flow

```
1. Go to /admin.html
2. Enter admin PIN (default: 1234)
3. Add questions:
   - Type question text
   - Fill in 4 options (A, B, C, D)
   - Select the correct answer
   - Click "Add Question"
   - Repeat for all questions
4. Set question timer (default: 15 seconds)
5. Click "Create Quiz Room"
   → Room code appears (e.g., A3X7K2)
   → Shareable link appears (e.g., yoursite.com/?room=A3X7K2)
   → Copy and share with players
6. Watch players join in real time
7. Click "Start Game" when everyone's in
   → 10-second "Get Ready" countdown begins
8. During each question:
   - See the question + countdown timer
   - See live answer stats (how many answered, correct vs wrong)
   - See live leaderboard (updates as answers come in)
9. When timer expires:
   - Correct answer is revealed automatically
   - Click "Next Question" to advance
10. After last question: see final leaderboard
11. Click "Create New Quiz" to start fresh
```

### Player Flow

```
1. Go to / (or use shareable link with ?room=CODE)
2. Enter room code + nickname → click "Join Room"
3. See lobby: "You're In!" + list of other players
4. Wait for host to start
5. 10-second countdown: "Get Ready!"
6. Question appears with options + timer
7. Pick an answer → "Answer locked! Waiting for timer..."
8. Timer expires → correct answer highlighted
   - See if you were right/wrong + score earned
   - Wait for host to advance
9. Repeat for all questions
10. Game Over: see your rank (#1, #2, etc.) + total score
```

### Scoring

| Scenario | Score |
|----------|-------|
| Correct answer in 0.3s (on 15s timer) | ~980 pts |
| Correct answer in 5s (on 15s timer) | ~667 pts |
| Correct answer in 14s (on 15s timer) | ~67 pts |
| Correct answer after timer (edge case) | 10 pts |
| Wrong answer | 0 pts |
| No answer (timeout) | 0 pts |

Formula: `score = 1000 × (time_remaining_ms / total_time_ms)` (minimum 10 for correct).

---

## API Reference

All endpoints are prefixed with `/api`. JSON request/response bodies.

### Public Endpoints

| Method | Path | Description | Body / Params |
|--------|------|-------------|---------------|
| `GET` | `/api/health` | Health check | — |
| `POST` | `/api/join` | Join a room | `{"nickname":"...","room_code":"A3X7K2"}` |
| `GET` | `/api/players` | List players in room | — |
| `GET` | `/api/game/state` | Current game state | — |
| `POST` | `/api/answer` | Submit answer | `{"player_id":"...","question_id":N,"answer":N}` |
| `GET` | `/api/leaderboard` | Sorted rankings | — |
| `GET` | `/api/room/info?code=X` | Check if room exists | Query: `code` |
| `GET` | `/api/questions` | List all questions | — |

### Admin Endpoints (require `X-Admin-Token` header)

Authenticate first:

```bash
curl -X POST /api/admin/auth \
  -H "Content-Type: application/json" \
  -d '{"pin":"1234"}'
# → {"token":"abc123..."}
```

| Method | Path | Description | Body |
|--------|------|-------------|------|
| `POST` | `/api/admin/auth` | Get admin token | `{"pin":"1234"}` |
| `POST` | `/api/questions/add` | Add a question | `{"text":"...","options":["A","B","C","D"],"answer":1,"category":"math"}` |
| `POST` | `/api/questions/delete` | Delete a question | `{"id":1}` |
| `POST` | `/api/admin/timer` | Set timer (5-120s) | `{"time_limit":20}` |
| `POST` | `/api/room/create` | Create quiz room | — → `{"room_code":"A3X7K2","link":"..."}` |
| `POST` | `/api/game/start` | Start game (10s countdown) | — |
| `POST` | `/api/game/next` | Next question (reveal state only) | — |
| `POST` | `/api/game/reset` | Reset everything | — |

### WebSocket

```
ws://localhost:8080/api/ws?role=player&player_id=abc-123
ws://localhost:8080/api/ws?role=admin
```

---

## WebSocket Events

Events are JSON: `{"event":"name","data":{...}}`

| Event | Description | Sent To |
|-------|-------------|---------|
| `game_countdown` | 10s countdown started | All |
| `new_question` | New question loaded | All |
| `time_up` | Timer expired, correct answer revealed | All |
| `your_result` | Personal correct/wrong + score | Individual player |
| `game_finished` | All questions done | All |
| `game_reset` | Game reset to fresh state | All |
| `player_joined` | New player joined room | All |
| `players_update` | Full player list refresh | All |
| `player_answered` | Answer stats update | Admin only |
| `player_kicked` | Player removed | Kicked player |
| `leaderboard_update` | Rankings changed | All |

---

## Running Tests

```bash
# Go unit tests
make test

# Coverage report
make cover

# Quick smoke test
curl http://localhost:8080/api/health
```

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `bind: address already in use` | Change port: `QUIZHUB_PORT=3000 ./quizhub` |
| WebSocket not connecting | Ensure Nginx forwards `Upgrade` headers (see deployment section) |
| "Invalid room code" | Room was reset — admin needs to create a new room |
| "Game already in progress" | Players can't join mid-game — wait for reset |
| Admin returns 401 | Include `X-Admin-Token` header from `/api/admin/auth` |
| Binary won't run | Rebuild with correct `GOOS`/`GOARCH` for your server |

---

## License

MIT License. See [LICENSE](LICENSE) for full text.

---

Built with Go, SQLite, and zero JavaScript frameworks.
