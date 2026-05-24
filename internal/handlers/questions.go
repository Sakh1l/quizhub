package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sakh1l/quizhub/internal/models"
)

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
	if q.Answer < 0 || q.Answer >= len(q.Options) {
		writeError(w, http.StatusBadRequest, "answer must reference an option")
		return
	}
	id, err := h.DB.AddQuestion(q.Text, q.Options, q.Answer, q.Category)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add question")
		return
	}
	q.ID = id
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
	if err := h.DB.DeleteQuestion(req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete question")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
