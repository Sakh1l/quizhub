package handlers

import (
	"net/http"
	"strings"

	"github.com/skip2/go-qrcode"
)

func (h *Handler) JoinQRCode(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	activeCode := h.DB.GetRoomCode()
	if code == "" || activeCode == "" || !strings.EqualFold(code, activeCode) {
		writeError(w, http.StatusNotFound, "invalid room code")
		return
	}

	png, err := qrcode.Encode(publicJoinURL(activeCode), qrcode.Medium, 360)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate QR code")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}
