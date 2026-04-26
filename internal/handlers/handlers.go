package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/models"
	"github.com/sakh1l/quizhub/internal/ws"
)

const version = "1.0.0"
const countdownDuration = 10
const authRateWindow = 10 * time.Minute
const authMaxFailures = 5

type authAttempt struct {
	failures     int
	windowStart  time.Time
	lockoutUntil time.Time
}

type Handler struct {
	DB           *db.DB
	Hub          *ws.Hub
	AdminPIN     string
	tokenTTL     time.Duration
	stateMu      sync.RWMutex
	questionIDs  []int
	timeLimit    int
	adminTokens  map[string]time.Time
	authAttempts map[string]authAttempt
	trustProxy   bool
	timerMu      sync.Mutex
	activeTimer  *time.Timer
	DB          *db.DB
	Hub         *ws.Hub
	QuestionIDs []int
	TimeLimit   int
	AdminPIN    string
	AdminTokens map[string]bool
	mu          sync.RWMutex // protects QuestionIDs
	timerMu     sync.Mutex
	activeTimer *time.Timer
}

func New(database *db.DB, hub *ws.Hub) *Handler {
	pin := os.Getenv("QUIZHUB_ADMIN_PIN")
	if pin == "" {
		pin = "1234"
	}
	tokenTTL := 4 * time.Hour
	if raw := strings.TrimSpace(os.Getenv("QUIZHUB_ADMIN_TOKEN_TTL_MIN")); raw != "" {
		if mins, err := time.ParseDuration(raw + "m"); err == nil && mins > 0 {
			tokenTTL = mins
		}
	}
	trustProxy := strings.EqualFold(strings.TrimSpace(os.Getenv("QUIZHUB_TRUST_PROXY")), "true")
	return &Handler{
		DB:           database,
		Hub:          hub,
		timeLimit:    15,
		AdminPIN:     pin,
		tokenTTL:     tokenTTL,
		adminTokens:  make(map[string]time.Time),
		authAttempts: make(map[string]authAttempt),
		trustProxy:   trustProxy,
	}
}

func (h *Handler) getQuestionIDs() []int {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	return append([]int(nil), h.questionIDs...)
}

func (h *Handler) setQuestionIDs(ids []int) {
	h.stateMu.Lock()
	h.questionIDs = append([]int(nil), ids...)
	h.stateMu.Unlock()
}

func (h *Handler) clearQuestionIDs() {
	h.stateMu.Lock()
	h.questionIDs = nil
	h.stateMu.Unlock()
}

func (h *Handler) getTimeLimit() int {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	return h.timeLimit
}

func (h *Handler) setTimeLimit(v int) {
	h.stateMu.Lock()
	h.timeLimit = v
	h.stateMu.Unlock()
}

func (h *Handler) setAdminToken(token string) {
	h.stateMu.Lock()
	clear(h.adminTokens) // single active admin session
	h.adminTokens[token] = time.Now().Add(h.tokenTTL)
	h.stateMu.Unlock()
}

func (h *Handler) getClientIP(r *http.Request) string {
	if h.trustProxy {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); ip != "" {
					return ip
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) isAuthLocked(ip string, now time.Time) bool {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	attempt, ok := h.authAttempts[ip]
	if !ok {
		return false
	}
	if !attempt.lockoutUntil.IsZero() && now.Before(attempt.lockoutUntil) {
		return true
	}
	if !attempt.lockoutUntil.IsZero() && !now.Before(attempt.lockoutUntil) {
		delete(h.authAttempts, ip)
	}
	return false
}

func (h *Handler) recordAuthFailure(ip string, now time.Time) {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	attempt := h.authAttempts[ip]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) > authRateWindow {
		attempt.windowStart = now
		attempt.failures = 0
		attempt.lockoutUntil = time.Time{}
	}
	attempt.failures++
	if attempt.failures >= authMaxFailures {
		attempt.lockoutUntil = now.Add(authRateWindow)
	}
	h.authAttempts[ip] = attempt
}

func (h *Handler) clearAuthFailures(ip string) {
	h.stateMu.Lock()
	delete(h.authAttempts, ip)
	h.stateMu.Unlock()
}

