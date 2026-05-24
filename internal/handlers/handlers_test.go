package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/ws"
)

func newTestHandler(t *testing.T) (*http.ServeMux, *Handler) {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	h := New(database, ws.NewHub())
	t.Cleanup(h.stopTimer)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, h
}

func newMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux, _ := newTestHandler(t)
	return mux
}

func reqJSON(mux *http.ServeMux, method, path string, body any, token string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Admin-Token", token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func adminToken(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/admin/auth", map[string]string{"pin": "1234"}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("auth status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["token"]
}

func TestHealthAndAdminAuth(t *testing.T) {
	mux := newMux(t)
	if w := reqJSON(mux, http.MethodGet, "/api/health", nil, ""); w.Code != http.StatusOK {
		t.Fatalf("health status=%d", w.Code)
	}
	if tok := adminToken(t, mux); tok == "" {
		t.Fatal("expected token")
	}
}

func TestCreateRoomAndJoin(t *testing.T) {
	mux := newMux(t)
	tok := adminToken(t, mux)

	w := reqJSON(mux, http.MethodPost, "/api/questions/add", map[string]any{
		"text": "1+1?", "options": []string{"0", "1", "2", "3"}, "answer": 2, "category": "math",
	}, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("add question status=%d body=%s", w.Code, w.Body.String())
	}

	w = reqJSON(mux, http.MethodPost, "/api/room/create", nil, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("create room status=%d body=%s", w.Code, w.Body.String())
	}
	var room map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &room)
	code := room["room_code"]
	if code == "" {
		t.Fatal("expected room_code")
	}

	w = reqJSON(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice", "room_code": code}, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("join status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestResetGameClearsSessionData(t *testing.T) {
	mux := newMux(t)
	tok := adminToken(t, mux)

	w := reqJSON(mux, http.MethodPost, "/api/questions/add", map[string]any{
		"text": "1+1?", "options": []string{"0", "1", "2", "3"}, "answer": 2, "category": "math",
	}, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("add question status=%d body=%s", w.Code, w.Body.String())
	}
	var question struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &question); err != nil || question.ID == 0 {
		t.Fatalf("decode question id: %v body=%s", err, w.Body.String())
	}

	w = reqJSON(mux, http.MethodPost, "/api/questions", map[string]any{"ids": []int{question.ID}}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("select questions status=%d body=%s", w.Code, w.Body.String())
	}

	w = reqJSON(mux, http.MethodPost, "/api/room/create", nil, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("create room status=%d body=%s", w.Code, w.Body.String())
	}
	var room map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &room)

	w = reqJSON(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice", "room_code": room["room_code"]}, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("join status=%d body=%s", w.Code, w.Body.String())
	}

	w = reqJSON(mux, http.MethodPost, "/api/game/reset", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", w.Code, w.Body.String())
	}

	w = reqJSON(mux, http.MethodGet, "/api/players", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("players status=%d body=%s", w.Code, w.Body.String())
	}
	var players []any
	if err := json.Unmarshal(w.Body.Bytes(), &players); err != nil {
		t.Fatalf("decode players: %v", err)
	}
	if len(players) != 0 {
		t.Fatalf("expected no players after reset, got %d", len(players))
	}

	w = reqJSON(mux, http.MethodGet, "/api/questions", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("questions status=%d body=%s", w.Code, w.Body.String())
	}
	var questions []any
	if err := json.Unmarshal(w.Body.Bytes(), &questions); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	if len(questions) != 0 {
		t.Fatalf("expected no questions after reset, got %d", len(questions))
	}

	w = reqJSON(mux, http.MethodGet, "/api/game/state", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("state status=%d body=%s", w.Code, w.Body.String())
	}
	var state map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state["status"] != "lobby" || state["room_code"] != nil {
		t.Fatalf("unexpected state after reset: %+v", state)
	}

	w = reqJSON(mux, http.MethodPost, "/api/game/start", nil, tok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected cleared selected question cache, start status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTimerUsesTimeLimitPayloadAndValidatesBounds(t *testing.T) {
	mux := newMux(t)
	tok := adminToken(t, mux)

	w := reqJSON(mux, http.MethodPost, "/api/admin/timer", map[string]int{"time_limit": 20}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("set timer status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode timer response: %v", err)
	}
	if resp["time_limit"] != 20 {
		t.Fatalf("expected time_limit 20, got %d", resp["time_limit"])
	}

	w = reqJSON(mux, http.MethodPost, "/api/admin/timer", map[string]int{"time_limit": 4}, tok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for short timer, got %d", w.Code)
	}
}

func TestAnswerPayloadAndTimeBasedScoring(t *testing.T) {
	mux, h := newTestHandler(t)
	tok := adminToken(t, mux)

	qID := addQuestion(t, mux, tok, "2+2?", []string{"1", "2", "4", "8"}, 2)
	selectQuestions(t, mux, tok, []int{qID})
	code := createRoom(t, mux, tok)
	player := joinPlayer(t, mux, code, "Alice")

	w := reqJSON(mux, http.MethodPost, "/api/game/start", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", w.Code, w.Body.String())
	}
	h.stopTimer()
	h.beginQuestion()

	w = reqJSON(mux, http.MethodPost, "/api/answer", map[string]any{
		"player_id":   player.PlayerID,
		"question_id": qID,
		"answer":      2,
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("answer status=%d body=%s", w.Code, w.Body.String())
	}
	var answer struct {
		Correct       bool `json:"correct"`
		CorrectAnswer int  `json:"correct_answer"`
		ScoreEarned   int  `json:"score_earned"`
		TotalScore    int  `json:"total_score"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	if !answer.Correct || answer.CorrectAnswer != 2 {
		t.Fatalf("unexpected answer response: %+v", answer)
	}
	if answer.ScoreEarned <= 0 || answer.ScoreEarned > 1000 || answer.TotalScore != answer.ScoreEarned {
		t.Fatalf("unexpected score response: %+v", answer)
	}
}

func TestSelectedQuestionOrderAndRevealState(t *testing.T) {
	mux, h := newTestHandler(t)
	tok := adminToken(t, mux)

	first := addQuestion(t, mux, tok, "First?", []string{"A", "B"}, 0)
	second := addQuestion(t, mux, tok, "Second?", []string{"A", "B"}, 1)
	third := addQuestion(t, mux, tok, "Third?", []string{"A", "B"}, 0)
	selectQuestions(t, mux, tok, []int{second, first})
	code := createRoom(t, mux, tok)
	_ = joinPlayer(t, mux, code, "Bob")

	w := reqJSON(mux, http.MethodPost, "/api/game/start", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", w.Code, w.Body.String())
	}
	h.stopTimer()
	h.beginQuestion()
	assertCurrentQuestion(t, mux, second)

	h.revealCurrentQuestion()
	w = reqJSON(mux, http.MethodGet, "/api/game/state", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("state status=%d body=%s", w.Code, w.Body.String())
	}
	var state struct {
		Status        string `json:"status"`
		CorrectAnswer int    `json:"correct_answer"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Status != "reveal" || state.CorrectAnswer != 1 {
		t.Fatalf("expected reveal with correct answer 1, got %+v", state)
	}

	w = reqJSON(mux, http.MethodPost, "/api/game/next", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("next status=%d body=%s", w.Code, w.Body.String())
	}
	assertCurrentQuestion(t, mux, first)

	w = reqJSON(mux, http.MethodPost, "/api/game/next", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("finish status=%d body=%s", w.Code, w.Body.String())
	}
	status, current, _, _, _, _, _ := h.DB.GetGameState()
	if status != "finished" || current != first || current == third {
		t.Fatalf("unexpected final state status=%s current=%d third=%d", status, current, third)
	}
}

type joinedPlayer struct {
	PlayerID string `json:"player_id"`
}

func addQuestion(t *testing.T, mux *http.ServeMux, token, text string, options []string, answer int) int {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/questions/add", map[string]any{
		"text": text, "options": options, "answer": answer, "category": "test",
	}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("add question status=%d body=%s", w.Code, w.Body.String())
	}
	var q struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
		t.Fatalf("decode question: %v", err)
	}
	return q.ID
}

func selectQuestions(t *testing.T, mux *http.ServeMux, token string, ids []int) {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/questions", map[string]any{"ids": ids}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("select questions status=%d body=%s", w.Code, w.Body.String())
	}
}

