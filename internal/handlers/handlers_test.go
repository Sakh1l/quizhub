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

func setupTestHandler(t *testing.T) (*Handler, *http.ServeMux) {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

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

func getAdminToken(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	w := doRequest(mux, http.MethodPost, "/api/admin/auth", map[string]string{"pin": "1234"})
	if w.Code != http.StatusOK {
		t.Fatalf("admin auth failed: %d %s", w.Code, w.Body.String())
	}
	var resp models.AdminAuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("empty admin token")
	}
	return resp.Token
}

func createRoom(t *testing.T, mux *http.ServeMux, token string) string {
	t.Helper()
	w := doAdminRequest(mux, http.MethodPost, "/api/questions/add", map[string]interface{}{
		"text":    "2+2?",
		"options": []string{"1", "2", "4", "8"},
		"answer":  2,
	}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("add question failed: %d %s", w.Code, w.Body.String())
	}

	w = doAdminRequest(mux, http.MethodPost, "/api/room/create", map[string]interface{}{}, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("create room failed: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["room_code"]
}

func TestHealthHandler(t *testing.T) {
	_, mux := setupTestHandler(t)
	w := doRequest(mux, http.MethodGet, "/api/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestJoinRequiresRoomCode(t *testing.T) {
	_, mux := setupTestHandler(t)
	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{"nickname": "Alice"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestJoinWithValidRoom(t *testing.T) {
	_, mux := setupTestHandler(t)
	token := getAdminToken(t, mux)
	roomCode := createRoom(t, mux, token)

	w := doRequest(mux, http.MethodPost, "/api/join", map[string]string{
		"nickname":  "Alice",
		"room_code": roomCode,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStartGameRequiresAdmin(t *testing.T) {
	_, mux := setupTestHandler(t)
	w := doRequest(mux, http.MethodPost, "/api/game/start", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAdminAuthLockoutAfterFailedAttempts(t *testing.T) {
	_, mux := setupTestHandler(t)
	for i := 0; i < 5; i++ {
		w := doRequest(mux, http.MethodPost, "/api/admin/auth", map[string]string{"pin": "0000"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected unauthorized on failed attempt %d, got %d", i+1, w.Code)
		}
	}
	// Immediately after threshold, should still be locked even with correct PIN.
	w := doRequest(mux, http.MethodPost, "/api/admin/auth", map[string]string{"pin": "1234"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected lockout unauthorized, got %d", w.Code)
	}
}

func TestAdminWSRequiresToken(t *testing.T) {
	_, mux := setupTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/ws?role=admin", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
