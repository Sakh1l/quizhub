# QuizHub resource-constrained optimization plan

## Constraints

Keep the existing Fly.io `shared-cpu-1x` machine, 256 MB allocation, single SQLite database, one machine, and current deployment shape. Do not add CPU, create a new database, or scale out.

## Planned code changes

1. Cap the SQLite connection pool at one open/idle connection so concurrent player writes are serialized predictably on the existing database.
2. Add bounded retry/backoff for transient SQLite busy/locked errors around player creation. This is a correctness safeguard, not a capacity increase.
3. Return the player HTTP response before running nonessential WebSocket fan-out work. The browser already fetches `/api/players` after entering the lobby, so the full-list broadcast need not block the join response.
4. Preserve the current WebSocket protocol, room behavior, and database schema.
5. Validate with existing unit/E2E tests plus a synchronized 50-player join burst, then deploy only the code/image changes if the tests pass.

## Success criteria

The existing single-machine deployment should continue to use the same CPU, memory, volume, and SQLite file. The synchronized 50-player test should produce zero player-creation HTTP 500s, while the normal smoke flow and WebSocket broadcasts remain functional. Any remaining edge/TLS latency from a single-source burst will be reported separately because it is outside the application’s SQLite critical path.