func (h *Handler) isAdminTokenValid(token string) bool {
	if token == "" {
		return false
	}
	now := time.Now()
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	for t, expiresAt := range h.adminTokens {
		if expiresAt.Before(now) {
			delete(h.adminTokens, t)
		}
	}
	expiresAt, ok := h.adminTokens[token]
	return ok && expiresAt.After(now)
}

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
		if !h.isAdminTokenValid(token) {
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

func generateRoomCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1 to avoid confusion
	code := make([]byte, 6)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		code[i] = chars[n.Int64()]
	}
	return string(code)
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
	mux.HandleFunc("/api/room/info", methodOnly(http.MethodGet, h.RoomInfo))

	// Game control
	mux.HandleFunc("/api/game/start", methodOnly(http.MethodPost, h.adminOnly(h.StartGame)))
	mux.HandleFunc("/api/game/next", methodOnly(http.MethodPost, h.adminOnly(h.NextQuestion)))
	mux.HandleFunc("/api/game/reset", methodOnly(http.MethodPost, h.adminOnly(h.ResetGame)))

	// Admin
	mux.HandleFunc("/api/admin/auth", methodOnly(http.MethodPost, h.AdminAuth))
	mux.HandleFunc("/api/admin/timer", methodOnly(http.MethodPost, h.adminOnly(h.SetTimer)))
	mux.HandleFunc("/api/room/create", methodOnly(http.MethodPost, h.adminOnly(h.CreateRoom)))
	mux.HandleFunc("/api/questions/add", methodOnly(http.MethodPost, h.adminOnly(h.AddQuestion)))
	mux.HandleFunc("/api/questions/delete", methodOnly(http.MethodPost, h.adminOnly(h.DeleteQuestion)))
	mux.HandleFunc("/api/questions", h.QuestionsRouter)

	// WebSocket
	mux.HandleFunc("/api/ws", h.HandleWS)
}

// --- handlers ---

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

