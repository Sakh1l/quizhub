package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/models"
	"github.com/sakh1l/quizhub/internal/ws"
)

const version = "1.0.0"

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	DB          *db.DB
	Hub         *ws.Hub
	QuestionIDs []int
	TimeLimit   int
	AdminPIN    string
	AdminTokens map[string]bool // simple token store
}

// New creates a Handler with defaults.
func New(database *db.DB, hub *ws.Hub) *Handler {
	pin := os.Getenv("QUIZHUB_ADMIN_PIN")
	if pin == "" {
		pin = "1234"
	}
	return &Handler{
		DB:          database,
		Hub:         hub,
		TimeLimit:   15,
		AdminPIN:    pin,
		AdminTokens: make(map[string]bool),
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, models.APIError{Error: msg})
}

func methodOnly(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		next(w, r)
	}
}

func (h *Handler) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		if token == "" || !h.AdminTokens[token] {
			writeError(w, http.StatusUnauthorized, "admin access required")
			return
		}
		next(w, r)
	}
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// --- route registration ---

// Register mounts all API routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Public
	mux.HandleFunc("/api/health", h.Health)
	mux.HandleFunc("/api/join", methodOnly(http.MethodPost, h.Join))
	mux.HandleFunc("/api/players", methodOnly(http.MethodGet, h.Players))
	mux.HandleFunc("/api/game/state", methodOnly(http.MethodGet, h.State))
	mux.HandleFunc("/api/answer", methodOnly(http.MethodPost, h.Answer))
	mux.HandleFunc("/api/leaderboard", methodOnly(http.MethodGet, h.Leaderboard))

	// Game control (also used by admin panel)
	mux.HandleFunc("/api/game/start", methodOnly(http.MethodPost, h.StartGame))
	mux.HandleFunc("/api/game/next", methodOnly(http.MethodPost, h.NextQuestion))
	mux.HandleFunc("/api/game/reset", methodOnly(http.MethodPost, h.ResetGame))

	// Admin
	mux.HandleFunc("/api/admin/auth", methodOnly(http.MethodPost, h.AdminAuth))
	mux.HandleFunc("/api/admin/kick", methodOnly(http.MethodPost, h.adminOnly(h.KickPlayer)))
	mux.HandleFunc("/api/admin/timer", methodOnly(http.MethodPost, h.adminOnly(h.SetTimer)))
	mux.HandleFunc("/api/admin/config", methodOnly(http.MethodGet, h.adminOnly(h.GetConfig)))
	mux.HandleFunc("/api/questions", h.QuestionsRouter)
	mux.HandleFunc("/api/questions/add", methodOnly(http.MethodPost, h.adminOnly(h.AddQuestion)))
	mux.HandleFunc("/api/questions/edit", methodOnly(http.MethodPost, h.adminOnly(h.EditQuestion)))
	mux.HandleFunc("/api/questions/delete", methodOnly(http.MethodPost, h.adminOnly(h.DeleteQuestion)))
	mux.HandleFunc("/api/categories", methodOnly(http.MethodGet, h.GetCategories))

	// WebSocket
	mux.HandleFunc("/api/ws", h.Hub.HandleWS)
}

// --- handlers ---

// Health returns server status.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.HealthResponse{
		Status:      "ok",
		Version:     version,
		PlayerCount: h.DB.PlayerCount(),
	})
}

// Join registers a new player.
func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	var req models.JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Nickname == "" {
		writeError(w, http.StatusBadRequest, "nickname is required")
		return
	}

	if len(req.Nickname) > 30 {
		writeError(w, http.StatusBadRequest, "nickname must be 30 characters or fewer")
		return
	}

	player, err := h.DB.CreatePlayer(req.Nickname)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create player")
		return
	}

	// Broadcast to all clients
	h.Hub.Broadcast(ws.EventPlayerJoined, player)
	h.broadcastPlayers()

	writeJSON(w, http.StatusCreated, player)
}

// Players lists all players.
func (h *Handler) Players(w http.ResponseWriter, r *http.Request) {
	players, err := h.DB.ListPlayers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list players")
		return
	}
	writeJSON(w, http.StatusOK, players)
}

// StartGame begins a new quiz game.
func (h *Handler) StartGame(w http.ResponseWriter, r *http.Request) {
	ids, err := h.DB.GetQuestionIDs()
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusInternalServerError, "no questions available")
		return
	}

	h.QuestionIDs = ids
	now := time.Now().UTC().Format(time.RFC3339)
	if err := h.DB.SetGameState("question", ids[0], 0, now, h.TimeLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start game")
		return
	}

	q, _ := h.DB.GetQuestion(ids[0])
	state := models.GameState{
		Status:          "question",
		CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
		QuestionIndex:   0,
		TotalQuestions:   len(ids),
		TimeLeft:        h.TimeLimit,
	}

	// Broadcast game start to all clients
	h.Hub.Broadcast(ws.EventGameStarted, state)

	writeJSON(w, http.StatusOK, state)
}

