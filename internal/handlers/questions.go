package handlers

import (
	"encoding/json"
	"net/http"
)

// UpdateSelectedQuestions stages a subset of the active quiz's questions
// (and their order) to use for the next game, overriding the full-quiz
// default that StartGame otherwise falls back to.
func (h *Handler) UpdateSelectedQuestions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	h.setStagedQuestionIDs(req.IDs)
	writeJSON(w, http.StatusOK, map[string]string{"status": "questions updated"})
}

func (h *Handler) DeleteQuestion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := h.DB.DeleteQuestion(req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete question")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
