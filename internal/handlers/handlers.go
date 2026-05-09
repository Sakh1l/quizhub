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

	if code == "" || !strings.EqualFold(code, activeCode) {
		writeError(w, http.StatusNotFound, "invalid room code")
		return
	}

	status, _, _, _, _, _, _ := h.DB.GetGameState()
	writeJSON(w, http.StatusOK, map[string]string{
		"room_code": activeCode,
		"status":    status,
	})
}

func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	status, currentIdx, currentQ, startTime, timeLimit, total, questionIDs := h.DB.GetGameState()
	resp := models.GameStateResponse{
		Status:         status,
		CurrentIndex:   currentIdx,
		TotalQuestions: total,
		StartTime:      startTime,
		TimeLimit:      timeLimit,
	}
	if currentQ != nil {
		resp.CurrentQuestion = &models.Question{
			ID:      currentQ.ID,
			Text:    currentQ.Text,
			Options: currentQ.Options,
		}
	}
	if status == "finished" {
		resp.QuestionIDs = questionIDs
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"player_id"`
		Option   int    `json:"option"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "player_id and option are required")
		return
	}

	status, _, currentQ, startTime, timeLimit, _, _ := h.DB.GetGameState()
	if status != "question" || currentQ == nil {
		writeError(w, http.StatusBadRequest, "no active question")
		return
	}

	// Check time limit
	if time.Since(startTime) > time.Duration(timeLimit)*time.Second {
		writeError(w, http.StatusBadRequest, "time is up")
		return
	}

	isCorrect := req.Option == currentQ.CorrectOption
	err := h.DB.SubmitAnswer(req.PlayerID, currentQ.ID, isCorrect)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to submit answer")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"correct": isCorrect})
}

func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	scores, err := h.DB.GetLeaderboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, scores)
}

func (h *Handler) AdminAuth(w http.ResponseWriter, r *http.Request) {
	ip := h.getClientIP(r)
	now := time.Now()

	if h.isAuthLocked(ip, now) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, please try again later")
		return
	}

	var req struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PIN == "" {
		writeError(w, http.StatusBadRequest, "pin is required")
		return
	}

	if req.PIN != h.AdminPIN {
		h.recordAuthFailure(ip, now)
		writeError(w, http.StatusUnauthorized, "invalid pin")
		return
	}

	h.clearAuthFailures(ip)
	token := generateToken()
	h.setAdminToken(token)

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	code := generateRoomCode()
	err := h.DB.CreateRoom(code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create room")
		return
	}

	h.clearQuestionIDs()
	h.Hub.Broadcast(ws.EventRoomCreated, map[string]string{"room_code": code})
	writeJSON(w, http.StatusOK, map[string]string{"room_code": code})
}

func (h *Handler) SetTimer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Duration int `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Duration <= 0 {
		writeError(w, http.StatusBadRequest, "invalid duration")
		return
	}
	h.setTimeLimit(req.Duration)
	writeJSON(w, http.StatusOK, map[string]string{"status": "timer set"})
}

func (h *Handler) StartGame(w http.ResponseWriter, r *http.Request) {
	ids := h.getQuestionIDs()
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no questions selected")
		return
	}

	err := h.DB.StartGame(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start game")
		return
	}

	h.Hub.Broadcast(ws.EventGameStarted, nil)
	h.startQuestion()
	writeJSON(w, http.StatusOK, map[string]string{"status": "game started"})
}

func (h *Handler) startQuestion() {
	status, currentIdx, currentQ, _, _, total, _ := h.DB.GetGameState()
	if status != "question" || currentQ == nil {
		return
	}

	h.Hub.Broadcast(ws.EventNewQuestion, models.GameStateResponse{
		Status:          status,
		CurrentIndex:    currentIdx,
		TotalQuestions:  total,
		CurrentQuestion: currentQ,
		TimeLimit:       h.getTimeLimit(),
	})

	// Set timer for auto-next
	h.startTimer(time.Duration(h.getTimeLimit())*time.Second, func() {
		h.Hub.Broadcast(ws.EventTimeUp, nil)
	})
}

func (h *Handler) NextQuestion(w http.ResponseWriter, r *http.Request) {
	h.stopTimer()
	hasNext, err := h.DB.NextQuestion()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to advance question")
		return
	}

	if !hasNext {
		h.Hub.Broadcast(ws.EventGameFinished, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "game finished"})
		return
	}

	h.startQuestion()
	writeJSON(w, http.StatusOK, map[string]string{"status": "next question"})
}

func (h *Handler) ResetGame(w http.ResponseWriter, r *http.Request) {
	h.stopTimer()
	err := h.DB.ResetGame()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset game")
		return
	}
	h.clearQuestionIDs()
	h.Hub.Broadcast(ws.EventGameReset, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "game reset"})
}

func (h *Handler) QuestionsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListQuestions(w, r)
	case http.MethodPost:
		h.adminOnly(h.UpdateSelectedQuestions)(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) ListQuestions(w http.ResponseWriter, r *http.Request) {
	questions, err := h.DB.ListQuestions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list questions")
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

func (h *Handler) UpdateSelectedQuestions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	h.setQuestionIDs(req.IDs)
	writeJSON(w, http.StatusOK, map[string]string{"status": "questions updated"})
}

func (h *Handler) AddQuestion(w http.ResponseWriter, r *http.Request) {
	var q models.Question
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q.Text == "" || len(q.Options) < 2 {
		writeError(w, http.StatusBadRequest, "invalid question data")
		return
	}
	err := h.DB.AddQuestion(&q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add question")
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

func (h *Handler) DeleteQuestion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	err := h.DB.DeleteQuestion(req.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete question")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) broadcastPlayers() {
	players, _ := h.DB.ListPlayers()
	h.Hub.Broadcast(ws.EventPlayerList, players)
}