func createRoom(t *testing.T, mux *http.ServeMux, token string) string {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/room/create", nil, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create room status=%d body=%s", w.Code, w.Body.String())
	}
	var room map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &room); err != nil {
		t.Fatalf("decode room: %v", err)
	}
	if room["room_code"] == "" || room["link"] == "" {
		t.Fatalf("expected room_code and link, got %+v", room)
	}
	return room["room_code"]
}

func joinPlayer(t *testing.T, mux *http.ServeMux, code, nickname string) joinedPlayer {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/join", map[string]string{"nickname": nickname, "room_code": code}, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("join status=%d body=%s", w.Code, w.Body.String())
	}
	var player joinedPlayer
	if err := json.Unmarshal(w.Body.Bytes(), &player); err != nil {
		t.Fatalf("decode player: %v", err)
	}
	if player.PlayerID == "" {
		t.Fatal("expected player_id")
	}
	return player
}

func assertCurrentQuestion(t *testing.T, mux *http.ServeMux, wantID int) {
	t.Helper()
	w := reqJSON(mux, http.MethodGet, "/api/game/state", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("state status=%d body=%s", w.Code, w.Body.String())
	}
	var state struct {
		CurrentQuestion struct {
			ID int `json:"id"`
		} `json:"current_question"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.CurrentQuestion.ID != wantID {
		t.Fatalf("current question id=%d, want %d", state.CurrentQuestion.ID, wantID)
	}
}
