#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
PIN="${QUIZHUB_ADMIN_PIN:-1234}"

auth_json=$(curl -fsS -X POST "$BASE_URL/api/admin/auth" -H 'Content-Type: application/json' -d "{\"pin\":\"$PIN\"}")
token=$(printf '%s' "$auth_json" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
[ -n "$token" ] || { echo "admin auth failed" >&2; exit 1; }
curl -fsS -X POST "$BASE_URL/api/game/reset" -H 'Content-Type: application/json' -H "X-Admin-Token: $token" >/dev/null

quiz_json=$(curl -fsS -X POST "$BASE_URL/api/admin/quizzes" -H 'Content-Type: application/json' -H "X-Admin-Token: $token" -d '{"title":"Automated Smoke Quiz"}')
quiz_id=$(printf '%s' "$quiz_json" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
[ -n "$quiz_id" ] || { echo "quiz creation failed" >&2; exit 1; }

curl -fsS -X POST "$BASE_URL/api/admin/quizzes/questions" -H 'Content-Type: application/json' -H "X-Admin-Token: $token" -d "{\"quiz_id\":$quiz_id,\"text\":\"What is 2 + 2?\",\"options\":[\"3\",\"4\",\"5\",\"6\"],\"answer\":1,\"category\":\"math\"}" >/dev/null
curl -fsS -X POST "$BASE_URL/api/admin/timer" -H 'Content-Type: application/json' -H "X-Admin-Token: $token" -d '{"time_limit":5}' >/dev/null
room_json=$(curl -fsS -X POST "$BASE_URL/api/room/create" -H 'Content-Type: application/json' -H "X-Admin-Token: $token" -d "{\"quiz_id\":$quiz_id}")
room_code=$(printf '%s' "$room_json" | sed -n 's/.*"room_code":"\([^"]*\)".*/\1/p')
player_json=$(curl -fsS -X POST "$BASE_URL/api/join" -H 'Content-Type: application/json' -d "{\"nickname\":\"Smoke Player\",\"room_code\":\"$room_code\"}")
player_id=$(printf '%s' "$player_json" | sed -n 's/.*"player_id":"\([^"]*\)".*/\1/p')
[ -n "$player_id" ] || { echo "player join failed" >&2; exit 1; }

curl -fsS -X POST "$BASE_URL/api/game/start" -H 'Content-Type: application/json' -H "X-Admin-Token: $token" >/dev/null
status=""
for _ in $(seq 1 18); do
  state=$(curl -fsS "$BASE_URL/api/game/state")
  status=$(printf '%s' "$state" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')
  [ "$status" = "question" ] && break
  sleep 1
done
[ "$status" = "question" ] || { echo "question state not reached: $status" >&2; exit 1; }

question_id=$(printf '%s' "$state" | sed -n 's/.*"current_question":{"id":\([0-9]*\).*/\1/p')
curl -fsS -X POST "$BASE_URL/api/answer" -H 'Content-Type: application/json' -d "{\"player_id\":\"$player_id\",\"question_id\":$question_id,\"answer\":1}" >/dev/null

status=""
for _ in $(seq 1 10); do
  state=$(curl -fsS "$BASE_URL/api/game/state")
  status=$(printf '%s' "$state" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')
  [ "$status" = "reveal" ] && break
  sleep 1
done
[ "$status" = "reveal" ] || { echo "reveal state not reached: $status" >&2; exit 1; }

curl -fsS -X POST "$BASE_URL/api/game/next" -H 'Content-Type: application/json' -H "X-Admin-Token: $token" >/dev/null
final_state=$(curl -fsS "$BASE_URL/api/game/state")
final_status=$(printf '%s' "$final_state" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')
[ "$final_status" = "finished" ] || { echo "finished state not reached: $final_status" >&2; exit 1; }

printf 'E2E smoke passed: room=%s player=%s final_status=%s\n' "$room_code" "$player_id" "$final_status"
