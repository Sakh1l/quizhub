package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

const authRateWindow = 10 * time.Minute
const authMaxFailures = 5

type authAttempt struct {
	failures     int
	windowStart  time.Time
	lockoutUntil time.Time
}

func (h *Handler) setAdminToken(token string) {
	h.stateMu.Lock()
	clear(h.adminTokens)
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
