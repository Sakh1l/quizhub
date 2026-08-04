package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	quizID := createQuiz(t, mux, tok, "Trivia")
	addQuestion(t, mux, tok, quizID, "1+1?", []string{"0", "1", "2", "3"}, 2)

	w := reqJSON(mux, http.MethodPost, "/api/room/create", map[string]int{"quiz_id": quizID}, tok)
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

func TestCreateRoomRequiresNonEmptyQuiz(t *testing.T) {
	mux := newMux(t)
	tok := adminToken(t, mux)

	w := reqJSON(mux, http.MethodPost, "/api/room/create", nil, tok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 with no quiz_id, got %d body=%s", w.Code, w.Body.String())
	}

	quizID := createQuiz(t, mux, tok, "Empty Quiz")
	w = reqJSON(mux, http.MethodPost, "/api/room/create", map[string]int{"quiz_id": quizID}, tok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for quiz with no questions, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRoomRejectsWhileAnotherIsActive(t *testing.T) {
	mux := newMux(t)
	tok := adminToken(t, mux)

	quizA := createQuiz(t, mux, tok, "Quiz A")
	addQuestion(t, mux, tok, quizA, "Q1?", []string{"A", "B"}, 0)
	createRoom(t, mux, tok, quizA)

	quizB := createQuiz(t, mux, tok, "Quiz B")
	addQuestion(t, mux, tok, quizB, "Q2?", []string{"A", "B"}, 0)

	w := reqJSON(mux, http.MethodPost, "/api/room/create", map[string]int{"quiz_id": quizB}, tok)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 while a quiz is already running, got %d body=%s", w.Code, w.Body.String())
	}

	// Resetting the session frees things up to host a new quiz.
	w = reqJSON(mux, http.MethodPost, "/api/game/reset", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", w.Code, w.Body.String())
	}
	w = reqJSON(mux, http.MethodPost, "/api/room/create", map[string]int{"quiz_id": quizB}, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected room creation to succeed after reset, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestResetGameClearsSessionDataButKeepsQuizzes(t *testing.T) {
	mux := newMux(t)
	tok := adminToken(t, mux)

	quizID := createQuiz(t, mux, tok, "Trivia")
	addQuestion(t, mux, tok, quizID, "1+1?", []string{"0", "1", "2", "3"}, 2)

	w := reqJSON(mux, http.MethodPost, "/api/room/create", map[string]int{"quiz_id": quizID}, tok)
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

	// The quiz and its question survive reset — it's a reusable library, not
	// session-scoped scratch data.
	w = reqJSON(mux, http.MethodGet, "/api/admin/quizzes/questions?quiz_id="+strconv.Itoa(quizID), nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("questions status=%d body=%s", w.Code, w.Body.String())
	}
	var questions []any
	if err := json.Unmarshal(w.Body.Bytes(), &questions); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected the quiz's question to survive reset, got %d", len(questions))
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

	// No quiz is active after reset, so starting a game is rejected.
	w = reqJSON(mux, http.MethodPost, "/api/game/start", nil, tok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected start to be rejected with no active quiz, status=%d body=%s", w.Code, w.Body.String())
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

	quizID := createQuiz(t, mux, tok, "Trivia")
	qID := addQuestion(t, mux, tok, quizID, "2+2?", []string{"1", "2", "4", "8"}, 2)
	selectQuestions(t, mux, tok, []int{qID})
	code := createRoom(t, mux, tok, quizID)
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

	quizID := createQuiz(t, mux, tok, "Trivia")
	first := addQuestion(t, mux, tok, quizID, "First?", []string{"A", "B"}, 0)
	second := addQuestion(t, mux, tok, quizID, "Second?", []string{"A", "B"}, 1)
	third := addQuestion(t, mux, tok, quizID, "Third?", []string{"A", "B"}, 0)
	selectQuestions(t, mux, tok, []int{second, first})
	code := createRoom(t, mux, tok, quizID)
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
	dbState, _ := h.DB.GetGameState()
	if dbState.Status != "finished" || dbState.QuestionID != first || dbState.QuestionID == third {
		t.Fatalf("unexpected final state status=%s current=%d third=%d", dbState.Status, dbState.QuestionID, third)
	}
}

func TestQuizLibraryCRUD(t *testing.T) {
	mux := newMux(t)
	tok := adminToken(t, mux)

	quizID := createQuiz(t, mux, tok, "Original Title")
	addQuestion(t, mux, tok, quizID, "Q1?", []string{"A", "B"}, 0)

	w := reqJSON(mux, http.MethodGet, "/api/admin/quizzes", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("list quizzes status=%d body=%s", w.Code, w.Body.String())
	}
	var quizzes []struct {
		ID            int    `json:"id"`
		Title         string `json:"title"`
		QuestionCount int    `json:"question_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &quizzes); err != nil {
		t.Fatalf("decode quizzes: %v", err)
	}
	if len(quizzes) != 1 || quizzes[0].QuestionCount != 1 {
		t.Fatalf("unexpected quiz list: %+v", quizzes)
	}

	w = reqJSON(mux, http.MethodPost, "/api/admin/quizzes/rename", map[string]any{"id": quizID, "title": "Renamed"}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("rename status=%d body=%s", w.Code, w.Body.String())
	}

	w = reqJSON(mux, http.MethodPost, "/api/admin/quizzes/duplicate", map[string]any{"id": quizID}, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("duplicate status=%d body=%s", w.Code, w.Body.String())
	}
	var dup struct {
		ID            int    `json:"id"`
		Title         string `json:"title"`
		QuestionCount int    `json:"question_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &dup); err != nil {
		t.Fatalf("decode duplicate: %v", err)
	}
	if dup.Title != "Renamed (Copy)" || dup.QuestionCount != 1 {
		t.Fatalf("unexpected duplicate: %+v", dup)
	}

	w = reqJSON(mux, http.MethodPost, "/api/admin/quizzes/delete", map[string]any{"id": quizID}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}

	w = reqJSON(mux, http.MethodGet, "/api/admin/quizzes", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("list quizzes status=%d body=%s", w.Code, w.Body.String())
	}
	quizzes = nil
	_ = json.Unmarshal(w.Body.Bytes(), &quizzes)
	if len(quizzes) != 1 || quizzes[0].ID != dup.ID {
		t.Fatalf("expected only the duplicate to remain: %+v", quizzes)
	}
}

func TestQuizzesRequireAdmin(t *testing.T) {
	mux := newMux(t)
	if w := reqJSON(mux, http.MethodGet, "/api/admin/quizzes", nil, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}
}

type joinedPlayer struct {
	PlayerID string `json:"player_id"`
}

func createQuiz(t *testing.T, mux *http.ServeMux, token, title string) int {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/admin/quizzes", map[string]any{"title": title}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create quiz status=%d body=%s", w.Code, w.Body.String())
	}
	var q struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
		t.Fatalf("decode quiz: %v", err)
	}
	return q.ID
}

func addQuestion(t *testing.T, mux *http.ServeMux, token string, quizID int, text string, options []string, answer int) int {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/admin/quizzes/questions", map[string]any{
		"quiz_id": quizID, "text": text, "options": options, "answer": answer, "category": "test",
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

func createRoom(t *testing.T, mux *http.ServeMux, token string, quizID int) string {
	t.Helper()
	w := reqJSON(mux, http.MethodPost, "/api/room/create", map[string]int{"quiz_id": quizID}, token)
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
