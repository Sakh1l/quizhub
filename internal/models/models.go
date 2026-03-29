package models

import "time"

// Player represents a quiz participant.
type Player struct {
	ID        string    `json:"player_id"`
	Nickname  string    `json:"nickname"`
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// Question represents a single quiz question.
type Question struct {
	ID       int      `json:"id"`
	Text     string   `json:"text"`
	Options  []string `json:"options"`
	Answer   int      `json:"answer"`
	Category string   `json:"category"`
}

// QuestionOut is the question sent to clients (no answer field).
type QuestionOut struct {
	ID       int      `json:"id"`
	Text     string   `json:"text"`
	Options  []string `json:"options"`
	Category string   `json:"category"`
}

// JoinRequest is the body for POST /api/join.
type JoinRequest struct {
	Nickname string `json:"nickname"`
}

// AnswerRequest is the body for POST /api/answer.
type AnswerRequest struct {
	PlayerID   string `json:"player_id"`
	QuestionID int    `json:"question_id"`
	Answer     int    `json:"answer"`
}

// AnswerResponse is returned after submitting an answer.
type AnswerResponse struct {
	Correct       bool `json:"correct"`
	CorrectAnswer int  `json:"correct_answer"`
	ScoreEarned   int  `json:"score_earned"`
	TotalScore    int  `json:"total_score"`
}

// GameState represents the current state of the quiz game.
type GameState struct {
	Status          string       `json:"status"` // "lobby", "question", "reveal", "finished"
	CurrentQuestion *QuestionOut `json:"current_question,omitempty"`
	QuestionIndex   int          `json:"question_index"`
	TotalQuestions  int          `json:"total_questions"`
	TimeLeft        int          `json:"time_left"`
}

// LeaderboardEntry is a player's position on the leaderboard.
type LeaderboardEntry struct {
	Rank     int    `json:"rank"`
	PlayerID string `json:"player_id"`
	Nickname string `json:"nickname"`
	Score    int    `json:"score"`
}

// APIError is a standard JSON error response.
type APIError struct {
	Error string `json:"error"`
}

// HealthResponse is returned by the health endpoint.
type HealthResponse struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	PlayerCount int    `json:"player_count"`
}
