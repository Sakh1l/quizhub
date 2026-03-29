package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/models"
)

const version = "1.0.0"

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	DB           *db.DB
	QuestionIDs  []int // shuffled question order for current game
	TimeLimit    int   // seconds per question
}

// New creates a Handler with defaults.
func New(database *db.DB) *Handler {
	return &Handler{
		DB:        database,
		TimeLimit: 15,
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

// --- route registration ---

// Register mounts all API routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.Health)
	mux.HandleFunc("/api/join", methodOnly(http.MethodPost, h.Join))
	mux.HandleFunc("/api/players", methodOnly(http.MethodGet, h.Players))
	mux.HandleFunc("/api/game/start", methodOnly(http.MethodPost, h.StartGame))
	mux.HandleFunc("/api/game/next", methodOnly(http.MethodPost, h.NextQuestion))
	mux.HandleFunc("/api/game/state", methodOnly(http.MethodGet, h.State))
	mux.HandleFunc("/api/answer", methodOnly(http.MethodPost, h.Answer))
	mux.HandleFunc("/api/leaderboard", methodOnly(http.MethodGet, h.Leaderboard))
	mux.HandleFunc("/api/game/reset", methodOnly(http.MethodPost, h.ResetGame))
	mux.HandleFunc("/api/questions", methodOnly(http.MethodGet, h.ListQuestions))
	mux.HandleFunc("/api/questions/add", methodOnly(http.MethodPost, h.AddQuestion))
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

// StartGame begins a new quiz game, shuffling question order.
func (h *Handler) StartGame(w http.ResponseWriter, r *http.Request) {
	ids, err := h.DB.GetQuestionIDs()
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusInternalServerError, "no questions available")
		return
	}

	h.QuestionIDs = ids

	// Set the first question
	now := time.Now().UTC().Format(time.RFC3339)
	if err := h.DB.SetGameState("question", ids[0], 0, now, h.TimeLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start game")
		return
	}

	q, _ := h.DB.GetQuestion(ids[0])
	state := models.GameState{
		Status:         "question",
		CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
		QuestionIndex:  0,
		TotalQuestions:  len(ids),
		TimeLeft:       h.TimeLimit,
	}
	writeJSON(w, http.StatusOK, state)
}

// NextQuestion advances to the next question or finishes the game.
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
		// Game over
		if err := h.DB.SetGameState("finished", 0, nextIdx, "", h.TimeLimit); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to finish game")
			return
		}
		writeJSON(w, http.StatusOK, models.GameState{
			Status:         "finished",
			QuestionIndex:  nextIdx,
			TotalQuestions:  len(h.QuestionIDs),
		})
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
		Status:         "question",
		CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
		QuestionIndex:  nextIdx,
		TotalQuestions:  len(h.QuestionIDs),
		TimeLeft:       h.TimeLimit,
	}
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
		// Calculate remaining time
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

	// Verify player exists
	_, err := h.DB.GetPlayer(req.PlayerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}

	// Verify game is active
	status, qID, _, startedAt, timeLimit, err := h.DB.GetGameState()
	if err != nil || status != "question" {
		writeError(w, http.StatusBadRequest, "no active question")
		return
	}

	// Check for duplicate answer
	if h.DB.HasAnswered(req.PlayerID, qID) {
		writeError(w, http.StatusConflict, "already answered this question")
		return
	}

	// Get the question
	q, err := h.DB.GetQuestion(qID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get question")
		return
	}

	// Calculate score
	correct := req.Answer == q.Answer
	score := 0

	if correct {
		// Speed bonus: faster answers get more points (max 1000, min 100)
		if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
			elapsed := time.Since(t).Milliseconds()
			limitMs := int64(timeLimit) * 1000
			if elapsed < limitMs {
				ratio := float64(limitMs-elapsed) / float64(limitMs)
				score = 100 + int(ratio*900) // 100 to 1000
			} else {
				score = 100 // answered correctly but slow
			}
		} else {
			score = 500 // fallback
		}
	}

	// Record answer
	recorded, err := h.DB.RecordAnswer(req.PlayerID, qID, req.Answer, correct, score)
	if err != nil || !recorded {
		writeError(w, http.StatusInternalServerError, "failed to record answer")
		return
	}

	// Update score
	if score > 0 {
		h.DB.UpdatePlayerScore(req.PlayerID, score)
	}

	// Get updated total
	player, _ := h.DB.GetPlayer(req.PlayerID)

	writeJSON(w, http.StatusOK, models.AnswerResponse{
		Correct:       correct,
		CorrectAnswer: q.Answer,
		ScoreEarned:   score,
		TotalScore:    player.Score,
	})
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

// ResetGame clears all state and returns to lobby.
func (h *Handler) ResetGame(w http.ResponseWriter, r *http.Request) {
	if err := h.DB.ResetGame(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset game")
		return
	}
	h.QuestionIDs = nil
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// ListQuestions returns all available questions (admin).
func (h *Handler) ListQuestions(w http.ResponseWriter, r *http.Request) {
	count := h.DB.QuestionCount()
	var questions []models.Question
	for i := 1; i <= count+20; i++ { // IDs may not be sequential after deletes
		q, err := h.DB.GetQuestion(i)
		if err == nil {
			questions = append(questions, q)
		}
	}
	if questions == nil {
		questions = []models.Question{}
	}
	writeJSON(w, http.StatusOK, questions)
}

// AddQuestion adds a new question to the bank.
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
