# QuizHub - Comprehensive TODO List

> **Generated**: January 2026
> **App**: QuizHub - A real-time multiplayer quiz game
> **Stack**: Go 1.25.1 (backend) + Vanilla HTML/CSS/JS (frontend)
> **Current state**: Single-commit prototype with minimal functionality

---

## Table of Contents

- [A. Missing / Incomplete UI Unit Tests](#a-missing--incomplete-ui-unit-tests)
- [B. Code Quality & Clean Code Improvements](#b-code-quality--clean-code-improvements)
- [C. Comprehensive Feature / Bug / Improvement Backlog](#c-comprehensive-feature--bug--improvement-backlog)

---

## A. Missing / Incomplete UI Unit Tests

### A.1 - No test infrastructure exists

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| A.1.1  | P0       | **Zero test files** in the entire repo. No `_test.go` files, no JS test files, no test config. |
| A.1.2  | P0       | Add Go test framework setup (`go test`) for backend handler tests.                        |
| A.1.3  | P0       | Add a frontend testing framework (e.g., Playwright, Cypress, or JSDOM + Jest).            |
| A.1.4  | P1       | Add CI pipeline config (GitHub Actions) to run tests on every push/PR.                    |

### A.2 - Backend handler tests needed

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| A.2.1  | P0       | `healthHandler` - Test returns 200 with expected body.                                    |
| A.2.2  | P0       | `joinHandler` - Test valid join returns player JSON with ID, nickname, score=0.           |
| A.2.3  | P0       | `joinHandler` - Test empty nickname returns 400.                                          |
| A.2.4  | P0       | `joinHandler` - Test malformed JSON returns 400.                                          |
| A.2.5  | P0       | `playersHandler` - Test returns all joined players.                                       |
| A.2.6  | P0       | `startQuestionHandler` - Test returns question with expected structure.                   |
| A.2.7  | P0       | `answerHandler` - Test correct answer awards positive score.                              |
| A.2.8  | P0       | `answerHandler` - Test wrong answer awards 0 score.                                      |
| A.2.9  | P0       | `answerHandler` - Test answer with no active question returns 400.                        |
| A.2.10 | P0       | `answerHandler` - Test malformed body returns 400.                                        |
| A.2.11 | P0       | `leaderboardHandler` - Test returns players sorted by score descending.                   |
| A.2.12 | P1       | Concurrency tests - Multiple goroutines joining/answering simultaneously.                 |
| A.2.13 | P1       | Integration test - Full flow: join -> start question -> answer -> leaderboard.            |

### A.3 - Frontend / UI tests needed

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| A.3.1  | P0       | Test: Join form renders with input and button visible.                                    |
| A.3.2  | P0       | Test: Clicking "Join" with a nickname hides join div and shows "Waiting" div.             |
| A.3.3  | P0       | Test: Clicking "Join" with empty nickname shows validation error (currently missing).     |
| A.3.4  | P0       | Test: After join, `playerId` is set from server response.                                 |
| A.3.5  | P1       | Test: Network error during join is handled gracefully (currently no error handling).       |
| A.3.6  | P1       | Test: Question display and answer selection works (feature doesn't exist yet).            |
| A.3.7  | P1       | Test: Leaderboard display renders correctly (feature doesn't exist yet).                  |
| A.3.8  | P2       | Test: Accessibility - all interactive elements are keyboard-navigable.                    |
| A.3.9  | P2       | Test: Responsive layout works on mobile viewports.                                        |

---

## B. Code Quality & Clean Code Improvements

### B.1 - Project structure & organization

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| B.1.1  | P0       | **Add `.gitignore`** - Binary (`quizhub`), `join-quiz.png`, OS files, IDE configs are committed. |
| B.1.2  | P0       | **Remove committed binary** (`/app/quizhub`) from the repo. Binaries should never be tracked. |
| B.1.3  | P0       | **Remove duplicate QR code** - `join-quiz.png` exists in both `/app/` and `/app/cmd/server/`. |
| B.1.4  | P0       | **Add `README.md`** - No documentation exists. Needs: project description, setup instructions, API docs, contributing guide. |
| B.1.5  | P1       | **Add `LICENSE`** file - Required for any open-source project.                            |
| B.1.6  | P1       | **Add `CONTRIBUTING.md`** - Guidelines for contributors.                                  |
| B.1.7  | P1       | **Add `Makefile`** - Standardize build, test, run, lint commands.                         |
| B.1.8  | P2       | **Add `CHANGELOG.md`** - Track changes between versions.                                  |

### B.2 - Backend code quality (`main.go`)

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| B.2.1  | P0       | **Single-file monolith** - All 220 lines in one file. Split into packages: `handlers/`, `models/`, `middleware/`, `server/`. |
| B.2.2  | P0       | **Global mutable state** - `players`, `currentQuestion`, `questionStartTime` are package-level vars. Encapsulate in a `GameState` struct with methods. |
| B.2.3  | P0       | **No HTTP method validation** - All handlers accept GET, POST, DELETE, etc. Use `r.Method` checks or a router like `chi`/`gorilla/mux`. |
| B.2.4  | P0       | **No CORS headers** - Frontend from a different origin will be blocked. Add CORS middleware. |
| B.2.5  | P0       | **Hardcoded QR URL** (`http://localhost:8080`) in `generateQRCode()` - Should use env var or derive from actual listen address. |
| B.2.6  | P1       | **No request logging** - Add structured logging middleware (e.g., `log/slog` or `zerolog`). |
| B.2.7  | P1       | **No graceful shutdown** - Server doesn't handle SIGTERM/SIGINT. Use `http.Server.Shutdown()` with context. |
| B.2.8  | P1       | **No input sanitization** - Nickname is used as-is (potential XSS if rendered).           |
| B.2.9  | P1       | **No response helper functions** - JSON encoding and header setting is repeated in every handler. Extract to `writeJSON(w, status, data)`. |
| B.2.10 | P1       | **Error responses are plain text** - Use consistent JSON error format: `{"error": "message"}`. |
| B.2.11 | P1       | **Score calculation can overflow** - `10000 - responseTime` is an `int` subtraction where `responseTime` could be very large. Already clamped to 0, but logic should be clearer. |
| B.2.12 | P2       | **No API versioning** - Routes like `/join` should be `/api/v1/join` for future compatibility. |
| B.2.13 | P2       | **No rate limiting** - Endpoint spam is possible.                                         |
| B.2.14 | P2       | **No health check detail** - `/health` just returns a string. Should return JSON with version, uptime, player count. |

### B.3 - Frontend code quality (`index.html`)

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| B.3.1  | P0       | **Inline event handlers** - `onclick="joinQuiz()"` is anti-pattern. Use `addEventListener`. |
| B.3.2  | P0       | **No error handling** - `fetch` call has no `.catch()` or error status check.             |
| B.3.3  | P0       | **No input validation** - Empty nickname is sent to server without client-side check.     |
| B.3.4  | P0       | **Global variables** - `playerId` is in global scope. Wrap in an IIFE or module.          |
| B.3.5  | P0       | **Inline CSS** - Styles in `<style>` tag. Extract to separate `.css` file(s).             |
| B.3.6  | P0       | **Inline JS** - Script in `<script>` tag. Extract to separate `.js` file(s).             |
| B.3.7  | P1       | **No `data-testid` attributes** - Interactive elements lack test hooks.                   |
| B.3.8  | P1       | **No loading state** - No feedback while join request is in-flight.                       |
| B.3.9  | P1       | **No `<label>` for input** - Accessibility violation: input has no associated label.      |
| B.3.10 | P1       | **No `<meta charset>`** - Missing charset declaration.                                    |
| B.3.11 | P1       | **`<br><br>` for spacing** - Use CSS margin/padding instead of break tags.                |
| B.3.12 | P2       | **No semantic HTML** - Use `<main>`, `<section>`, `<form>` instead of raw `<div>`.        |
| B.3.13 | P2       | **No favicon** - Browser shows default icon.                                              |
| B.3.14 | P2       | **No `<noscript>` fallback** - Users with JS disabled see nothing.                        |

### B.4 - Code formatting & linting

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| B.4.1  | P0       | **Mixed indentation** in `main.go` - Line 197 uses spaces while rest uses tabs. Run `gofmt`. |
| B.4.2  | P1       | **Add `golangci-lint`** config (`.golangci.yml`) for comprehensive linting.               |
| B.4.3  | P1       | **Add `prettier`** or `html-validate` for frontend file formatting.                       |
| B.4.4  | P2       | **Add `.editorconfig`** for consistent editor settings across contributors.               |

---

## C. Comprehensive Feature / Bug / Improvement Backlog

### C.1 - Bugs & Critical Issues

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| C.1.1  | P0       | **Only 1 hardcoded question** - `startQuestionHandler` always returns "What is 2+2?". App is non-functional as a quiz. |
| C.1.2  | P0       | **No question display in UI** - Frontend only has join screen. Cannot see questions, answer them, or view scores. |
| C.1.3  | P0       | **No leaderboard in UI** - `/leaderboard` endpoint exists but UI doesn't consume it.     |
| C.1.4  | P0       | **QR code path mismatch** - `generateQRCode()` writes to `join-quiz.png` relative to CWD, but server serves from `./web/static`. QR code is not accessible via web. |
| C.1.5  | P1       | **Player ID not validated** in `answerHandler` - Non-existent `player_id` silently creates a zero-value player entry. |
| C.1.6  | P1       | **Race condition** - `currentQuestion` and `questionStartTime` are read outside mutex lock in `answerHandler`. |
| C.1.7  | P1       | **No duplicate nickname check** - Multiple players can join with the same name.           |
| C.1.8  | P2       | **No player session/auth** - Anyone who knows a player ID can submit answers.             |

### C.2 - Missing Core Features

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| C.2.1  | P0       | **Question bank** - Add a pool of questions (JSON file, database, or API). Support categories and difficulty. |
| C.2.2  | P0       | **Question UI** - Display current question with answer options as clickable buttons.      |
| C.2.3  | P0       | **Answer feedback UI** - Show correct/incorrect after answering with score earned.        |
| C.2.4  | P0       | **Leaderboard UI** - Show live leaderboard with player rankings.                          |
| C.2.5  | P0       | **Game flow** - Implement multi-question rounds: question -> answer -> next question -> final scores. |
| C.2.6  | P1       | **Timer** - Countdown timer per question visible to players.                              |
| C.2.7  | P1       | **Admin/Host view** - Separate interface for quiz host to start games and advance questions. |
| C.2.8  | P1       | **Real-time updates** - Use WebSockets or SSE instead of polling for live game sync.      |
| C.2.9  | P2       | **Room system** - Support multiple concurrent quiz rooms with unique codes.               |
| C.2.10 | P2       | **Persistent storage** - Replace in-memory maps with a database (SQLite, PostgreSQL, or MongoDB). |

### C.3 - UX / UI Improvements

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| C.3.1  | P0       | **Complete redesign** - Current UI is a bare-bones HTML form. Needs a modern, visually engaging design. |
| C.3.2  | P0       | **Responsive design** - Current layout breaks on small screens. Implement mobile-first CSS. |
| C.3.3  | P1       | **Loading spinner/skeleton** - Show loading states during API calls.                      |
| C.3.4  | P1       | **Toast notifications** - Success/error messages for join, answer, etc.                   |
| C.3.5  | P1       | **Animations** - Entrance animations, question transitions, score reveals.                |
| C.3.6  | P1       | **Sound effects** - Audio feedback for correct/wrong answers, timer warning.              |
| C.3.7  | P1       | **Dark mode** - Support system preference and manual toggle.                              |
| C.3.8  | P2       | **Theme/branding** - Custom quiz themes, host branding.                                   |
| C.3.9  | P2       | **Confetti/celebrations** - Visual reward for correct answers or winning.                 |
| C.3.10 | P2       | **Avatars** - Player avatars or profile pictures.                                         |

### C.4 - Performance & Infrastructure

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| C.4.1  | P1       | **Dockerfile** - No containerization. Add multi-stage Docker build.                       |
| C.4.2  | P1       | **Environment config** - Use `.env` file or config package instead of only `PORT` from env. |
| C.4.3  | P1       | **Structured logging** - Replace `fmt.Println` with `slog` or `zerolog`.                 |
| C.4.4  | P2       | **Metrics** - Add Prometheus metrics endpoint for monitoring.                             |
| C.4.5  | P2       | **Request timeouts** - Add read/write timeouts to `http.Server`.                          |
| C.4.6  | P2       | **Connection limits** - Protect against connection exhaustion.                            |

### C.5 - Security

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| C.5.1  | P0       | **No input sanitization** - Nicknames are stored and potentially rendered without escaping. XSS risk. |
| C.5.2  | P1       | **No rate limiting** - Join and answer endpoints can be spammed.                          |
| C.5.3  | P1       | **No authentication** - Any client can hit admin endpoints like `/start-question`.        |
| C.5.4  | P2       | **No HTTPS enforcement** - App serves over plain HTTP.                                    |
| C.5.5  | P2       | **Security headers** - Missing `X-Content-Type-Options`, `X-Frame-Options`, `CSP`, etc.  |

### C.6 - Open Source Best Practices

| ID     | Priority | Item                                                                                      |
|--------|----------|-------------------------------------------------------------------------------------------|
| C.6.1  | P0       | **README.md** - Project overview, screenshots/GIF, features, quick start, API docs, tech stack, architecture diagram. |
| C.6.2  | P0       | **LICENSE** - Choose and add a license (MIT, Apache 2.0, etc.).                           |
| C.6.3  | P1       | **CONTRIBUTING.md** - Setup guide, coding standards, PR process, issue templates.         |
| C.6.4  | P1       | **Issue templates** - Bug report and feature request templates (`.github/ISSUE_TEMPLATE/`). |
| C.6.5  | P1       | **PR template** - `.github/PULL_REQUEST_TEMPLATE.md` with checklist.                     |
| C.6.6  | P1       | **Code of Conduct** - `CODE_OF_CONDUCT.md` (adopt Contributor Covenant).                 |
| C.6.7  | P1       | **GitHub Actions CI** - Lint, test, build on every PR.                                    |
| C.6.8  | P2       | **Semantic versioning** - Tag releases with `vX.Y.Z`.                                    |
| C.6.9  | P2       | **Badges** - Build status, coverage, Go version, license badge in README.                 |
| C.6.10 | P2       | **GoDoc comments** - Export all types/functions with proper doc comments.                  |

---

## Summary Statistics

| Category                  | P0  | P1  | P2  | Total |
|---------------------------|-----|-----|-----|-------|
| A. Unit Tests             | 16  | 7   | 2   | **25**|
| B. Code Quality           | 13  | 12  | 7   | **32**|
| C. Features/Bugs/Infra    | 14  | 16  | 13  | **43**|
| **Total**                 | **43** | **35** | **22** | **100** |

---

## Recommended Execution Order

### Phase 1 - Foundation (Week 1)
1. Add `.gitignore`, remove binary and duplicate files (B.1.1, B.1.2, B.1.3)
2. Run `gofmt` and fix indentation (B.4.1)
3. Add `README.md` and `LICENSE` (B.1.4, B.1.5, C.6.1, C.6.2)
4. Set up Go test framework and write handler tests (A.1.2, A.2.x)
5. Split `main.go` into packages (B.2.1)

### Phase 2 - Backend Hardening (Week 2)
6. Encapsulate game state in struct (B.2.2)
7. Add HTTP method validation (B.2.3)
8. Add CORS middleware (B.2.4)
9. Add request logging and graceful shutdown (B.2.6, B.2.7)
10. Build question bank and multi-question flow (C.2.1, C.2.5)
11. Fix bugs: player ID validation, race conditions (C.1.5, C.1.6)

### Phase 3 - Frontend Overhaul (Week 3)
12. Separate JS and CSS into own files (B.3.5, B.3.6)
13. Add question display, answer UI, leaderboard UI (C.2.2, C.2.3, C.2.4)
14. Add `data-testid` attributes to all elements (B.3.7)
15. Add loading states, error handling, input validation (B.3.2, B.3.3, B.3.8)
16. Modern responsive redesign (C.3.1, C.3.2)

### Phase 4 - Polish & OSS (Week 4)
17. Add frontend test suite (A.3.x)
18. CI pipeline with GitHub Actions (A.1.4, C.6.7)
19. Contributing guide, templates, code of conduct (C.6.3-C.6.6)
20. WebSocket support for real-time gameplay (C.2.8)
21. Timer, admin view, room system (C.2.6, C.2.7, C.2.9)

---

*This TODO list covers 100 items across testing, code quality, and features. Tackle P0 items first for a functional, well-tested quiz app.*
