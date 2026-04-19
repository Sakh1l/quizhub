package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/models"
	"github.com/sakh1l/quizhub/internal/ws"
)

const version = "1.0.0"
const countdownDuration = 10 // seconds

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	DB          *db.DB
	Hub         *ws.Hub
	QuestionIDs []int
	TimeLimit   int
	AdminPIN    string
	AdminTokens map[string]bool

	timerMu     sync.Mutex
	activeTimer *time.Timer
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

// --- timer management ---

func (h *Handler) stopTimer() {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()
	if h.activeTimer != nil {
		h.activeTimer.Stop()
		h.activeTimer = nil
	}
}

func (h *Handler) startTimer(d time.Duration, fn func()) {
	h.stopTimer()
	h.timerMu.Lock()
	h.activeTimer = time.AfterFunc(d, fn)
	h.timerMu.Unlock()
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

func (h *Handler) Register(mux *http.ServeMux) {
	// Public
	mux.HandleFunc("/api/health", h.Health)
	mux.HandleFunc("/api/join", methodOnly(http.MethodPost, h.Join))
	mux.HandleFunc("/api/players", methodOnly(http.MethodGet, h.Players))
	mux.HandleFunc("/api/game/state", methodOnly(http.MethodGet, h.State))
	mux.HandleFunc("/api/answer", methodOnly(http.MethodPost, h.Answer))
	mux.HandleFunc("/api/leaderboard", methodOnly(http.MethodGet, h.Leaderboard))
	mux.HandleFunc("/api/categories", methodOnly(http.MethodGet, h.GetCategories))
	mux.HandleFunc("/api/questions", h.QuestionsRouter)

	// Game control (admin only for start/next, public for state)
	mux.HandleFunc("/api/game/start", methodOnly(http.MethodPost, h.StartGame))
	mux.HandleFunc("/api/game/next", methodOnly(http.MethodPost, h.NextQuestion))
	mux.HandleFunc("/api/game/reset", methodOnly(http.MethodPost, h.ResetGame))

	// Admin
	mux.HandleFunc("/api/admin/auth", methodOnly(http.MethodPost, h.AdminAuth))
	mux.HandleFunc("/api/admin/kick", methodOnly(http.MethodPost, h.adminOnly(h.KickPlayer)))
	mux.HandleFunc("/api/admin/timer", methodOnly(http.MethodPost, h.adminOnly(h.SetTimer)))
	mux.HandleFunc("/api/admin/config", methodOnly(http.MethodGet, h.adminOnly(h.GetConfig)))
	mux.HandleFunc("/api/questions/add", methodOnly(http.MethodPost, h.adminOnly(h.AddQuestion)))
	mux.HandleFunc("/api/questions/edit", methodOnly(http.MethodPost, h.adminOnly(h.EditQuestion)))
	mux.HandleFunc("/api/questions/delete", methodOnly(http.MethodPost, h.adminOnly(h.DeleteQuestion)))

	// WebSocket
	mux.HandleFunc("/api/ws", h.Hub.HandleWS)
}

// --- handlers ---

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.HealthResponse{
		Status:      "ok",
		Version:     version,
		PlayerCount: h.DB.PlayerCount(),
	})
}

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

// StartGame: Admin starts → 10s countdown → auto-loads first question.
func (h *Handler) StartGame(w http.ResponseWriter, r *http.Request) {
	ids, err := h.DB.GetQuestionIDs()
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusInternalServerError, "no questions available")
		return
	}

	h.QuestionIDs = ids

	// Set state to countdown
	if err := h.DB.SetGameState("countdown", 0, 0, "", h.TimeLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start countdown")
		return
	}

	// Broadcast countdown event
	h.Hub.Broadcast(ws.EventGameCountdown, map[string]interface{}{
		"duration":        countdownDuration,
		"total_questions": len(ids),
	})

	// After 10 seconds, auto-load first question
	h.startTimer(time.Duration(countdownDuration)*time.Second, func() {
		h.loadQuestion(0)
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "countdown",
		"duration":        countdownDuration,
		"total_questions": len(ids),
	})
}

// loadQuestion loads a question by index, sets state, broadcasts, and starts timer.
func (h *Handler) loadQuestion(idx int) {
	if idx >= len(h.QuestionIDs) {
		// Game finished
		h.DB.SetGameState("finished", 0, idx, "", h.TimeLimit)
		h.Hub.Broadcast(ws.EventGameFinished, map[string]interface{}{
			"status":         "finished",
			"total_questions": len(h.QuestionIDs),
		})
		h.broadcastLeaderboard()
		return
	}

	qID := h.QuestionIDs[idx]
	now := time.Now().UTC().Format(time.RFC3339)
	h.DB.SetGameState("question", qID, idx, now, h.TimeLimit)

	q, _ := h.DB.GetQuestion(qID)
	state := models.GameState{
		Status:          "question",
		CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
		QuestionIndex:   idx,
		TotalQuestions:   len(h.QuestionIDs),
		TimeLeft:        h.TimeLimit,
	}

	h.Hub.Broadcast(ws.EventNewQuestion, state)

	// Start question timer — when it fires, reveal the answer
	h.startTimer(time.Duration(h.TimeLimit)*time.Second, func() {
		h.revealAnswer(qID, idx)
	})
}

