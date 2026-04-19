# QuizHub Test Credentials

## Admin Panel
- **URL**: `/admin.html`
- **PIN**: `1234` (default, configurable via `QUIZHUB_ADMIN_PIN` env var)
- **Auth flow**: POST `/api/admin/auth` with `{"pin":"1234"}` returns `{"token":"..."}`
- **Usage**: Include `X-Admin-Token: <token>` header in admin-protected requests

## Player
- **URL**: `/` (landing page)
- Join via POST `/api/join` with `{"nickname":"...","room_code":"..."}` 
- Room code is provided by admin after creating a quiz room
- Pre-fill room code via URL: `/?room=CODE`