// NextQuestion advances to the next question or finishes.
func (h *Handler) NextQuestion(w http.ResponseWriter, r *http.Request) {
	status, _, qIdx, _, _, err := h.DB.GetGameState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get game state")
		return
	}
	if status == "lobby" || status == "finished" {
		writeError(w, http.StatusBadRequest, "game is not active")
		return
	}

	nextIdx := qIdx + 1

	if nextIdx >= len(h.QuestionIDs) {
		if err := h.DB.SetGameState("finished", 0, nextIdx, "", h.TimeLimit); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to finish game")
			return
		}
		state := models.GameState{
			Status:         "finished",
			QuestionIndex:  nextIdx,
			TotalQuestions:  len(h.QuestionIDs),
		}
		h.Hub.Broadcast(ws.EventGameFinished, state)
		h.broadcastLeaderboard()
		writeJSON(w, http.StatusOK, state)
		return
	}

	qID := h.QuestionIDs[nextIdx]
	now := time.Now().UTC().Format(time.RFC3339)
	if err := h.DB.SetGameState("question", qID, nextIdx, now, h.TimeLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to advance question")
		return
	}

	q, _ := h.DB.GetQuestion(qID)
	state := models.GameState{
		Status:          "question",
		CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
		QuestionIndex:   nextIdx,
		TotalQuestions:   len(h.QuestionIDs),
		TimeLeft:        h.TimeLimit,
	}

	h.Hub.Broadcast(ws.EventNewQuestion, state)
	writeJSON(w, http.StatusOK, state)
}

// State returns the current game state.
func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	status, qID, qIdx, startedAt, timeLimit, err := h.DB.GetGameState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get game state")
		return
	}

	state := models.GameState{
		Status:         status,
		QuestionIndex:  qIdx,
		TotalQuestions:  len(h.QuestionIDs),
		TimeLeft:       timeLimit,
	}

	if status == "question" && qID > 0 {
		q, qErr := h.DB.GetQuestion(qID)
		if qErr == nil {
			state.CurrentQuestion = &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category}
		}
		if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
			elapsed := int(time.Since(t).Seconds())
			state.TimeLeft = timeLimit - elapsed
			if state.TimeLeft < 0 {
				state.TimeLeft = 0
			}
		}
	}

	writeJSON(w, http.StatusOK, state)
}

// Answer processes a player's answer submission.
func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	var req models.AnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "player_id is required")
		return
	}

	_, err := h.DB.GetPlayer(req.PlayerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}

	status, qID, _, startedAt, timeLimit, err := h.DB.GetGameState()
	if err != nil || status != "question" {
		writeError(w, http.StatusBadRequest, "no active question")
		return
	}

	if h.DB.HasAnswered(req.PlayerID, qID) {
		writeError(w, http.StatusConflict, "already answered this question")
		return
	}

	q, err := h.DB.GetQuestion(qID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get question")
		return
	}

	correct := req.Answer == q.Answer
	score := 0

	if correct {
		if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
			elapsed := time.Since(t).Milliseconds()
			limitMs := int64(timeLimit) * 1000
			if elapsed < limitMs {
				ratio := float64(limitMs-elapsed) / float64(limitMs)
				score = 100 + int(ratio*900)
			} else {
				score = 100
			}
		} else {
			score = 500
		}
	}

	recorded, err := h.DB.RecordAnswer(req.PlayerID, qID, req.Answer, correct, score)
	if err != nil || !recorded {
		writeError(w, http.StatusInternalServerError, "failed to record answer")
		return
	}

	if score > 0 {
		h.DB.UpdatePlayerScore(req.PlayerID, score)
	}

	player, _ := h.DB.GetPlayer(req.PlayerID)

	resp := models.AnswerResponse{
		Correct:       correct,
		CorrectAnswer: q.Answer,
		ScoreEarned:   score,
		TotalScore:    player.Score,
	}

	// Broadcast to admin: someone answered
	total, correctCount, wrongCount := h.DB.GetAnswerStats(qID)
	h.Hub.BroadcastToRole("admin", ws.EventPlayerAnswered, map[string]interface{}{
		"player_id":     req.PlayerID,
		"nickname":      player.Nickname,
		"correct":       correct,
		"total_answers": total,
		"correct_count": correctCount,
		"wrong_count":   wrongCount,
	})

	// Broadcast updated leaderboard
	h.broadcastLeaderboard()

	writeJSON(w, http.StatusOK, resp)
}

