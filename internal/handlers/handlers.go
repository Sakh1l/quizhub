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

func (h *Handler) appendQuestionID(id int) {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	for _, existing := range h.questionIDs {
		if existing == id {
			return
		}
	}
	h.questionIDs = append(h.questionIDs, id)
}

func (h *Handler) removeQuestionID(id int) {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	filtered := h.questionIDs[:0]
	for _, existing := range h.questionIDs {
		if existing != id {
			filtered = append(filtered, existing)
		}
	}
	h.questionIDs = filtered
}

func (h *Handler) ensureQuestionIDs() ([]int, error) {
	ids := h.getQuestionIDs()
	if len(ids) > 0 {
		return ids, nil
	}
	ids, err := h.DB.GetQuestionIDs()
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		h.setQuestionIDs(ids)
	}
	return ids, nil
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

func (h *Handler) roomLink(r *http.Request, code string) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Host
	return scheme + "://" + host + "/?room=" + code
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
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"room_code": activeCode,
		"status":    status,
		"joinable":  status == "lobby",
	})
}

func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	status, currentQID, currentIdx, startedAt, timeLimit, roomCode, _ := h.DB.GetGameState()
	resp := models.GameState{
		Status:        status,
		QuestionIndex: currentIdx,
		TimeLeft:      timeLimit,
		RoomCode:      roomCode,
	}
	if ids := h.getQuestionIDs(); len(ids) > 0 {
		resp.TotalQuestions = len(ids)
	} else {
		resp.TotalQuestions = h.DB.QuestionCount()
	}
	if startedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, startedAt); err == nil {
			var duration time.Duration
			if status == "countdown" {
				duration = countdownDuration * time.Second
			} else if status == "question" {
				duration = time.Duration(timeLimit) * time.Second
			}
			if duration > 0 {
				remaining := int(time.Until(parsed.Add(duration)).Seconds())
				if remaining < 0 {
					remaining = 0
				}
				resp.TimeLeft = remaining
			}
		}
	}
	if currentQID > 0 {
		if q, err := h.DB.GetQuestion(currentQID); err == nil {
			resp.CurrentQuestion = &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category}
			if status == "reveal" {
				answer := q.Answer
				resp.CorrectAnswer = &answer
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID   string `json:"player_id"`
		QuestionID int    `json:"question_id"`
		Answer     int    `json:"answer"`
		Option     int    `json:"option"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "player_id and answer are required")
		return
	}

	status, currentQID, _, startTime, timeLimit, _, _ := h.DB.GetGameState()
	if status != "question" || currentQID == 0 {
		writeError(w, http.StatusBadRequest, "no active question")
		return
	}
	if req.QuestionID != 0 && req.QuestionID != currentQID {
		writeError(w, http.StatusBadRequest, "question is no longer active")
		return
	}
	currentQ, err := h.DB.GetQuestion(currentQID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no active question")
		return
	}
	selected := req.Answer
	if req.Answer == 0 && req.Option != 0 {
		selected = req.Option
	}
	if selected < 0 || selected >= len(currentQ.Options) {
		writeError(w, http.StatusBadRequest, "answer option is invalid")
		return
	}
	if _, err := h.DB.GetPlayer(req.PlayerID); err != nil {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}
	if h.DB.HasAnswered(req.PlayerID, currentQ.ID) {
		writeError(w, http.StatusConflict, "answer already submitted")
		return
	}

	// Check time limit
	elapsed := time.Duration(0)
	if startTime != "" {
		if startedAt, err := time.Parse(time.RFC3339, startTime); err == nil {
			elapsed = time.Since(startedAt)
			if elapsed > time.Duration(timeLimit)*time.Second {
				writeError(w, http.StatusBadRequest, "time is up")
				return
			}
		}
	}

	isCorrect := selected == currentQ.Answer
	scoreEarned := 0
	if isCorrect {
		totalMS := (time.Duration(timeLimit) * time.Second).Milliseconds()
		remainingMS := totalMS - elapsed.Milliseconds()
		if remainingMS < 0 {
			remainingMS = 0
		}
		scoreEarned = int((remainingMS * 1000) / totalMS)
		if scoreEarned < 10 {
			scoreEarned = 10
		}
	}
	recorded, err := h.DB.RecordAnswer(req.PlayerID, currentQ.ID, selected, isCorrect, scoreEarned)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to submit answer")
		return
	}
	if !recorded {
		writeError(w, http.StatusConflict, "answer already submitted")
		return
	}
	if scoreEarned > 0 {
		if err := h.DB.UpdatePlayerScore(req.PlayerID, scoreEarned); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update score")
			return
		}
	}
	player, _ := h.DB.GetPlayer(req.PlayerID)
	h.Hub.SendToPlayer(req.PlayerID, ws.EventYourResult, models.AnswerResponse{
		Correct:       isCorrect,
		CorrectAnswer: currentQ.Answer,
		ScoreEarned:   scoreEarned,
		TotalScore:    player.Score,
	})
	h.broadcastAnswerStats(currentQ.ID)
	h.broadcastLeaderboard()

	writeJSON(w, http.StatusOK, map[string]bool{"recorded": true})
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
	ids, err := h.ensureQuestionIDs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load questions")
		return
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "add at least one question before creating a room")
		return
	}

	code := generateRoomCode()
	err = h.DB.CreateRoom(code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create room")
		return
	}

	h.Hub.Broadcast(ws.EventRoomCreated, map[string]string{"room_code": code})
	writeJSON(w, http.StatusCreated, map[string]string{"room_code": code, "link": h.roomLink(r, code)})
}

func (h *Handler) SetTimer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Duration  int `json:"duration"`
		TimeLimit int `json:"time_limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid duration")
		return
	}
	duration := req.TimeLimit
	if duration == 0 {
		duration = req.Duration
	}
	if duration < 5 || duration > 120 {
		writeError(w, http.StatusBadRequest, "time_limit must be between 5 and 120 seconds")
		return
	}
	h.setTimeLimit(duration)
	if err := h.DB.SetTimeLimit(duration); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set timer")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"time_limit": h.getTimeLimit()})
}

