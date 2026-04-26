# Architecture Review (April 26, 2026)

## Current architecture snapshot

QuizHub is a clean single-binary Go web app with:
- `cmd/server` bootstrapping DB, middleware, HTTP routes, WebSocket hub, and embedded static assets.
- A layered backend under `internal/` (`db`, `handlers`, `middleware`, `models`, `ws`).
- Vanilla frontend assets embedded via `go:embed` from `web/static`.

This is a practical architecture for small-to-medium deployments: easy local setup, low ops overhead, and minimal moving parts.

## Strengths

1. **Simple deployability**: single binary + embedded static assets + SQLite makes environments easy to reproduce.
2. **Good baseline separation**: transport (`handlers`), persistence (`db`), and realtime fanout (`ws`) are split.
3. **Operational guardrails**: graceful shutdown, middleware chain, read/write/idle timeouts, panic recovery.
4. **Realtime flow is explicit**: game lifecycle events are easy to trace through WebSocket events.

## Improvement opportunities

### 1) Concurrency safety in handler state (High)

`Handler` keeps mutable shared fields (`QuestionIDs`, `TimeLimit`, `AdminTokens`, timer pointer). Requests and timer callbacks can access these concurrently.

**Why improve**
- Race conditions are possible under concurrent admin actions or reconnect spikes.
- Future features (multi-room, richer admin actions) will increase contention.

**Recommendation**
- Add explicit synchronization for all mutable handler state.
- Better: move game/session state to a dedicated game service object with mutex-protected state + clear API.
- Run `go test -race ./...` in CI.

### 2) Authentication/token model hardening (High)

Admin tokens are in-memory map entries without expiration, rotation, or persistence boundaries.

**Why improve**
- Memory-only, non-expiring tokens are okay for prototypes but weak for internet-facing deployment.

**Recommendation**
- Add expiring signed tokens (e.g., JWT or HMAC session tokens) with TTL.
- Add logout/revocation semantics.
- Add optional allowlist for admin origin and rate-limited auth endpoint.

### 3) WebSocket origin policy and access controls (High)

WebSocket upgrader currently allows any origin.

**Why improve**
- Permissive origin checks increase abuse and CSRF-like cross-origin websocket risk.

**Recommendation**
- Restrict origins via config (`QUIZHUB_ALLOWED_ORIGINS`) and verify during upgrade.
- Validate role + player/admin identity server-side before accepting long-lived sockets.

### 4) Single-room coupling limits scalability (Medium)

`game_state` has a singleton row (`id=1`) and most handlers assume one global active room.

**Why improve**
- Prevents concurrent quizzes and complicates horizontal scaling.

**Recommendation**
- Introduce room-scoped tables (`rooms`, `room_players`, `room_questions`, `room_state`).
- Route every command/query by `room_code`.
- Scope WS channels by room.

### 5) DB reset semantics are destructive (Medium)

`ResetGame` deletes players, answers, and questions.

**Why improve**
- Admin-created question banks disappear after each reset.
- No audit/history for sessions.

**Recommendation**
- Split "reset round" vs "wipe content" into separate admin actions.
- Keep a reusable question bank; store round/session results separately.

### 6) Test suite drift and dependency hygiene (Medium)

Current tests and module metadata appear out-of-sync with runtime codepaths/environment.

**Why improve**
- Reliability and release confidence are lower when tests are stale or failing by default.

**Recommendation**
- Reconcile handler tests with current room-based join flow.
- Run `go mod tidy` to normalize `go.sum` entries.
- Add CI checks for `go test ./...`, `go test -race ./...`, and lint.

### 7) Frontend source-of-truth duplication (Low/Medium)

`frontend/public/*` and `web/static/*` currently mirror each other.

**Why improve**
- Duplicate copies can drift and create ambiguous ownership.

**Recommendation**
- Pick one canonical source directory and generate/sync embed assets in build step.
- Document the pipeline in README/Makefile.

### 8) API contracts and versioning (Low/Medium)

Endpoints are pragmatic but largely implicit contracts.

**Recommendation**
- Add OpenAPI spec for REST endpoints and event schema docs for WS payloads.
- Move to `/api/v1` namespace before larger surface changes.

## Suggested implementation order

1. **Security baseline**: WS origin restrictions + admin token TTL + auth rate limiting.
2. **Correctness baseline**: synchronize handler mutable state; add race tests in CI.
3. **Developer productivity**: fix test drift + module tidy + CI pipeline.
4. **Scalability track**: room-scoped data model and WS room channels.
5. **Product quality**: persistent question bank, session history, API schema/versioning.

## Bottom line

The architecture is solid for MVP/small deployments, but moving from "works well" to "production robust" mostly requires hardening shared-state concurrency, auth/session security, and automated test/CI discipline.

---

## Recheck update (after latest pushed changes)

I re-ran the architecture check and verified core backend files again. The top recommendations from the first pass are still valid; none of the previously highlighted high-priority risks appears fully addressed yet.

### Quick status

| Area | Status on recheck | Notes |
|---|---|---|
| Handler shared mutable state synchronization | Not addressed | Mutable fields are still shared on `Handler` with only timer-specific locking. |
| Admin token hardening (TTL/expiry/revocation) | Not addressed | Token map remains in-memory boolean store without expiry lifecycle. |
| WebSocket origin validation | Not addressed | `CheckOrigin` still returns `true`. |
| Single-room architecture | Not addressed | Singleton `game_state` row model remains. |
| Reset semantics preserving question bank | Not addressed | Reset still deletes questions/players/answers in one path. |
| Module/test baseline health | Still failing in this environment | `go test ./...` still reports missing `go.sum` entries for core deps. |

### Suggested immediate next patch (high impact, low scope)

1. Add a mutex-protected admin/session state object inside handlers and run race tests.
2. Introduce expiring admin tokens with TTL + rotation.
3. Lock down WebSocket origins via env-configurable allowlist.
4. Split reset operations:
   - `reset_round` (players/answers/game state only)
   - `reset_all` (explicit destructive wipe)
5. Restore build reliability:
   - Run `go mod tidy` in a network-enabled environment and commit full `go.sum`.
   - Add CI to fail on `go test ./...` and `go test -race ./...`.
