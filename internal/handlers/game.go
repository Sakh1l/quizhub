package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/sakh1l/quizhub/internal/models"
	"github.com/sakh1l/quizhub/internal/ws"
)

func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	state, _ := h.DB.GetGameState()
	resp := models.GameState{
		Status:        state.Status,
		QuestionIndex: state.QuestionIndex,
		TimeLeft:      state.TimeLimit,
		RoomCode:      state.RoomCode,
	}
	if state.QuestionID > 0 {
		if q, err := h.DB.GetQuestion(state.QuestionID); err == nil {
			resp.CurrentQuestion = &models.QuestionOut{ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category}
			if state.Status == "reveal" {
				resp.CorrectAnswer = q.Answer
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID   string `json:"player_id"`
		QuestionID int    `json:"question_id"`
		Answer     *int   `json:"answer"`
		Option     *int   `json:"option"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "player_id and answer are required")
		return
	}
	selected := req.Answer
	if selected == nil {
		selected = req.Option
	}
	if selected == nil {
		writeError(w, http.StatusBadRequest, "player_id and answer are required")
		return
	}

	state, _ := h.DB.GetGameState()
	if !state.HasActiveQuestion() {
		writeError(w, http.StatusBadRequest, "no active question")
		return
	}
	currentQ, err := h.DB.GetQuestion(state.QuestionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no active question")
		return
	}
	if req.QuestionID != 0 && req.QuestionID != state.QuestionID {
		writeError(w, http.StatusBadRequest, "question_id does not match active question")
		return
	}
	if *selected < 0 || *selected >= len(currentQ.Options) {
		writeError(w, http.StatusBadRequest, "answer is out of range")
		return
	}

	score := 0
	if state.StartedAt != "" {
		if startedAt, err := time.Parse(time.RFC3339, state.StartedAt); err == nil {
			elapsed := time.Since(startedAt)
			if elapsed > time.Duration(state.TimeLimit)*time.Second {
				writeError(w, http.StatusBadRequest, "time is up")
				return
			}
			remaining := math.Max(0, float64(time.Duration(state.TimeLimit)*time.Second-elapsed))
			score = int(math.Round(1000 * remaining / float64(time.Duration(state.TimeLimit)*time.Second)))
			if score > 1000 {
				score = 1000
			}
		}
	}

	isCorrect := *selected == currentQ.Answer
	if !isCorrect {
		score = 0
	}
	recorded, err := h.DB.SubmitAnswer(req.PlayerID, currentQ.ID, *selected, isCorrect, score)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to submit answer")
		return
	}

	player, _ := h.DB.GetPlayer(req.PlayerID)
	resp := models.AnswerResponse{
		Correct:       isCorrect,
		CorrectAnswer: currentQ.Answer,
		ScoreEarned:   score,
		TotalScore:    player.Score,
	}
	if recorded {
		h.Hub.SendToPlayer(req.PlayerID, ws.EventYourResult, resp)
		h.broadcastAnswerStats(currentQ.ID)
		h.broadcastLeaderboard()
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	scores, err := h.DB.Leaderboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, scores)
}

func (h *Handler) SetTimer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TimeLimit int `json:"time_limit"`
		Duration  int `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid time_limit")
		return
	}
	limit := req.TimeLimit
	if limit == 0 {
		limit = req.Duration
	}
	if limit < 5 || limit > 120 {
		writeError(w, http.StatusBadRequest, "time_limit must be between 5 and 120")
		return
	}
	h.setTimeLimit(limit)
	if err := h.DB.SetTimeLimit(limit); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set timer")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"time_limit": h.getTimeLimit()})
}

func (h *Handler) StartGame(w http.ResponseWriter, r *http.Request) {
	state, err := h.DB.GetGameState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load game state")
		return
	}
	if state.QuizID == 0 {
		writeError(w, http.StatusBadRequest, "no quiz selected — create a room first")
		return
	}

	ids := h.getStagedQuestionIDs()
	if len(ids) == 0 {
		ids, err = h.DB.GetQuestionIDs(state.QuizID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load questions")
			return
		}
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no questions selected")
		return
	}

	if err := h.DB.StartGame(ids); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start game")
		return
	}

	h.Hub.Broadcast(ws.EventGameCountdown, map[string]interface{}{"duration": countdownDuration, "total_questions": len(ids)})
	h.startTimer(time.Duration(countdownDuration)*time.Second, h.beginQuestion)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "countdown", "total_questions": len(ids)})
}

func (h *Handler) beginQuestion() {
	if err := h.DB.BeginCurrentQuestion(); err != nil {
		return
	}
	h.startQuestion()
}

func (h *Handler) startQuestion() {
	state, err := h.DB.GetGameState()
	if err != nil || !state.HasActiveQuestion() {
		return
	}
	q, err := h.DB.GetQuestion(state.QuestionID)
	if err != nil {
		return
	}

	ids, _ := h.DB.ActiveQuestionIDs()
	h.Hub.Broadcast(ws.EventNewQuestion, map[string]interface{}{
		"status":          state.Status,
		"question_index":  state.QuestionIndex,
		"total_questions": len(ids),
		"current_question": models.QuestionOut{
			ID: q.ID, Text: q.Text, Options: q.Options, Category: q.Category,
		},
		"time_left": state.TimeLimit,
	})

	h.startTimer(time.Duration(state.TimeLimit)*time.Second, func() {
		h.revealCurrentQuestion()
	})
}

func (h *Handler) revealCurrentQuestion() {
	state, err := h.DB.GetGameState()
	if err != nil || !state.HasActiveQuestion() {
		return
	}
	q, err := h.DB.GetQuestion(state.QuestionID)
	if err != nil {
		return
	}
	state.Status = "reveal"
	state.StartedAt = ""
	_ = h.DB.SetGameState(state)
	total, correct, wrong := h.DB.GetAnswerStats(state.QuestionID)
	data := map[string]interface{}{
		"correct_answer": q.Answer,
		"total_answers":  total,
		"correct_count":  correct,
		"wrong_count":    wrong,
	}
	h.Hub.Broadcast(ws.EventTimeUp, data)
	h.broadcastLeaderboard()
}

func (h *Handler) NextQuestion(w http.ResponseWriter, r *http.Request) {
	h.stopTimer()
	hasNext, err := h.DB.NextQuestion()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to advance question")
		return
	}

	if !hasNext {
		h.broadcastLeaderboard()
		h.Hub.Broadcast(ws.EventGameFinished, nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "game finished"})
		return
	}

	h.startQuestion()
	writeJSON(w, http.StatusOK, map[string]string{"status": "next question"})
}

func (h *Handler) ResetGame(w http.ResponseWriter, r *http.Request) {
	h.stopTimer()
	if err := h.DB.ResetGame(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset game")
		return
	}
	h.clearStagedQuestionIDs()
	h.Hub.Broadcast(ws.EventGameReset, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "game reset"})
}

func (h *Handler) broadcastAnswerStats(questionID int) {
	total, correct, wrong := h.DB.GetAnswerStats(questionID)
	h.Hub.Broadcast(ws.EventPlayerAnswered, map[string]int{
		"question_id":   questionID,
		"total_answers": total,
		"correct_count": correct,
		"wrong_count":   wrong,
	})
}

func (h *Handler) broadcastLeaderboard() {
	leaderboard, err := h.DB.Leaderboard()
	if err == nil {
		h.Hub.Broadcast(ws.EventLeaderboard, leaderboard)
	}
}
