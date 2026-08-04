package handlers

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"

	"github.com/sakh1l/quizhub/internal/models"
	"github.com/sakh1l/quizhub/internal/ws"
)

func generateRoomCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 6)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		code[i] = chars[n.Int64()]
	}
	return string(code)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.HealthResponse{
		Status:      "ok",
		Version:     version,
		PlayerCount: h.DB.PlayerCount(),
	})
}

func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.URL.Query().Get("role"), "admin") {
		token := strings.TrimSpace(r.URL.Query().Get("admin_token"))
		if token == "" {
			token = r.Header.Get("X-Admin-Token")
		}
		if !h.isAdminTokenValid(token) {
			writeError(w, http.StatusUnauthorized, "admin access required")
			return
		}
	}
	h.Hub.HandleWS(w, r)
}

func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nickname string `json:"nickname"`
		RoomCode string `json:"room_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "nickname and room_code are required")
		return
	}
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.RoomCode = strings.ToUpper(strings.TrimSpace(req.RoomCode))
	if req.Nickname == "" || req.RoomCode == "" {
		writeError(w, http.StatusBadRequest, "nickname and room_code are required")
		return
	}
	if len(req.Nickname) > 30 {
		writeError(w, http.StatusBadRequest, "nickname must be 30 characters or fewer")
		return
	}

	activeCode := h.DB.GetRoomCode()
	if activeCode == "" || !strings.EqualFold(req.RoomCode, activeCode) {
		writeError(w, http.StatusNotFound, "invalid room code")
		return
	}

	state, _ := h.DB.GetGameState()
	if state.Status != "lobby" {
		writeError(w, http.StatusBadRequest, "game already in progress")
		return
	}

	player, err := h.DB.CreatePlayer(req.Nickname)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create player")
		return
	}

	h.Hub.Broadcast(ws.EventPlayerJoined, player)
	h.broadcastPlayers()
	writeJSON(w, http.StatusCreated, player)
}

func (h *Handler) Players(w http.ResponseWriter, r *http.Request) {
	players, err := h.DB.ListPlayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list players")
		return
	}
	writeJSON(w, http.StatusOK, players)
}

func (h *Handler) RoomInfo(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	activeCode := h.DB.GetRoomCode()

	if code == "" || !strings.EqualFold(code, activeCode) {
		writeError(w, http.StatusNotFound, "invalid room code")
		return
	}

	state, _ := h.DB.GetGameState()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"room_code": activeCode,
		"status":    state.Status,
		"joinable":  state.Status == "lobby",
	})
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuizID int `json:"quiz_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.QuizID <= 0 {
		writeError(w, http.StatusBadRequest, "quiz_id is required")
		return
	}

	// This build supports one room at a time (see ROADMAP.md M2 for
	// multi-room). Refuse a second room instead of silently clobbering
	// whatever's already running.
	if h.DB.GetRoomCode() != "" {
		writeError(w, http.StatusConflict, "a quiz is already running — please wait for it to finish, or ask the host to reset the session before starting a new one")
		return
	}

	quiz, err := h.DB.GetQuiz(req.QuizID)
	if err != nil {
		writeError(w, http.StatusNotFound, "quiz not found")
		return
	}
	if quiz.QuestionCount == 0 {
		writeError(w, http.StatusBadRequest, "quiz has no questions")
		return
	}

	code := generateRoomCode()
	if err := h.DB.CreateRoom(code, req.QuizID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create room")
		return
	}

	h.Hub.Broadcast(ws.EventRoomCreated, map[string]string{"room_code": code})
	writeJSON(w, http.StatusCreated, map[string]string{
		"room_code": code,
		"link":      "/?room=" + code,
	})
}

func (h *Handler) broadcastPlayers() {
	players, _ := h.DB.ListPlayers()
	h.Hub.Broadcast(ws.EventPlayersUpdate, players)
}