// Leaderboard returns sorted player rankings.
func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := h.DB.Leaderboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// ResetGame clears all state.
func (h *Handler) ResetGame(w http.ResponseWriter, r *http.Request) {
	if err := h.DB.ResetGame(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset game")
		return
	}
	h.QuestionIDs = nil
	h.Hub.Broadcast(ws.EventGameReset, map[string]string{"status": "reset"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// --- Admin handlers ---

// AdminAuth verifies PIN and returns a token.
func (h *Handler) AdminAuth(w http.ResponseWriter, r *http.Request) {
	var req models.AdminAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PIN == "" {
		writeError(w, http.StatusBadRequest, "pin is required")
		return
	}

	if req.PIN != h.AdminPIN {
		writeError(w, http.StatusUnauthorized, "invalid pin")
		return
	}

	token := generateToken()
	h.AdminTokens[token] = true

	writeJSON(w, http.StatusOK, models.AdminAuthResponse{Token: token})
}

// KickPlayer removes a player from the game.
func (h *Handler) KickPlayer(w http.ResponseWriter, r *http.Request) {
	var req models.KickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "player_id is required")
		return
	}

	player, err := h.DB.GetPlayer(req.PlayerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}

	if err := h.DB.DeletePlayer(req.PlayerID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to kick player")
		return
	}

	// Notify kicked player and disconnect their WebSocket
	h.Hub.SendToPlayer(req.PlayerID, ws.EventPlayerKicked, map[string]string{
		"message": "You have been removed from the game",
	})
	h.Hub.DisconnectPlayer(req.PlayerID)
	h.broadcastPlayers()

	writeJSON(w, http.StatusOK, map[string]string{"kicked": player.Nickname})
}

// SetTimer updates the time limit per question.
func (h *Handler) SetTimer(w http.ResponseWriter, r *http.Request) {
	var req models.TimerConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.TimeLimit < 5 || req.TimeLimit > 120 {
		writeError(w, http.StatusBadRequest, "time_limit must be between 5 and 120 seconds")
		return
	}
	h.TimeLimit = req.TimeLimit
	writeJSON(w, http.StatusOK, map[string]int{"time_limit": h.TimeLimit})
}

// GetConfig returns current game configuration.
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cats, _ := h.DB.GetCategories()
	writeJSON(w, http.StatusOK, models.GameConfig{
		TimeLimit:  h.TimeLimit,
		Categories: cats,
	})
}

// QuestionsRouter routes GET /api/questions (public) and admin operations.
func (h *Handler) QuestionsRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ListQuestions(w, r)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// ListQuestions returns all questions.
func (h *Handler) ListQuestions(w http.ResponseWriter, r *http.Request) {
	questions, err := h.DB.ListAllQuestions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list questions")
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

// AddQuestion adds a new question.
func (h *Handler) AddQuestion(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Text     string   `json:"text"`
		Options  []string `json:"options"`
		Answer   int      `json:"answer"`
		Category string   `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if q.Text == "" || len(q.Options) < 2 {
		writeError(w, http.StatusBadRequest, "text and at least 2 options are required")
		return
	}
	if q.Answer < 0 || q.Answer >= len(q.Options) {
		writeError(w, http.StatusBadRequest, "answer index out of range")
		return
	}

	id, err := h.DB.AddQuestion(q.Text, q.Options, q.Answer, q.Category)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add question")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "text": q.Text})
}

// EditQuestion updates an existing question.
func (h *Handler) EditQuestion(w http.ResponseWriter, r *http.Request) {
	var req models.EditQuestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Text == "" || len(req.Options) < 2 {
		writeError(w, http.StatusBadRequest, "text and at least 2 options are required")
		return
	}
	if req.Answer < 0 || req.Answer >= len(req.Options) {
		writeError(w, http.StatusBadRequest, "answer index out of range")
		return
	}

	if err := h.DB.UpdateQuestion(req.ID, req.Text, req.Options, req.Answer, req.Category); err != nil {
		writeError(w, http.StatusNotFound, "question not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteQuestion removes a question.
func (h *Handler) DeleteQuestion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID <= 0 {
		writeError(w, http.StatusBadRequest, "valid question id is required")
		return
	}

	if err := h.DB.DeleteQuestion(req.ID); err != nil {
		writeError(w, http.StatusNotFound, "question not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GetCategories returns available question categories.
func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.DB.GetCategories()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get categories")
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

// --- broadcast helpers ---

func (h *Handler) broadcastPlayers() {
	players, err := h.DB.ListPlayers()
	if err != nil {
		return
	}
	h.Hub.Broadcast(ws.EventPlayersUpdate, players)
}

func (h *Handler) broadcastLeaderboard() {
	entries, err := h.DB.Leaderboard()
	if err != nil {
		return
	}
	h.Hub.Broadcast(ws.EventLeaderboard, entries)
}

// StartGameWithCategories starts a game filtered by categories.
func (h *Handler) StartGameWithCategories(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Categories []string `json:"categories"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ids, err := h.DB.GetQuestionIDsByCategories(req.Categories)
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no questions found for selected categories")
		return
	}

	h.QuestionIDs = ids
	now := time.Now().UTC().Format(time.RFC3339)
	if err := h.DB.SetGameState("question", ids[0], 0, now, h.TimeLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start game")
		return
	}

	q, _ := h.DB.GetQuestion(ids[0])
	state := models.GameState{
		Status:          "question",
		CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
		QuestionIndex:   0,
		TotalQuestions:   len(ids),
		TimeLeft:        h.TimeLimit,
	}

	h.Hub.Broadcast(ws.EventGameStarted, state)
	writeJSON(w, http.StatusOK, state)
}

func init() {
	// placeholder to ensure strconv import if needed
}
