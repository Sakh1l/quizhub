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
	Status          string       `json:"status"` // "lobby", "countdown", "question", "reveal", "finished"
	CurrentQuestion *QuestionOut `json:"current_question,omitempty"`
	QuestionIndex   int          `json:"question_index"`
	TotalQuestions  int          `json:"total_questions"`
	TimeLeft        int          `json:"time_left"`
	CorrectAnswer   int          `json:"correct_answer,omitempty"`
	RoomCode        string       `json:"room_code,omitempty"`
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

// AdminAuthRequest is the body for POST /api/admin/auth.
type AdminAuthRequest struct {
	PIN string `json:"pin"`
}

// AdminAuthResponse is returned after successful admin auth.
type AdminAuthResponse struct {
	Token string `json:"token"`
}

// KickRequest is the body for POST /api/admin/kick.
type KickRequest struct {
	PlayerID string `json:"player_id"`
}

// TimerConfigRequest is the body for POST /api/admin/timer.
type TimerConfigRequest struct {
	TimeLimit int `json:"time_limit"`
}

// EditQuestionRequest is the body for PUT /api/admin/questions.
type EditQuestionRequest struct {
	ID       int      `json:"id"`
	Text     string   `json:"text"`
	Options  []string `json:"options"`
	Answer   int      `json:"answer"`
	Category string   `json:"category"`
}

// AnswerStats tracks answer distribution for admin view.
type AnswerStats struct {
	QuestionID   int `json:"question_id"`
	TotalAnswers int `json:"total_answers"`
	CorrectCount int `json:"correct_count"`
	WrongCount   int `json:"wrong_count"`
}

// GameConfig holds configurable game settings.
type GameConfig struct {
	TimeLimit  int      `json:"time_limit"`
	Categories []string `json:"categories,omitempty"`
}
