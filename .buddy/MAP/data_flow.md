🏠 [Home](../README_FOR_HUMANS.md) · [Getting Started](../GETTING_STARTED.md) · [Architecture](../ARCHITECTURE.md) · [Tech](../TECH_STACK.md) · [Integrations](../INTEGRATIONS.md) · [Repo Map](../MAP/repo_map.md) · [Links](../LINKS.md)

---

# Data Flow

Simple end-to-end flows with file pointers.

## Admin creates a room
1. Admin UI calls `POST /api/admin/auth` to get a token (internal/handlers.AdminAuth).
2. Admin calls `POST /api/room/create` (handlers.CreateRoom) → generate room code (handlers.generateRoomCode) → DB.CreateRoom stores code and resets state (internal/db/db.go).
3. Hub broadcasts `room_created` event (internal/ws/hub.go).

## Player joins a room
1. Player posts to `POST /api/join` with nickname and room_code (handlers.Join).
2. handlers.Join calls DB.CreatePlayer and broadcasts `player_joined` via hub.
3. Player opens WebSocket `/api/ws?role=player&player_id=...` to receive events.

## Game start and answers
1. Admin selects questions and calls `POST /api/game/start` (handlers.StartGame) which calls DB.StartGame.
2. handlers.startQuestion broadcasts `new_question` and sets a server-side timer.
3. Players submit answers to `POST /api/answer` (handlers.Answer) → DB.SubmitAnswer / RecordAnswer updates answers and scores.
4. After time limit, handlers triggers `time_up`; admin can call `POST /api/game/next` to advance.

These flows show where to look in handlers and DB files for logic.
