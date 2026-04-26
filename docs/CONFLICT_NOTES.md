# PR Conflict Notes

## Context
PR #2 reported merge conflicts against `main`.

## Resolution applied in this branch
- Removed non-runtime, high-churn documentation file `docs/ARCHITECTURE_REVIEW.md` from this branch to reduce overlap.
- Kept PR A enforcement code paths focused on runtime security changes (admin WS auth, admin auth lockout controls, trusted-proxy IP switch, single active admin session token behavior).

## Why this helps
Keeping the branch scoped to runtime enforcement files reduces unnecessary diff overlap and makes merge conflict resolution on core files more straightforward.
