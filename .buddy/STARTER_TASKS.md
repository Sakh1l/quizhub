🏠 [Home](./README_FOR_HUMANS.md) · [Getting Started](./GETTING_STARTED.md) · [Architecture](./ARCHITECTURE.md) · [Tech](./TECH_STACK.md) · [Integrations](./INTEGRATIONS.md) · [Repo Map](./MAP/repo_map.md) · [Links](./LINKS.md)

---

# Starter Tasks

Short, safe tasks for your first contributions. Pick one. Do it. Open a small PR.

## Quick wins (10–30 minutes)

- Run the app locally:
  - `make build` then `./quizhub` or `make run`.
  - Open http://localhost:8080 and http://localhost:8080/admin.html (PIN: 1234).
  - File to check: cmd/server/main.go

- Run tests:
  - `make test`
  - If a test fails, copy the failing output and open an issue or PR that fixes a small cause.
  - Files to check: internal/db/db_test.go, internal/handlers/handlers_test.go

- Fix a doc typo:
  - Look for small wording issues in README.md or .buddy/*.md and submit a one-line change.

## Small coding tasks (30–90 minutes)

- Add a unit test for a DB helper or handler endpoint.
  - Pick a function in internal/db or internal/handlers with no tests and add a test.
  - Run `make test` until it passes.

- Improve an error or log message.
  - Search for confusing logs (e.g., generic "failed" messages) and make them clearer.
  - File to edit: internal/db/db.go or internal/handlers/handlers.go

- Add a missing HTTP validation case.
  - Example: find an input that isn't checked and add a handler-side check and test.

## Learning tasks (1–3 hours)

- Trace the WebSocket flow end-to-end.
  - Start at internal/ws/hub.go, then handlers.HandleWS and handlers.startQuestion.
  - Add a short comment in the code explaining the flow.

- Improve the README example that shows how to deploy with Docker or systemd.
  - Make one small change that clarifies a command or environment variable.

## Where to look (file hints)

- Program start: cmd/server/main.go
- Handlers and API: internal/handlers/handlers.go
- DB logic & migrations: internal/db/db.go
- WebSocket hub: internal/ws/hub.go
- Frontend (embedded): web/static/* and web/embed.go
- Build & tasks: Makefile, Dockerfile, .github/workflows/go.yml

## How to make a tiny PR

1. Create a topic branch: `git checkout -b fix/readme-typo`
2. Make the small change.
3. Run `make test`.
4. Commit and push. Open a PR with 1–3 sentences explaining why.

## If you get stuck

- Post the failing command and the exact error text in your team chat.
- Point to the file you edited and the tests you ran.

Next step: Run one quick win now (run app or tests). When done, tell Buddy and I will suggest a follow-up starter task.