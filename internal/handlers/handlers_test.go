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

func newMux(t *testing.T) *http.ServeMux {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	h := New(database, ws.NewHub())
	mux := http.NewServeMux()
	h.Register(mux)
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
