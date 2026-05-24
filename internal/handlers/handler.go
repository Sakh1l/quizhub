package handlers

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/ws"
)

const version = "1.0.0"
const countdownDuration = 10

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

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.Health)
	mux.HandleFunc("/api/join", methodOnly(http.MethodPost, h.Join))
	mux.HandleFunc("/api/players", methodOnly(http.MethodGet, h.Players))
	mux.HandleFunc("/api/game/state", methodOnly(http.MethodGet, h.State))
	mux.HandleFunc("/api/answer", methodOnly(http.MethodPost, h.Answer))
	mux.HandleFunc("/api/leaderboard", methodOnly(http.MethodGet, h.Leaderboard))
	mux.HandleFunc("/api/room/info", methodOnly(http.MethodGet, h.RoomInfo))

	mux.HandleFunc("/api/game/start", methodOnly(http.MethodPost, h.adminOnly(h.StartGame)))
	mux.HandleFunc("/api/game/next", methodOnly(http.MethodPost, h.adminOnly(h.NextQuestion)))
	mux.HandleFunc("/api/game/reset", methodOnly(http.MethodPost, h.adminOnly(h.ResetGame)))

	mux.HandleFunc("/api/admin/auth", methodOnly(http.MethodPost, h.AdminAuth))
	mux.HandleFunc("/api/admin/timer", methodOnly(http.MethodPost, h.adminOnly(h.SetTimer)))
	mux.HandleFunc("/api/room/create", methodOnly(http.MethodPost, h.adminOnly(h.CreateRoom)))
	mux.HandleFunc("/api/questions/add", methodOnly(http.MethodPost, h.adminOnly(h.AddQuestion)))
	mux.HandleFunc("/api/questions/delete", methodOnly(http.MethodPost, h.adminOnly(h.DeleteQuestion)))
	mux.HandleFunc("/api/questions", h.QuestionsRouter)

	mux.HandleFunc("/api/ws", h.HandleWS)
}
