# QuizHub Bug Fix PRD

## Original Problem Statement
There is a bug: when admin after adding questions and starting the game, the game says "questions not selected". The requested validation was full UI testing with 2 sample questions and 2 players joining and answering questions. Admin access code: 1234.

## Architecture Decisions
- App is a Go/SQLite/WebSocket QuizHub server embedded behind the FastAPI proxy in `/app/backend/server.py`.
- Frontend is vanilla JavaScript served from embedded Go static files under `/app/web/static`.
- Question selection is now resilient: newly added questions are tracked for the active quiz, and create/start falls back to DB questions if no explicit selected-question cache exists.
- The Go binary `/app/quizhub` was rebuilt and backend restarted so production preview runs the fixed source.

## Implemented
- Fixed admin-created questions not being selected when starting a room/game.
- Hardened room creation/start game flow so it loads available questions and avoids false "no questions selected" failures.
- Confirmed timer payload compatibility with UI (`time_limit`) and full countdown/question/reveal/finish flow.
- Verified with self Playwright flow and independent testing agent: admin added 2 questions, 2 players joined, answered both questions, and final leaderboard displayed.

## Prioritized Backlog
### P0
- None currently; reported game-start bug is fixed and verified.

### P1
- Add a visible success toast after question creation and room creation for clearer host feedback.
- Add a lobby-side question count indicator before starting the game.

### P2
- Modularize `/app/internal/handlers/handlers.go` to reduce future regression risk.
- Add more permanent UI regression tests around WebSocket reconnect and room reset edge cases.

## Next Tasks
- Optional UX polish: improve admin setup clarity around selected questions and game readiness.