// Join requires room_code + nickname.
func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nickname string `json:"nickname"`
		RoomCode string `json:"room_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Nickname == "" || req.RoomCode == "" {
		writeError(w, http.StatusBadRequest, "nickname and room_code are required")
		return
	}
	if len(req.Nickname) > 30 {
		writeError(w, http.StatusBadRequest, "nickname must be 30 characters or fewer")
		return
	}

	// Validate room code
	activeCode := h.DB.GetRoomCode()
	if activeCode == "" || !strings.EqualFold(req.RoomCode, activeCode) {
		writeError(w, http.StatusNotFound, "invalid room code")
		return
	}

	// Check game hasn't already started
	status, _, _, _, _, _, _ := h.DB.GetGameState()
	if status != "lobby" {
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

// RoomInfo returns the current room status (for players to check).
func (h *Handler) RoomInfo(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	activeCode := h.DB.GetRoomCode()

	if code == "" || activeCode == "" || !strings.EqualFold(code, activeCode) {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	status, _, _, _, _, _, _ := h.DB.GetGameState()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"room_code": activeCode,
		"status":    status,
		"joinable":  status == "lobby",
	})
}

// CreateRoom generates a unique room code.
func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	// Must have at least 1 question
	if h.DB.QuestionCount() == 0 {
		writeError(w, http.StatusBadRequest, "add at least one question before creating a room")
		return
	}

	code := generateRoomCode()
	if err := h.DB.SetRoomCode(code); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create room")
		return
	}

	// Build shareable link
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") != "" {
		scheme = r.Header.Get("X-Forwarded-Proto")
	}
	link := scheme + "://" + host + "/?room=" + code

	writeJSON(w, http.StatusCreated, map[string]string{
		"room_code": code,
		"link":      link,
	})
}

// StartGame: Admin starts → 10s countdown → auto-loads first question.
func (h *Handler) StartGame(w http.ResponseWriter, r *http.Request) {
	roomCode := h.DB.GetRoomCode()
	if roomCode == "" {
		writeError(w, http.StatusBadRequest, "create a room first")
		return
	}

	ids, err := h.DB.GetQuestionIDs()
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusInternalServerError, "no questions available")
		return
	}

	h.setQuestionIDs(ids)
	timeLimit := h.getTimeLimit()
	if err := h.DB.SetGameState("countdown", 0, 0, "", timeLimit); err != nil {
	h.mu.Lock()
	h.QuestionIDs = ids
	h.mu.Unlock()
	if err := h.DB.SetGameState("countdown", 0, 0, "", h.TimeLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start countdown")
		return
	}

	h.Hub.Broadcast(ws.EventGameCountdown, map[string]interface{}{
		"duration":        countdownDuration,
		"total_questions": len(ids),
	})

	h.startTimer(time.Duration(countdownDuration)*time.Second, func() {
		h.loadQuestion(0)
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "countdown",
		"duration":        countdownDuration,
		"total_questions": len(ids),
	})
}

func (h *Handler) loadQuestion(idx int) {
	ids := h.getQuestionIDs()
	timeLimit := h.getTimeLimit()
	if idx >= len(ids) {
		h.DB.SetGameState("finished", 0, idx, "", timeLimit)
		h.Hub.Broadcast(ws.EventGameFinished, map[string]interface{}{
			"status":          "finished",
			"total_questions": len(ids),
	h.mu.RLock()
	qIDs := h.QuestionIDs
	timeLimit := h.TimeLimit
	h.mu.RUnlock()

	if idx >= len(qIDs) {
		h.DB.SetGameState("finished", 0, idx, "", timeLimit)
		h.Hub.Broadcast(ws.EventGameFinished, map[string]interface{}{
			"status":          "finished",
			"total_questions": len(qIDs),
		})
		h.broadcastLeaderboard()
		return
	}

	qID := ids[idx]
	qID := qIDs[idx]
	now := time.Now().UTC().Format(time.RFC3339)
	h.DB.SetGameState("question", qID, idx, now, timeLimit)

	q, _ := h.DB.GetQuestion(qID)
	state := models.GameState{
		Status:          "question",
		CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
		QuestionIndex:   idx,
		TotalQuestions:  len(ids),
		TotalQuestions:  len(qIDs),
		TimeLeft:        timeLimit,
	}

	h.Hub.Broadcast(ws.EventNewQuestion, state)

	h.startTimer(time.Duration(timeLimit)*time.Second, func() {
		h.revealAnswer(qID, idx)
	})
}

func (h *Handler) revealAnswer(qID, idx int) {
	q, err := h.DB.GetQuestion(qID)
	if err != nil {
		return
	}

	h.DB.SetGameState("reveal", qID, idx, "", h.getTimeLimit())

	h.Hub.Broadcast(ws.EventTimeUp, map[string]interface{}{
		"question_id":    qID,
		"correct_answer": q.Answer,
		"question_index": idx,
	})

	players, _ := h.DB.ListPlayers()
	for _, p := range players {
		answered := h.DB.HasAnswered(p.ID, qID)
		result := map[string]interface{}{
			"correct":      false,
			"score_earned": 0,
			"total_score":  p.Score,
			"answered":     answered,
		}
		if answered {
			_, correct, _ := h.DB.GetPlayerAnswer(p.ID, qID)
			result["correct"] = correct
		}
		h.Hub.SendToPlayer(p.ID, ws.EventYourResult, result)
	}

	h.broadcastLeaderboard()
}

func (h *Handler) NextQuestion(w http.ResponseWriter, r *http.Request) {
	status, _, qIdx, _, _, _, err := h.DB.GetGameState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get game state")
		return
	}
	if status != "reveal" {
		writeError(w, http.StatusBadRequest, "can only advance after answer reveal")
		return
	}

	ids := h.getQuestionIDs()
	nextIdx := qIdx + 1
	h.loadQuestion(nextIdx)

	if nextIdx >= len(ids) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":          "finished",
			"question_index":  nextIdx,
			"total_questions": len(ids),
		})
	} else {
		q, _ := h.DB.GetQuestion(ids[nextIdx])
		timeLimit := h.getTimeLimit()
	h.mu.RLock()
	qIDs := h.QuestionIDs
	timeLimit := h.TimeLimit
	h.mu.RUnlock()

	if nextIdx >= len(qIDs) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":          "finished",
			"question_index":  nextIdx,
			"total_questions": len(qIDs),
		})
	} else {
		q, _ := h.DB.GetQuestion(qIDs[nextIdx])
		writeJSON(w, http.StatusOK, models.GameState{
			Status:          "question",
			CurrentQuestion: &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category},
			QuestionIndex:   nextIdx,
			TotalQuestions:  len(ids),
			TotalQuestions:  len(qIDs),
			TimeLeft:        timeLimit,
		})
	}
}

func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	status, qID, qIdx, startedAt, timeLimit, roomCode, err := h.DB.GetGameState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get game state")
		return
	}

	h.mu.RLock()
	totalQ := len(h.QuestionIDs)
	h.mu.RUnlock()

	state := models.GameState{
		Status:         status,
		QuestionIndex:  qIdx,
		TotalQuestions: len(h.getQuestionIDs()),
		TotalQuestions: totalQ,
		TimeLeft:       timeLimit,
		RoomCode:       roomCode,
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
			qq, _ := h.DB.GetQuestion(qID)
			state.CorrectAnswer = qq.Answer
		}
	}

	writeJSON(w, http.StatusOK, state)
}

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

	status, qID, _, startedAt, timeLimit, _, err := h.DB.GetGameState()
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
				score = int(1000.0 * float64(limitMs-elapsed) / float64(limitMs))
				if score < 10 {
					score = 10
				}
			} else {
				score = 10
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

	total, correctCount, wrongCount := h.DB.GetAnswerStats(qID)
	h.Hub.BroadcastToRole("admin", ws.EventPlayerAnswered, map[string]interface{}{
		"player_id":     req.PlayerID,
		"nickname":      player.Nickname,
		"total_answers": total,
		"correct_count": correctCount,
		"wrong_count":   wrongCount,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"recorded": true})
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
	h.clearQuestionIDs()
	h.mu.Lock()
	h.QuestionIDs = nil
	h.mu.Unlock()
	h.Hub.Broadcast(ws.EventGameReset, map[string]string{"status": "reset"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// --- Admin handlers ---

func (h *Handler) AdminAuth(w http.ResponseWriter, r *http.Request) {
	var req models.AdminAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PIN == "" {
		writeError(w, http.StatusBadRequest, "invalid credentials")
		return
	}
	ip := h.getClientIP(r)
	now := time.Now()
	if h.isAuthLocked(ip, now) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if req.PIN != h.AdminPIN {
		h.recordAuthFailure(ip, now)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	h.clearAuthFailures(ip)
	token := generateToken()
	h.setAdminToken(token)
	writeJSON(w, http.StatusOK, models.AdminAuthResponse{Token: token})
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
	h.setTimeLimit(req.TimeLimit)
	writeJSON(w, http.StatusOK, map[string]int{"time_limit": h.getTimeLimit()})
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
