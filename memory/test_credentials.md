# QuizHub Test Credentials

## Admin Panel
- **URL**: `/admin.html`
- **PIN**: `1234` (default, configurable via `QUIZHUB_ADMIN_PIN` env var)
- **Auth flow**: POST `/api/admin/auth` with `{"pin":"1234"}` returns `{"token":"..."}`
- **Usage**: Include `X-Admin-Token: <token>` header in admin-protected requests

## Player
- No authentication required
- Join via POST `/api/join` with `{"nickname":"..."}` 
