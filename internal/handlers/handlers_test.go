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
