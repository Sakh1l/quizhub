# QuizHub Roadmap: Toward a Wayground/Kahoot-style Platform

Context: KonfHub's QuizHub (the live-quiz tool this app was modeled after) is no longer functional.
The goal is to grow this project into its replacement, borrowing proven UX and product ideas from
Kahoot and Wayground (formerly Quizizz) — while keeping the thing that makes this codebase valuable:
**a single Go binary, embedded SQLite, zero external dependencies.**

This document is a gap analysis + phased milestone plan. It supersedes the "Backlog" section of
`memory/PRD.md` and the P1/P2 rows of `TODO.md` for anything roadmap-shaped; those files stay as the
historical build log and code-quality checklist respectively.

---

## 1. Current State (verified against code, 2026-08-04)

| Area | Reality today |
|---|---|
| Rooms | **Exactly one room can exist at a time**, enforced at the schema level (`game_state` has a hard-coded single row, `CHECK (id = 1)`). Not a policy choice — a structural limit. |
| Quiz content | Questions are created ad hoc per session and **deleted on every reset** (`ResetGame` runs `DELETE FROM questions`). There is no concept of a saved, reusable quiz — hosts retype questions every time. |
| Question types | One type only: 4-option single-select multiple choice. No true/false, polls, multi-select, media, or open text. |
| Auth | Admin = one shared PIN (`QUIZHUB_ADMIN_PIN`) issuing bearer tokens. No user accounts, no per-host identity, no multi-host support. |
| Game modes | Host-paced live only (admin clicks "Next Question"). No self-paced/async/homework mode. |
| Real-time | Solid — `internal/ws` hub, server-authoritative timers (`time.AfterFunc`), millisecond-precision speed scoring (0–1000/question). This is genuinely competitive with Kahoot's model already. |
| Engagement/UX | No sound, no avatars, no confetti/animation, no streak bonuses, no team mode, no dark/light toggle — grepped the frontend, none of it exists yet. |
| Reporting | Live leaderboard only. No post-game export, no per-question analytics, no historical results (everything is wiped on reset). |
| Deployment | Single Fly.io machine, 256MB, SQLite on a mounted volume. Fine for one concurrent quiz; will not survive multi-room without rethinking the storage/concurrency model. |
| Code health | Clean: race-tested (`go test -race`), CI on every push, `gofmt`/`go vet`/`go mod tidy` enforced, decent handler test coverage. This is a good foundation to build on, not a rewrite candidate. |

**The headline finding:** every other gap (question types, engagement, reporting) is additive. The
one gap that blocks everything else architecturally is **rooms/quizzes are not first-class,
persistent, multi-tenant entities.** That has to be milestone 1.

---

## 2. What to borrow from Kahoot / Wayground

| Product idea | Source | Why it matters here |
|---|---|---|
| Reusable quiz library (create once, host many times, organize into folders) | Both | Directly fixes "retype questions every session" |
| Game PIN + big-screen/projector view separate from player device | Kahoot | Matches the conference/event use case KonfHub QuizHub served |
| Self-paced "homework" mode (players move through questions on their own timeline) | Wayground | Async engagement outside a live event window |
| Question variety: true/false, checkbox (multi-select), poll (no right answer), open text | Both | Cheap to add once question types are modeled generically |
| Media per question (image/GIF/video) | Both | High perceived-quality win, moderate storage/embed cost |
| Streak bonuses, power-ups, sound effects, confetti, avatars | Both | The "fun" layer — cheap, high engagement ROI, no architecture risk |
| Team mode | Kahoot | Good for large in-person audiences (conferences, classrooms) |
| Post-game reports + CSV export, per-question breakdown | Both | Hosts (esp. corporate/education) expect this after the game ends |
| Randomized answer/question order per player | Both | Anti-cheating for in-person games |

