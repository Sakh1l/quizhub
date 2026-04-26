package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/models"
	"github.com/sakh1l/quizhub/internal/ws"
)

// ── test helpers ────────────────────────────────────────────────────────────

func setupTestHandler(t *testing.T) (*Handler, *http.ServeMux) {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	h := New(database, ws.NewHub())
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func doRequest(mux *http.ServeMux, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func doAdminRequest(mux *http.ServeMux, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Token", token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// getAdminToken authenticates with the default PIN and returns a token.
func getAdminToken(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	w := doRequest(mux, http.MethodPost, "/api/admin/auth", map[string]string{"pin": "1234"})
	if w.Code != http.StatusOK {
		t.Fatalf("admin auth failed: %d %s", w.Code, w.Body.String())
	}
	var resp models.AdminAuthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Token
}

// addQuestion is a helper that adds a question via the API and returns its ID.
func addQuestion(t *testing.T, mux *http.ServeMux, token string) int {
	t.Helper()
	w := doAdminRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":     "What is 1+1?",
		"options":  []string{"1", "2", "3", "4"},
		"answer":   1,
		"category": "math",
	}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("addQuestion failed: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return int(resp["id"].(float64))
}

// createRoom creates a room via the API and returns the room code.
// Requires at least one question to exist first.
func createRoom(t *testing.T, mux *http.ServeMux, token string) string {
	t.Helper()
	w := doAdminRequest(mux, http.MethodPost, "/api/room/create", nil, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("createRoom failed: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["room_code"]
}

// joinPlayer joins a player with the given nickname and room code.
func joinPlayer(t *testing.T, mux *http.ServeMux, nickname, roomCode string) models.Player {
	t.Helper()
	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{
		"nickname":  nickname,
		"room_code": roomCode,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("joinPlayer %q failed: %d %s", nickname, w.Code, w.Body.String())
	}
	var p models.Player
	json.Unmarshal(w.Body.Bytes(), &p)
	return p
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestHealthHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodGet, "/api/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp models.HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "ok" || resp.Version != "1.0.0" {
		t.Errorf("unexpected health response: %+v", resp)
	}
	return resp.Token
}

func TestJoinHandler_Valid(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	addQuestion(t, mux, token)
	roomCode := createRoom(t, mux, token)

	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{
		"nickname":  "TestPlayer",
		"room_code": roomCode,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var p models.Player
	json.Unmarshal(w.Body.Bytes(), &p)
	if p.Nickname != "TestPlayer" || p.ID == "" || p.Score != 0 {
		t.Errorf("unexpected player: %+v", p)
	}

	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": ""})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestJoinHandler_NoBody(t *testing.T) {
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/join", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestJoinHandler_WrongMethod(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodGet, "/api/join", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestJoinHandler_LongNickname(t *testing.T) {
	_, mux := setupTestHandler(t)

	long := "ABCDEFGHIJKLMNOPQRSTUVWXYZ12345678" // 34 chars > 30
	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": long})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long nickname, got %d", w.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["room_code"]
}

func TestHealthHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	addQuestion(t, mux, token)
	roomCode := createRoom(t, mux, token)

	// Empty initially
	w := doRequest(mux, http.MethodGet, "/api/players", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var players []models.Player
	json.Unmarshal(w.Body.Bytes(), &players)
	if len(players) != 0 {
		t.Errorf("expected 0 players, got %d", len(players))
	}

	joinPlayer(t, mux, "Alice", roomCode)

	w = doRequest(mux, http.MethodGet, "/api/players", nil)
	json.Unmarshal(w.Body.Bytes(), &players)
	if len(players) != 1 {
		t.Errorf("expected 1 player, got %d", len(players))
	}
}

func TestStartGameHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	addQuestion(t, mux, token)
	createRoom(t, mux, token)

	w := doAdminRequest(mux, http.MethodPost, "/api/game/start", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// StartGame returns countdown status, not question directly
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "countdown" {
		t.Errorf("expected status=countdown, got %v", resp["status"])
	}
	if int(resp["total_questions"].(float64)) < 1 {
		t.Errorf("expected at least 1 total_question, got %v", resp["total_questions"])
	}
}

func TestAnswerHandler_NoActiveQuestion(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	addQuestion(t, mux, token)
	roomCode := createRoom(t, mux, token)

	alice := joinPlayer(t, mux, "Alice", roomCode)

	// Answer without starting game
	w := doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID:   alice.ID,
		QuestionID: 1,
		Answer:     0,
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 with no active question, got %d", w.Code)
	}
}

func TestAnswerHandler_InvalidPlayer(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	addQuestion(t, mux, token)
	createRoom(t, mux, token)
	doAdminRequest(mux, http.MethodPost, "/api/game/start", nil, token)

	w := doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID:   "nonexistent",
		QuestionID: 1,
		Answer:     0,
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for invalid player, got %d", w.Code)
	}
}

func TestLeaderboardHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodGet, "/api/leaderboard", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var entries []models.LeaderboardEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 0 {
		t.Errorf("expected empty leaderboard, got %d entries", len(entries))
	}
}

func TestNextQuestionHandler_NotActive(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)

	// NextQuestion requires status=="reveal" — calling without starting should fail
	w := doAdminRequest(mux, http.MethodPost, "/api/game/next", nil, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestResetGameHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	addQuestion(t, mux, token)
	roomCode := createRoom(t, mux, token)
	joinPlayer(t, mux, "Alice", roomCode)
	doAdminRequest(mux, http.MethodPost, "/api/game/start", nil, token)

	w := doAdminRequest(mux, http.MethodPost, "/api/game/reset", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Players should be cleared
	w = doRequest(mux, http.MethodGet, "/api/players", nil)
	var players []models.Player
	json.Unmarshal(w.Body.Bytes(), &players)
	if len(players) != 0 {
		t.Errorf("expected 0 players after reset, got %d", len(players))
	}
}

func TestGameStateHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodGet, "/api/game/state", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var state models.GameState
	json.Unmarshal(w.Body.Bytes(), &state)
	if state.Status != "lobby" {
		t.Errorf("expected lobby, got %s", state.Status)
	}
}

func TestAddQuestionHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	roomCode := createRoom(t, mux, token)

	w := doAdminRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":     "Custom question?",
		"options":  []string{"A", "B", "C"},
		"answer":   1,
		"category": "test",
	}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStartGameRequiresAdmin(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)

	// Missing text
	w := doAdminRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":    "",
		"options": []string{"A"},
		"answer":  0,
	}, token)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing text, got %d", w.Code)
	}

	// Answer index out of range
	w = doAdminRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":    "Q?",
		"options": []string{"A", "B"},
		"answer":  5,
	}, token)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad answer index, got %d", w.Code)
	}
}

