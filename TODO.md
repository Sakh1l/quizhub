# QuizHub - Comprehensive TODO List

> **Generated**: January 2026 | **Updated**: January 2026 (post-refactor)
> **App**: QuizHub - A real-time multiplayer trivia game
> **Stack**: Go 1.24+ (backend) + Vanilla HTML/CSS/JS (frontend) + SQLite (embedded)
> **Architecture**: Single binary with embedded database and static assets

---

## Table of Contents

- [A. Unit Tests](#a-unit-tests)
- [B. Code Quality & Clean Code](#b-code-quality--clean-code)
- [C. Feature / Bug / Improvement Backlog](#c-feature--bug--improvement-backlog)

---

## A. Unit Tests

### A.1 - Test infrastructure

| ID     | Priority | Status | Item                                                                 |
|--------|----------|--------|----------------------------------------------------------------------|
| A.1.1  | P0       | DONE   | Go test framework with `go test ./...`                               |
| A.1.2  | P0       | DONE   | DB layer tests (`internal/db/db_test.go`) - 14 tests, 82% coverage  |
| A.1.3  | P0       | DONE   | Handler tests (`internal/handlers/handlers_test.go`) - 21 tests, 72% coverage |
| A.1.4  | P1       | TODO   | Add CI pipeline (GitHub Actions) to run tests on push/PR             |
| A.1.5  | P1       | TODO   | Middleware unit tests (CORS, Logger, Recover, SecurityHeaders)       |
| A.1.6  | P2       | TODO   | Increase handler coverage to 85%+ (State, edge cases)               |

### A.2 - Frontend tests

| ID     | Priority | Status | Item                                                                 |
|--------|----------|--------|----------------------------------------------------------------------|
| A.2.1  | P1       | TODO   | Add Playwright/Cypress e2e tests for join flow                       |
| A.2.2  | P1       | TODO   | E2E test: full game flow (join -> answer -> leaderboard)             |
| A.2.3  | P2       | TODO   | E2E test: error states (network failure, empty nickname)             |
| A.2.4  | P2       | TODO   | Accessibility audit with axe-core or Lighthouse                      |

---

## B. Code Quality & Clean Code

### B.1 - Project structure

| ID     | Priority | Status | Item                                                                 |
|--------|----------|--------|----------------------------------------------------------------------|
| B.1.1  | P0       | DONE   | `.gitignore` - excludes binaries, DB files, IDE configs              |
| B.1.2  | P0       | DONE   | Removed committed binary (`quizhub`) and QR code artifacts           |
| B.1.3  | P0       | DONE   | `README.md` with project overview, API docs, quick start             |
| B.1.4  | P0       | DONE   | `LICENSE` (MIT)                                                      |
| B.1.5  | P0       | DONE   | `Makefile` with build, run, test, fmt, lint, clean targets           |
| B.1.6  | P1       | TODO   | `CONTRIBUTING.md` - coding standards, PR process                     |
| B.1.7  | P1       | TODO   | Issue/PR templates (`.github/`)                                      |
| B.1.8  | P1       | TODO   | `CODE_OF_CONDUCT.md`                                                 |
| B.1.9  | P2       | TODO   | `CHANGELOG.md`                                                       |

### B.2 - Backend architecture

| ID     | Priority | Status | Item                                                                 |
|--------|----------|--------|----------------------------------------------------------------------|
| B.2.1  | P0       | DONE   | Split monolith into packages: `db/`, `handlers/`, `middleware/`, `models/` |
| B.2.2  | P0       | DONE   | Encapsulated game state in DB instead of global vars                 |
| B.2.3  | P0       | DONE   | HTTP method validation on all endpoints                              |
| B.2.4  | P0       | DONE   | CORS middleware                                                      |
| B.2.5  | P0       | DONE   | Consistent JSON error responses (`{"error":"..."}`)                  |
| B.2.6  | P0       | DONE   | Request logging middleware                                           |
| B.2.7  | P0       | DONE   | Graceful shutdown with signal handling                               |
| B.2.8  | P0       | DONE   | Security headers (X-Content-Type-Options, X-Frame-Options, etc.)     |
| B.2.9  | P0       | DONE   | Panic recovery middleware                                            |
| B.2.10 | P0       | DONE   | Input validation (nickname length, answer range, required fields)    |
| B.2.11 | P0       | DONE   | Server timeouts (read, write, idle)                                  |
| B.2.12 | P1       | TODO   | Rate limiting middleware                                             |
| B.2.13 | P1       | TODO   | API versioning (`/api/v1/`)                                         |
| B.2.14 | P2       | TODO   | Prometheus metrics endpoint                                          |
| B.2.15 | P2       | TODO   | golangci-lint config and pre-commit hooks                            |

### B.3 - Frontend architecture

| ID     | Priority | Status | Item                                                                 |
|--------|----------|--------|----------------------------------------------------------------------|
| B.3.1  | P0       | DONE   | Separated JS into `app.js` (IIFE pattern, strict mode)              |
| B.3.2  | P0       | DONE   | Separated CSS into `app.css` (custom properties, responsive)         |
| B.3.3  | P0       | DONE   | `data-testid` on all interactive elements                            |
| B.3.4  | P0       | DONE   | Client-side input validation with error messages                     |
| B.3.5  | P0       | DONE   | Loading states on all async actions                                  |
| B.3.6  | P0       | DONE   | Error handling on all fetch calls                                    |
| B.3.7  | P0       | DONE   | Semantic HTML (`<header>`, proper structure)                         |
| B.3.8  | P0       | DONE   | `<noscript>` fallback                                                |
| B.3.9  | P0       | DONE   | Favicon                                                              |
| B.3.10 | P0       | DONE   | No inline event handlers (uses `addEventListener` pattern)           |
| B.3.11 | P0       | DONE   | No global variables (wrapped in IIFE)                                |
| B.3.12 | P1       | TODO   | Keyboard accessibility (focus management, aria attributes)           |
| B.3.13 | P2       | TODO   | `<label>` associations for all inputs                                |

---

## C. Feature / Bug / Improvement Backlog

### C.1 - Completed features (previously bugs/missing)

| ID     | Status | Item                                                                       |
|--------|--------|----------------------------------------------------------------------------|
| C.1.1  | DONE   | Question bank with 15 questions across 6 categories (was 1 hardcoded)      |
| C.1.2  | DONE   | Full question display UI with answer buttons                               |
| C.1.3  | DONE   | Leaderboard UI with rankings, gold/silver/bronze styling                   |
| C.1.4  | DONE   | SQLite persistent storage (was in-memory maps)                             |
| C.1.5  | DONE   | Player ID validation in answer handler                                     |
| C.1.6  | DONE   | Duplicate answer prevention (UNIQUE constraint)                            |
| C.1.7  | DONE   | Speed-based scoring (100-1000 points based on response time)               |
| C.1.8  | DONE   | Countdown timer with visual progress bar                                   |
| C.1.9  | DONE   | Answer feedback (correct/wrong with score earned)                          |
| C.1.10 | DONE   | Game flow: lobby -> questions -> finished -> play again                    |
| C.1.11 | DONE   | Add custom questions via API                                               |
| C.1.12 | DONE   | Single binary with `go:embed` for static files                            |
| C.1.13 | DONE   | Configurable port and DB path via environment variables                    |

### C.2 - Remaining features

| ID     | Priority | Item                                                                       |
|--------|----------|----------------------------------------------------------------------------|
| C.2.1  | P1       | Real-time updates via WebSockets/SSE (currently polling every 2s)          |
| C.2.2  | P1       | Admin/Host view - separate interface to control game flow                  |
| C.2.3  | P1       | Room system - multiple concurrent quiz sessions with join codes            |
| C.2.4  | P1       | Authentication for host/admin endpoints                                    |
| C.2.5  | P2       | Sound effects (correct/wrong answers, timer warning)                       |
| C.2.6  | P2       | Dark/light mode toggle                                                     |
| C.2.7  | P2       | Player avatars/profile pictures                                            |
| C.2.8  | P2       | Confetti animation on correct answers                                      |
| C.2.9  | P2       | Question categories filter for game creation                               |
| C.2.10 | P2       | Question difficulty levels                                                 |
| C.2.11 | P2       | Streak bonuses (consecutive correct answers)                               |
| C.2.12 | P2       | Share game results / screenshot export                                     |

### C.3 - Infrastructure

| ID     | Priority | Item                                                                       |
|--------|----------|----------------------------------------------------------------------------|
| C.3.1  | P1       | Dockerfile (multi-stage build for minimal image)                           |
| C.3.2  | P1       | GitHub Actions CI (lint, test, build)                                      |
| C.3.3  | P2       | Release automation with goreleaser                                         |
| C.3.4  | P2       | README badges (build, coverage, Go version, license)                       |
| C.3.5  | P2       | `.editorconfig` for consistent formatting                                  |

---

## Summary

| Category         | Done | TODO | Total |
|------------------|------|------|-------|
| A. Tests         | 3    | 6    | 9     |
| B. Code Quality  | 24   | 8    | 32    |
| C. Features      | 13   | 17   | 30    |
| **Total**        | **40** | **31** | **71** |

**40 of 71 items completed (56%).** All P0 items are done. Remaining work is P1/P2 enhancements.