Explicitly **not** borrowing (out of scope / against this project's stated constraints): AI quiz
generation, public quiz marketplace/discovery, mobile apps, moving off Go/SQLite. These can be
revisited later but aren't needed to replace what KonfHub QuizHub did.

---

## 3. Milestones

Each milestone is independently shippable and leaves `main` deployable. Ordered by dependency, not
strictly by size — M1 unlocks the most value per effort and everything else builds on it.

### M0 — Pre-work hardening (small, do first)
- Wire up `golangci-lint` in CI (currently only local `make lint`, optional install)
- Middleware unit tests (CORS, Logger, Recover, SecurityHeaders) — `TODO.md` A.1.5
- Decide + document data-retention policy before M1 makes data persistent (currently everything is
  designed to be wiped; once quizzes/history persist, need a real answer for GDPR-style deletion)

### M1 — Quiz Library (persistent, reusable quizzes)
**The unblocking milestone.** Turn questions from session-scoped scratch data into owned, saved entities.
- New `quizzes` table: `id, title, owner_id (nullable until M3), created_at`
- `questions` gets a `quiz_id` FK instead of being global
- Admin panel: create/save/duplicate/delete quizzes, browse a library instead of typing questions
  fresh each time
- `ResetGame` stops deleting `questions` — it clears only session/player/answer state
- Room creation picks *which* saved quiz to run, rather than "whatever's currently in the table"

### M2 — Multi-Room / Concurrent Sessions
**The hard architectural milestone.** Requires removing the `game_state` single-row constraint.
- Replace the singleton `game_state` row with a `rooms` table (`id, code, quiz_id, status,
  current_question_id, question_index, started_at, time_limit`)
- `Handler`'s in-memory state (`stagedQuestionIDs`, `timeLimit`, `activeTimer`) becomes
  per-room state — likely a `map[roomCode]*roomState` with its own mutex, or a small in-process
  room manager
- WebSocket hub needs room-scoped broadcast (`BroadcastToRoom(code, ...)` instead of global
  `Broadcast`) — currently every event goes to every connected client regardless of room
- Player/admin WS connections carry a room code, not just role
- This is the milestone most likely to introduce concurrency bugs — budget real time for
  `go test -race` coverage of the new room manager

### M3 — Host Accounts & Real Auth
- Replace the single shared admin PIN with actual host accounts (email/password to start;
  OAuth is a stretch goal, not a requirement)
- `quizzes.owner_id` becomes meaningful — hosts see only their own library
- Session tokens per host instead of one global token pool (`adminTokens` map today is already
  close to this shape — extend rather than replace)
- Keep a lightweight anonymous/PIN path for the person who just wants to run one quiz without
  signing up — don't force accounts on the core "join with a code" player flow

### M4 — Richer Question Types & Media
- Generalize `Question`/`QuestionOut` beyond single-select MCQ: true/false, multi-select,
  poll (no correct answer, just distribution), open text/fill-in-blank
- Scoring logic in `game.go` (`Answer` handler) needs a per-type strategy instead of the current
  hardcoded `selected == currentQ.Answer` check
- Image support per question: either embed as base64 in SQLite (simple, fits the "single binary,
  no external deps" philosophy) or accept external URLs — recommend starting with URLs to avoid DB
  bloat, revisit embedding only if self-hosted image upload becomes a real requirement

### M5 — Engagement & Presentation Layer
Lowest architectural risk, highest visible "this feels like Kahoot now" payoff:
- Sound effects (correct/wrong/tick/countdown), confetti on correct answers
- Streak bonus scoring (consecutive correct answers add a multiplier)
- Player avatars/color identity (no upload needed — procedural/icon-based like Kahoot's)
- Dark/light theme toggle
- Optional: separate "big screen" presentation view (`/present?room=CODE`) distinct from the
  player's own phone screen, for the projector/conference-room use case KonfHub served
- Team mode (players group under a team name, team score = aggregate)

### M6 — Self-Paced / Async Mode
- New room status alongside the live state machine: `self_paced`, where each player advances
  through questions independently instead of waiting on admin's "Next Question"
- Requires per-player question-index tracking rather than one global `current_question_id`
  (natural fallout of the M2 room-scoped state work)
- Scoring formula needs a variant that doesn't depend on a shared server-side countdown

### M7 — Reports & Analytics
- Post-game summary: per-question accuracy, average response time, score distribution
- CSV export of results (host-facing)
- Historical view of past sessions for a quiz (depends on M1 persistence + M3 ownership)

### M8 — Scale/Ops polish (only if usage demands it)
- Re-evaluate SQLite-on-a-single-Fly-machine once multi-room (M2) is live and concurrent write
  load is real — WAL mode already enabled, likely fine well past what a conference/classroom needs
- Only consider Postgres/multi-instance if you actually hit contention; don't do this speculatively

---

## 4. Suggested sequencing

```
M0 → M1 → M2 → (M3 and M4 in parallel) → M5 → M6 → M7 → (M8 if needed)
```

M1 and M2 are the load-bearing milestones — everything else (accounts, question types, engagement,
reports) is additive once quizzes are persistent, owned entities and rooms are no longer a global
singleton. M5 (engagement/presentation) is the cheapest milestone to get a highly visible "feels
like Kahoot" result and could be pulled forward if a demo/launch date is driving priority over
architectural completeness.

**Recommended starting point: M1 (Quiz Library).** It's schema-additive (no breaking rework of the
WebSocket/timer machinery), unblocks M3/M4/M7, and immediately fixes the most obviously broken
piece of the current UX — hosts losing their questions every time they reset a game.