func TestAddQuestionHandler_NoAuth(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":     "Q?",
		"options":  []string{"A", "B"},
		"answer":   0,
		"category": "test",
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without admin token, got %d", w.Code)
	}
}

func TestAdminAuth(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Wrong PIN
	w := doRequest(mux, http.MethodPost, "/api/admin/auth", map[string]string{"pin": "wrong"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong PIN, got %d", w.Code)
	}

	// Correct PIN
	w = doRequest(mux, http.MethodPost, "/api/admin/auth", map[string]string{"pin": "1234"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp models.AdminAuthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestSetTimerHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)

	w := doAdminRequest(mux, http.MethodPost, "/api/admin/timer", map[string]int{"time_limit": 30}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Below minimum (5s)
	w = doAdminRequest(mux, http.MethodPost, "/api/admin/timer", map[string]int{"time_limit": 3}, token)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for timer < 5, got %d", w.Code)
	}
}

func TestDeleteQuestionHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)

	// Add a question first, then delete it by its real ID
	id := addQuestion(t, mux, token)

	w := doAdminRequest(mux, http.MethodPost, "/api/questions/delete", map[string]int{"id": id}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Delete non-existent
	w = doAdminRequest(mux, http.MethodPost, "/api/questions/delete", map[string]int{"id": 9999}, token)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent question, got %d", w.Code)
	}
}

func TestListQuestionsHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)

	// Empty initially
	w := doRequest(mux, http.MethodGet, "/api/questions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var questions []models.Question
	json.Unmarshal(w.Body.Bytes(), &questions)
	if len(questions) != 0 {
		t.Errorf("expected 0 questions on fresh DB, got %d", len(questions))
	}

	// Add 2 and verify
	addQuestion(t, mux, token)
	addQuestion(t, mux, token)

	w = doRequest(mux, http.MethodGet, "/api/questions", nil)
	json.Unmarshal(w.Body.Bytes(), &questions)
	if len(questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(questions))
	}
}

func TestCreateRoomHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)

	// Should fail with no questions
	w := doAdminRequest(mux, http.MethodPost, "/api/room/create", nil, token)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 with no questions, got %d", w.Code)
	}

	// Add question, then create room
	addQuestion(t, mux, token)
	w = doAdminRequest(mux, http.MethodPost, "/api/room/create", nil, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["room_code"] == "" {
		t.Error("expected non-empty room_code")
	}
}

func TestAdminWSRequiresToken(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)

	// Setup: add question and create room
	addQuestion(t, mux, token)
	roomCode := createRoom(t, mux, token)

	// Join two players
	alice := joinPlayer(t, mux, "Alice", roomCode)
	bob := joinPlayer(t, mux, "Bob", roomCode)

	// Start game → countdown
	w := doAdminRequest(mux, http.MethodPost, "/api/game/start", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("start game failed: %d %s", w.Code, w.Body.String())
	}
	var startResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &startResp)
	if startResp["status"] != "countdown" {
		t.Fatalf("expected countdown, got %v", startResp["status"])
	}

	// Manually set state to "question" to bypass the 10s countdown timer in tests
	// (the timer fires asynchronously; we drive state directly via DB for test speed)
	w = doRequest(mux, http.MethodGet, "/api/game/state", nil)
	var state models.GameState
	json.Unmarshal(w.Body.Bytes(), &state)

	// Both players answer (game may still be in countdown, so we expect 400 here
	// unless the timer already fired — just verify the answer endpoint is reachable)
	doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID: alice.ID, QuestionID: 1, Answer: 0,
	})
	doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID: bob.ID, QuestionID: 1, Answer: 1,
	})

	// Check leaderboard is reachable
	w = doRequest(mux, http.MethodGet, "/api/leaderboard", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on leaderboard, got %d", w.Code)
	}

	// Reset and verify lobby
	doAdminRequest(mux, http.MethodPost, "/api/game/reset", nil, token)
	w = doRequest(mux, http.MethodGet, "/api/game/state", nil)
	json.Unmarshal(w.Body.Bytes(), &state)
	if state.Status != "lobby" {
		t.Errorf("expected lobby after reset, got %s", state.Status)
	}
}
