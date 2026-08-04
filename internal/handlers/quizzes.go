package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/sakh1l/quizhub/internal/models"
)

const maxQuizTitleLen = 100

// QuizzesRouter lists (GET) or creates (POST) quizzes in the admin's library.
func (h *Handler) QuizzesRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.adminOnly(h.ListQuizzes)(w, r)
	case http.MethodPost:
		h.adminOnly(h.CreateQuiz)(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) ListQuizzes(w http.ResponseWriter, r *http.Request) {
	quizzes, err := h.DB.ListQuizzes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list quizzes")
		return
	}
	writeJSON(w, http.StatusOK, quizzes)
}

func (h *Handler) CreateQuiz(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		req.Title = "Untitled Quiz"
	}
	if len(req.Title) > maxQuizTitleLen {
		writeError(w, http.StatusBadRequest, "title must be 100 characters or fewer")
		return
	}
	id, err := h.DB.CreateQuiz(req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create quiz")
		return
	}
	quiz, err := h.DB.GetQuiz(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load created quiz")
		return
	}
	writeJSON(w, http.StatusCreated, quiz)
}

func (h *Handler) RenameQuiz(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || len(req.Title) > maxQuizTitleLen {
		writeError(w, http.StatusBadRequest, "title must be 1-100 characters")
		return
	}
	if err := h.DB.RenameQuiz(req.ID, req.Title); err != nil {
		writeError(w, http.StatusNotFound, "quiz not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

func (h *Handler) DuplicateQuiz(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	newID, err := h.DB.DuplicateQuiz(req.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "quiz not found")
		return
	}
	quiz, err := h.DB.GetQuiz(newID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load duplicated quiz")
		return
	}
	writeJSON(w, http.StatusCreated, quiz)
}

func (h *Handler) DeleteQuiz(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.DB.DeleteQuiz(req.ID); err != nil {
		writeError(w, http.StatusNotFound, "quiz not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// QuizQuestionsRouter lists (GET, ?quiz_id=) or adds (POST, body includes
// quiz_id) questions scoped to a single saved quiz.
func (h *Handler) QuizQuestionsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.adminOnly(h.ListQuizQuestions)(w, r)
	case http.MethodPost:
		h.adminOnly(h.AddQuizQuestion)(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) ListQuizQuestions(w http.ResponseWriter, r *http.Request) {
	quizID, err := strconv.Atoi(r.URL.Query().Get("quiz_id"))
	if err != nil || quizID <= 0 {
		writeError(w, http.StatusBadRequest, "quiz_id is required")
		return
	}
	questions, err := h.DB.ListQuestionsByQuiz(quizID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list questions")
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

func (h *Handler) AddQuizQuestion(w http.ResponseWriter, r *http.Request) {
	var q models.Question
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q.QuizID <= 0 || q.Text == "" || len(q.Options) < 2 {
		writeError(w, http.StatusBadRequest, "quiz_id, text, and at least 2 options are required")
		return
	}
	if q.Answer < 0 || q.Answer >= len(q.Options) {
		writeError(w, http.StatusBadRequest, "answer must reference an option")
		return
	}
	if _, err := h.DB.GetQuiz(q.QuizID); err != nil {
		writeError(w, http.StatusNotFound, "quiz not found")
		return
	}
	id, err := h.DB.AddQuestion(q.QuizID, q.Text, q.Options, q.Answer, q.Category)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add question")
		return
	}
	q.ID = id
	writeJSON(w, http.StatusCreated, q)
}