// revealAnswer is called when the question timer expires.
func (h *Handler) revealAnswer(qID, idx int) {
	q, err := h.DB.GetQuestion(qID)
	if err != nil {
		return
	}

	h.DB.SetGameState("reveal", qID, idx, "", h.TimeLimit)

	// Broadcast time_up with correct answer to ALL clients
	h.Hub.Broadcast(ws.EventTimeUp, map[string]interface{}{
		"question_id":    qID,
		"correct_answer": q.Answer,
		"question_index": idx,
	})

	// Send personal results to each player
	players, _ := h.DB.ListPlayers()
	for _, p := range players {
		answered := h.DB.HasAnswered(p.ID, qID)
		result := map[string]interface{}{
			"correct":      false,
			"score_earned":  0,
			"total_score":   p.Score,
			"answered":      answered,
		}
		if answered {
			// Check if their answer was correct by looking at the score earned
			_, correct, _ := h.DB.GetPlayerAnswer(p.ID, qID)
			result["correct"] = correct
		}
		h.Hub.SendToPlayer(p.ID, ws.EventYourResult, result)
	}

	// Broadcast updated leaderboard
	h.broadcastLeaderboard()
}

// NextQuestion: Admin advances to next question. Only works in "reveal" state.
func (h *Handler) NextQuestion(w http.ResponseWriter, r *http.Request) {
	status, _, qIdx, _, _, err := h.DB.GetGameState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get game state")
		return
	}

	if status != "reveal" {
		writeError(w, http.StatusBadRequest, "can only advance after answer reveal")
		return
	}

	nextIdx := qIdx + 1
	h.loadQuestion(nextIdx)

	if nextIdx >= len(h.QuestionIDs) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":         "finished",
			"question_index": nextIdx,
			"total_questions": len(h.QuestionIDs),
		})
	} else {
		q, _ := h.DB.GetQuestion(h.QuestionIDs[nextIdx])
		writeJSON(w, http.StatusOK, models.GameState{
			Status:          "question",
			CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
			QuestionIndex:   nextIdx,
			TotalQuestions:   len(h.QuestionIDs),
			TimeLeft:        h.TimeLimit,
		})
	}
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

	if (status == "question" || status == "reveal") && qID > 0 {
		q, qErr := h.DB.GetQuestion(qID)
		if qErr == nil {
			state.CurrentQuestion = &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category}
		}
		if status == "question" {
			if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
				elapsed := int(time.Since(t).Seconds())
				state.TimeLeft = timeLimit - elapsed
				if state.TimeLeft < 0 {
					state.TimeLeft = 0
				}
			}
		} else {
			state.TimeLeft = 0
		}

		// In reveal state, include correct answer
		if status == "reveal" {
			qq, _ := h.DB.GetQuestion(qID)
			state.CorrectAnswer = qq.Answer
		}
	}

	writeJSON(w, http.StatusOK, state)
}

// Answer records a player's answer. Does NOT reveal correct answer.
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

	player, err := h.DB.GetPlayer(req.PlayerID)
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
				// Score: 0-1000 scaled by remaining time ratio
				score = int(1000.0 * float64(limitMs-elapsed) / float64(limitMs))
				if score < 10 {
					score = 10
				}
			} else {
				score = 10 // correct but after timer (edge case)
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

	// Notify admin about answer stats
	total, correctCount, wrongCount := h.DB.GetAnswerStats(qID)
	h.Hub.BroadcastToRole("admin", ws.EventPlayerAnswered, map[string]interface{}{
		"player_id":     req.PlayerID,
		"nickname":      player.Nickname,
		"total_answers": total,
		"correct_count": correctCount,
		"wrong_count":   wrongCount,
	})

	// Return only acknowledgment — no correct/wrong reveal
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recorded": true,
	})
}

func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := h.DB.Leaderboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) ResetGame(w http.ResponseWriter, r *http.Request) {
	h.stopTimer()
	if err := h.DB.ResetGame(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset game")
		return
	}
	h.QuestionIDs = nil
	h.Hub.Broadcast(ws.EventGameReset, map[string]string{"status": "reset"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// --- Admin handlers ---

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
	h.Hub.SendToPlayer(req.PlayerID, ws.EventPlayerKicked, map[string]string{
		"message": "You have been removed from the game",
	})
	h.Hub.DisconnectPlayer(req.PlayerID)
	h.broadcastPlayers()
	writeJSON(w, http.StatusOK, map[string]string{"kicked": player.Nickname})
}

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

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cats, _ := h.DB.GetCategories()
	writeJSON(w, http.StatusOK, models.GameConfig{
		TimeLimit:  h.TimeLimit,
		Categories: cats,
	})
}

func (h *Handler) QuestionsRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.ListQuestions(w, r)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (h *Handler) ListQuestions(w http.ResponseWriter, r *http.Request) {
	questions, err := h.DB.ListAllQuestions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list questions")
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

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
