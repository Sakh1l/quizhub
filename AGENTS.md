# Repository Guidelines

## Project Structure & Module Organization

QuizHub is a Go application that ships as one binary with embedded static assets. The entry point is `cmd/server/main.go`. Core server code lives under `internal/`: `db` handles SQLite migrations and queries, `handlers` owns HTTP endpoints split by concern, `middleware` contains request wrappers, `models` defines shared structs, and `ws` manages WebSocket broadcasting. The active frontend is embedded from `web/static/` via `web/embed.go`. Go tests sit beside their packages as `*_test.go`.

## Build, Test, and Development Commands

- `make build`: compile `./cmd/server` into `quizhub`.
- `make run`: build, then start the local server, usually on `:8080`.
- `make test`: run all Go tests with verbose output and `-count=1`.
- `make cover`: generate `coverage.out` and open the Go HTML coverage report.
- `make fmt`: run `gofmt -w .`.
- `make lint`: run `golangci-lint run ./...` when installed.
- `docker build -t quizhub .`: validate the container build.

## Coding Style & Naming Conventions

Use idiomatic Go formatted by `gofmt`; keep package names short, lowercase, and aligned with directory names. Public Go identifiers use `PascalCase`, private identifiers use `camelCase`, and tests use `TestName` functions. Keep handler logic in `internal/handlers`, persistence in `internal/db`, and WebSocket fanout in `internal/ws`. Frontend files are plain HTML, CSS, and JavaScript; follow existing DOM naming and CSS class patterns.

## Testing Guidelines

Prefer Go unit tests close to the code under test, especially for database behavior, handler responses, room lifecycle, scoring, and WebSocket-relevant state transitions. Run `make test` before submitting changes. Use `make cover` for broader server changes. Keep tests deterministic; avoid wall-clock timing unless timer behavior is the subject.

## Commit & Pull Request Guidelines

Recent history uses concise imperative subjects such as `Add admin reset session button` and `Fix pre-existing handler/db compile issues and stabilize tests`; some automation commits use `buddy:`. Keep subjects specific and under roughly 72 characters. Pull requests should include a short description, test results, linked issues when relevant, and screenshots for visible frontend/admin changes. Note deployment impact, especially Docker, Fly.io, database paths, or admin authentication.

## Security & Configuration Tips

Do not commit real credentials or production data. Treat files under `memory/` as local project context and review them carefully before sharing. The default admin PIN documented for local use should not be reused for production deployments.
