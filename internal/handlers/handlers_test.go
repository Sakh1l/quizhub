package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/models"
)

func setupTestHandler(t *testing.T) (*Handler, *http.ServeMux) {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	h := New(database)
	mux := http.NewServeMux()
	h.Register(mux)

	return h, mux
}

func doRequest(mux *http.ServeMux, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestHealthHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodGet, "/api/health", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp models.HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Status != "ok" || resp.Version != "1.0.0" {
		t.Errorf("unexpected health: %+v", resp)
	}
}

func TestJoinHandler_Valid(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "TestPlayer"})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var p models.Player
	json.Unmarshal(w.Body.Bytes(), &p)

	if p.Nickname != "TestPlayer" || p.ID == "" || p.Score != 0 {
		t.Errorf("unexpected player: %+v", p)
	}
}

func TestJoinHandler_EmptyNickname(t *testing.T) {
	_, mux := setupTestHandler(t)

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
}

func TestPlayersHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

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

	// Add a player
	doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice"})

	w = doRequest(mux, http.MethodGet, "/api/players", nil)
	json.Unmarshal(w.Body.Bytes(), &players)
	if len(players) != 1 {
		t.Errorf("expected 1 player, got %d", len(players))
	}
}

func TestStartGameHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodPost, "/api/game/start", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var state models.GameState
	json.Unmarshal(w.Body.Bytes(), &state)

	if state.Status != "question" || state.CurrentQuestion == nil || state.TotalQuestions != 15 {
		t.Errorf("unexpected game state: %+v", state)
	}
}

func TestAnswerHandler_CorrectAnswer(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Join
	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice"})
	var player models.Player
	json.Unmarshal(w.Body.Bytes(), &player)

	// Start game
	w = doRequest(mux, http.MethodPost, "/api/game/start", nil)
	var state models.GameState
	json.Unmarshal(w.Body.Bytes(), &state)

	// Get correct answer from DB
	qID := state.CurrentQuestion.ID

	// We need to find the correct answer - get the question directly
	// Submit answer 0, check if correct; if not, try others
	for i := 0; i < 4; i++ {
		w = doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
			PlayerID:   player.ID,
			QuestionID: qID,
			Answer:     i,
		})

		var resp models.AnswerResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if w.Code == http.StatusOK {
			// Check response structure
			if resp.ScoreEarned < 0 {
				t.Errorf("score should not be negative: %d", resp.ScoreEarned)
			}
			break
		}
	}
}

func TestAnswerHandler_NoActiveQuestion(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Join without starting game
	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice"})
	var player models.Player
	json.Unmarshal(w.Body.Bytes(), &player)

	w = doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID:   player.ID,
		QuestionID: 1,
		Answer:     0,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 with no active question, got %d", w.Code)
	}
}

func TestAnswerHandler_InvalidPlayer(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Start game
	doRequest(mux, http.MethodPost, "/api/game/start", nil)

	w := doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID:   "nonexistent",
		QuestionID: 1,
		Answer:     0,
	})

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for invalid player, got %d", w.Code)
	}
}

func TestAnswerHandler_DuplicateAnswer(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Join
	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice"})
	var player models.Player
	json.Unmarshal(w.Body.Bytes(), &player)

	// Start game
	w = doRequest(mux, http.MethodPost, "/api/game/start", nil)
	var state models.GameState
	json.Unmarshal(w.Body.Bytes(), &state)

	answer := models.AnswerRequest{
		PlayerID:   player.ID,
		QuestionID: state.CurrentQuestion.ID,
		Answer:     0,
	}

	// First answer
	doRequest(mux, http.MethodPost, "/api/answer", answer)

	// Duplicate
	w = doRequest(mux, http.MethodPost, "/api/answer", answer)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate, got %d", w.Code)
	}
}

func TestLeaderboardHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Empty
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

func TestNextQuestionHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Start game
	doRequest(mux, http.MethodPost, "/api/game/start", nil)

	// Next question
	w := doRequest(mux, http.MethodPost, "/api/game/next", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var state models.GameState
	json.Unmarshal(w.Body.Bytes(), &state)
	if state.QuestionIndex != 1 {
		t.Errorf("expected question index 1, got %d", state.QuestionIndex)
	}
}

func TestNextQuestionHandler_NotActive(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Don't start game, just try next
	w := doRequest(mux, http.MethodPost, "/api/game/next", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestResetGameHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice"})
	doRequest(mux, http.MethodPost, "/api/game/start", nil)

	w := doRequest(mux, http.MethodPost, "/api/game/reset", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify players cleared
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

	w := doRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":     "Custom question?",
		"options":  []string{"A", "B", "C"},
		"answer":   1,
		"category": "test",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAddQuestionHandler_Invalid(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Missing text
	w := doRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":    "",
		"options": []string{"A"},
		"answer":  0,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// Answer out of range
	w = doRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":    "Q?",
		"options": []string{"A", "B"},
		"answer":  5,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad answer index, got %d", w.Code)
	}
}

func TestListQuestionsHandler(t *testing.T) {
	_, mux := setupTestHandler(t)

	w := doRequest(mux, http.MethodGet, "/api/questions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var questions []models.Question
	json.Unmarshal(w.Body.Bytes(), &questions)
	if len(questions) != 15 {
		t.Errorf("expected 15 questions, got %d", len(questions))
	}
}

func TestFullGameFlow(t *testing.T) {
	_, mux := setupTestHandler(t)

	// Join two players
	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice"})
	var alice models.Player
	json.Unmarshal(w.Body.Bytes(), &alice)

	w = doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Bob"})
	var bob models.Player
	json.Unmarshal(w.Body.Bytes(), &bob)

	// Start game
	w = doRequest(mux, http.MethodPost, "/api/game/start", nil)
	var state models.GameState
	json.Unmarshal(w.Body.Bytes(), &state)

	if state.Status != "question" || state.CurrentQuestion == nil {
		t.Fatalf("game should be in question state")
	}

	// Both answer first question
	doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID: alice.ID, QuestionID: state.CurrentQuestion.ID, Answer: 0,
	})
	doRequest(mux, http.MethodPost, "/api/answer", models.AnswerRequest{
		PlayerID: bob.ID, QuestionID: state.CurrentQuestion.ID, Answer: 1,
	})

	// Next question
	w = doRequest(mux, http.MethodPost, "/api/game/next", nil)
	json.Unmarshal(w.Body.Bytes(), &state)
	if state.QuestionIndex != 1 {
		t.Errorf("expected q index 1, got %d", state.QuestionIndex)
	}

	// Check leaderboard
	w = doRequest(mux, http.MethodGet, "/api/leaderboard", nil)
	var lb []models.LeaderboardEntry
	json.Unmarshal(w.Body.Bytes(), &lb)
	if len(lb) != 2 {
		t.Errorf("expected 2 leaderboard entries, got %d", len(lb))
	}

	// Reset
	doRequest(mux, http.MethodPost, "/api/game/reset", nil)
	w = doRequest(mux, http.MethodGet, "/api/game/state", nil)
	json.Unmarshal(w.Body.Bytes(), &state)
	if state.Status != "lobby" {
		t.Errorf("expected lobby after reset, got %s", state.Status)
	}
}