func (h *Handler) StartGame(w http.ResponseWriter, r *http.Request) {
	ids, err := h.ensureQuestionIDs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load questions")
		return
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no questions selected")
		return
	}
	status, _, _, _, _, _, _ := h.DB.GetGameState()
	if status != "lobby" {
		writeError(w, http.StatusBadRequest, "game already in progress")
		return
	}
	timeLimit := h.getTimeLimit()
	if timeLimit < 5 || timeLimit > 120 {
		timeLimit = 15
		h.setTimeLimit(timeLimit)
	}
	if err := h.DB.SetGameState("countdown", 0, 0, time.Now().UTC().Format(time.RFC3339), timeLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start game")
		return
	}

	h.Hub.Broadcast(ws.EventGameCountdown, map[string]interface{}{
		"duration":        countdownDuration,
		"total_questions": len(ids),
	})
	h.startTimer(countdownDuration*time.Second, func() { h.startQuestionAt(0) })

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "countdown", "duration": countdownDuration, "total_questions": len(ids)})
}

func (h *Handler) startQuestionAt(index int) {
	ids, err := h.ensureQuestionIDs()
	if err != nil || index < 0 || index >= len(ids) {
		return
	}
	timeLimit := h.getTimeLimit()
	if timeLimit < 5 || timeLimit > 120 {
		timeLimit = 15
	}
	currentQID := ids[index]
	if err := h.DB.SetGameState("question", currentQID, index, time.Now().UTC().Format(time.RFC3339), timeLimit); err != nil {
		return
	}
	q, err := h.DB.GetQuestion(currentQID)
	if err != nil {
		return
	}

	h.Hub.Broadcast(ws.EventNewQuestion, map[string]interface{}{
		"status":          "question",
		"question_index":  index,
		"total_questions": len(ids),
		"current_question": models.QuestionOut{
			ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category,
		},
		"time_left": timeLimit,
	})

	h.startTimer(time.Duration(timeLimit)*time.Second, h.revealCurrentQuestion)
}

func (h *Handler) revealCurrentQuestion() {
	status, currentQID, currentIdx, _, timeLimit, _, _ := h.DB.GetGameState()
	if status != "question" || currentQID == 0 {
		return
	}
	q, err := h.DB.GetQuestion(currentQID)
	if err != nil {
		return
	}
	if err := h.DB.SetGameState("reveal", currentQID, currentIdx, "", timeLimit); err != nil {
		return
	}
	h.Hub.Broadcast(ws.EventTimeUp, map[string]interface{}{"correct_answer": q.Answer})
	players, _ := h.DB.ListPlayers()
	for _, player := range players {
		_, correct, scoreEarned, totalScore, found, err := h.DB.GetPlayerAnswerResult(player.ID, currentQID)
		if err != nil || !found {
			continue
		}
		h.Hub.SendToPlayer(player.ID, ws.EventYourResult, models.AnswerResponse{
			Correct:       correct,
			CorrectAnswer: q.Answer,
			ScoreEarned:   scoreEarned,
			TotalScore:    totalScore,
		})
	}
	h.broadcastAnswerStats(currentQID)
	h.broadcastLeaderboard()
}

func (h *Handler) NextQuestion(w http.ResponseWriter, r *http.Request) {
	h.stopTimer()
	status, _, currentIdx, _, timeLimit, _, _ := h.DB.GetGameState()
	if status != "reveal" {
		writeError(w, http.StatusBadRequest, "question is not ready to advance")
		return
	}
	ids, err := h.ensureQuestionIDs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load questions")
		return
	}

	nextIdx := currentIdx + 1
	if nextIdx >= len(ids) {
		_ = h.DB.SetGameState("finished", 0, currentIdx, "", timeLimit)
		h.broadcastLeaderboard()
		h.Hub.Broadcast(ws.EventGameFinished, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "finished"})
		return
	}

	h.startQuestionAt(nextIdx)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "question", "question_index": nextIdx})
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
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil || strings.TrimSpace(q.Text) == "" || len(q.Options) < 2 {
		writeError(w, http.StatusBadRequest, "invalid question data")
		return
	}
	if q.Answer < 0 || q.Answer >= len(q.Options) {
		writeError(w, http.StatusBadRequest, "correct answer is invalid")
		return
	}
	id, err := h.DB.AddQuestion(q.Text, q.Options, q.Answer, q.Category)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add question")
		return
	}
	q.ID = id
	h.appendQuestionID(id)
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
	h.removeQuestionID(req.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) broadcastAnswerStats(questionID int) {
	total, correct, wrong := h.DB.GetAnswerStats(questionID)
	h.Hub.BroadcastToRole("admin", ws.EventPlayerAnswered, models.AnswerStats{
		QuestionID:   questionID,
		TotalAnswers: total,
		CorrectCount: correct,
		WrongCount:   wrong,
	})
}

func (h *Handler) broadcastLeaderboard() {
	leaderboard, err := h.DB.GetLeaderboard()
	if err != nil {
		return
	}
	h.Hub.Broadcast(ws.EventLeaderboard, leaderboard)
}

func (h *Handler) broadcastPlayers() {
	players, _ := h.DB.ListPlayers()
	h.Hub.Broadcast(ws.EventPlayersUpdate, players)
}
