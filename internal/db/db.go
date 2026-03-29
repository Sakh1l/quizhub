package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sakh1l/quizhub/internal/models"
	_ "modernc.org/sqlite"
)

// DB wraps the sql.DB connection and all data operations.
type DB struct {
	conn *sql.DB
}

// New opens (or creates) a SQLite database and runs migrations.
func New(dsn string) (*DB, error) {
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite performance pragmas
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := d.seed(); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}

	return d, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS players (
			id TEXT PRIMARY KEY,
			nickname TEXT NOT NULL,
			score INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS questions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			text TEXT NOT NULL,
			options TEXT NOT NULL,
			answer INTEGER NOT NULL,
			category TEXT NOT NULL DEFAULT 'general'
		)`,
		`CREATE TABLE IF NOT EXISTS answers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			player_id TEXT NOT NULL,
			question_id INTEGER NOT NULL,
			selected INTEGER NOT NULL,
			correct INTEGER NOT NULL DEFAULT 0,
			score_earned INTEGER NOT NULL DEFAULT 0,
			answered_at TEXT NOT NULL,
			FOREIGN KEY (player_id) REFERENCES players(id),
			FOREIGN KEY (question_id) REFERENCES questions(id),
			UNIQUE(player_id, question_id)
		)`,
		`CREATE TABLE IF NOT EXISTS game_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			status TEXT NOT NULL DEFAULT 'lobby',
			current_question_id INTEGER,
			question_index INTEGER NOT NULL DEFAULT 0,
			question_started_at TEXT,
			time_limit INTEGER NOT NULL DEFAULT 15
		)`,
	}

	for _, s := range stmts {
		if _, err := d.conn.Exec(s); err != nil {
			return fmt.Errorf("exec %s: %w", s[:40], err)
		}
	}

	// Ensure game_state row exists
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO game_state (id, status) VALUES (1, 'lobby')`)
	return err
}

func (d *DB) seed() error {
	var count int
	d.conn.QueryRow("SELECT COUNT(*) FROM questions").Scan(&count)
	if count > 0 {
		return nil // already seeded
	}

	type q struct {
		text     string
		options  []string
		answer   int
		category string
	}

	questions := []q{
		{"What is the capital of France?", []string{"London", "Paris", "Berlin", "Madrid"}, 1, "geography"},
		{"Which planet is known as the Red Planet?", []string{"Venus", "Jupiter", "Mars", "Saturn"}, 2, "science"},
		{"What is 12 x 12?", []string{"120", "132", "144", "156"}, 2, "math"},
		{"Who wrote 'Romeo and Juliet'?", []string{"Dickens", "Shakespeare", "Austen", "Twain"}, 1, "literature"},
		{"What is the largest ocean on Earth?", []string{"Atlantic", "Indian", "Arctic", "Pacific"}, 3, "geography"},
		{"In what year did the Titanic sink?", []string{"1905", "1912", "1920", "1898"}, 1, "history"},
		{"What is the chemical symbol for gold?", []string{"Go", "Gd", "Au", "Ag"}, 2, "science"},
		{"How many sides does a hexagon have?", []string{"5", "6", "7", "8"}, 1, "math"},
		{"Which language has the most native speakers?", []string{"English", "Spanish", "Hindi", "Mandarin"}, 3, "geography"},
		{"What is the square root of 256?", []string{"14", "15", "16", "18"}, 2, "math"},
		{"Who painted the Mona Lisa?", []string{"Michelangelo", "Da Vinci", "Raphael", "Rembrandt"}, 1, "history"},
		{"What is the boiling point of water in Celsius?", []string{"90", "95", "100", "105"}, 2, "science"},
		{"Which continent is the Sahara Desert on?", []string{"Asia", "Africa", "Australia", "South America"}, 1, "geography"},
		{"What does HTTP stand for?", []string{"HyperText Transfer Protocol", "High Tech Transfer Protocol", "HyperText Transmission Process", "High Transfer Text Protocol"}, 0, "technology"},
		{"How many bones are in the adult human body?", []string{"186", "196", "206", "216"}, 2, "science"},
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO questions (text, options, answer, category) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, question := range questions {
		opts, _ := json.Marshal(question.options)
		if _, err := stmt.Exec(question.text, string(opts), question.answer, question.category); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// --- Player operations ---

// CreatePlayer inserts a new player and returns it.
func (d *DB) CreatePlayer(nickname string) (models.Player, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := d.conn.Exec(
		"INSERT INTO players (id, nickname, score, created_at) VALUES (?, ?, 0, ?)",
		id, nickname, now,
	)
	if err != nil {
		return models.Player{}, fmt.Errorf("insert player: %w", err)
	}

	return models.Player{
		ID:        id,
		Nickname:  nickname,
		Score:     0,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// GetPlayer returns a player by ID.
func (d *DB) GetPlayer(id string) (models.Player, error) {
	var p models.Player
	var createdAt string
	err := d.conn.QueryRow(
		"SELECT id, nickname, score, created_at FROM players WHERE id = ?", id,
	).Scan(&p.ID, &p.Nickname, &p.Score, &createdAt)
	if err != nil {
		return p, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return p, nil
}

// ListPlayers returns all players.
func (d *DB) ListPlayers() ([]models.Player, error) {
	rows, err := d.conn.Query("SELECT id, nickname, score, created_at FROM players ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		var createdAt string
		if err := rows.Scan(&p.ID, &p.Nickname, &p.Score, &createdAt); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		players = append(players, p)
	}
	if players == nil {
		players = []models.Player{}
	}
	return players, rows.Err()
}

// PlayerCount returns the number of players.
func (d *DB) PlayerCount() int {
	var c int
	d.conn.QueryRow("SELECT COUNT(*) FROM players").Scan(&c)
	return c
}

// Leaderboard returns players sorted by score descending.
func (d *DB) Leaderboard() ([]models.LeaderboardEntry, error) {
	rows, err := d.conn.Query("SELECT id, nickname, score FROM players ORDER BY score DESC, nickname ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LeaderboardEntry
	rank := 1
	for rows.Next() {
		var e models.LeaderboardEntry
		if err := rows.Scan(&e.PlayerID, &e.Nickname, &e.Score); err != nil {
			return nil, err
		}
		e.Rank = rank
		rank++
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []models.LeaderboardEntry{}
	}
	return entries, rows.Err()
}

// UpdatePlayerScore adds delta to a player's score.
func (d *DB) UpdatePlayerScore(playerID string, delta int) error {
	_, err := d.conn.Exec("UPDATE players SET score = score + ? WHERE id = ?", delta, playerID)
	return err
}

// --- Question operations ---

// GetQuestion returns a question by ID.
func (d *DB) GetQuestion(id int) (models.Question, error) {
	var q models.Question
	var optsJSON string
	err := d.conn.QueryRow(
		"SELECT id, text, options, answer, category FROM questions WHERE id = ?", id,
	).Scan(&q.ID, &q.Text, &optsJSON, &q.Answer, &q.Category)
	if err != nil {
		return q, err
	}
	json.Unmarshal([]byte(optsJSON), &q.Options)
	return q, nil
}

// QuestionCount returns total questions available.
func (d *DB) QuestionCount() int {
	var c int
	d.conn.QueryRow("SELECT COUNT(*) FROM questions").Scan(&c)
	return c
}

// GetQuestionIDs returns all question IDs in random order.
func (d *DB) GetQuestionIDs() ([]int, error) {
	rows, err := d.conn.Query("SELECT id FROM questions ORDER BY RANDOM()")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- Answer operations ---

// RecordAnswer stores a player's answer. Returns true if this is a new answer (not duplicate).
func (d *DB) RecordAnswer(playerID string, questionID, selected int, correct bool, scoreEarned int) (bool, error) {
	correctInt := 0
	if correct {
		correctInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.conn.Exec(
		`INSERT OR IGNORE INTO answers (player_id, question_id, selected, correct, score_earned, answered_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		playerID, questionID, selected, correctInt, scoreEarned, now,
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// HasAnswered checks if a player already answered a question.
func (d *DB) HasAnswered(playerID string, questionID int) bool {
	var count int
	d.conn.QueryRow(
		"SELECT COUNT(*) FROM answers WHERE player_id = ? AND question_id = ?",
		playerID, questionID,
	).Scan(&count)
	return count > 0
}

// --- Game state operations ---

// GetGameState returns the current game state.
func (d *DB) GetGameState() (status string, questionID int, questionIndex int, startedAt string, timeLimit int, err error) {
	err = d.conn.QueryRow(
		"SELECT status, COALESCE(current_question_id, 0), question_index, COALESCE(question_started_at, ''), time_limit FROM game_state WHERE id = 1",
	).Scan(&status, &questionID, &questionIndex, &startedAt, &timeLimit)
	return
}

// SetGameState updates the game state.
func (d *DB) SetGameState(status string, questionID, questionIndex int, startedAt string, timeLimit int) error {
	var qIDVal interface{} = questionID
	if questionID == 0 {
		qIDVal = nil
	}
	var startedVal interface{} = startedAt
	if startedAt == "" {
		startedVal = nil
	}
	_, err := d.conn.Exec(
		"UPDATE game_state SET status = ?, current_question_id = ?, question_index = ?, question_started_at = ?, time_limit = ? WHERE id = 1",
		status, qIDVal, questionIndex, startedVal, timeLimit,
	)
	return err
}

// ResetGame clears all players, answers, and resets game state to lobby.
func (d *DB) ResetGame() error {
	stmts := []string{
		"DELETE FROM answers",
		"DELETE FROM players",
		"UPDATE game_state SET status = 'lobby', current_question_id = NULL, question_index = 0, question_started_at = NULL WHERE id = 1",
	}
	for _, s := range stmts {
		if _, err := d.conn.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// AddQuestion inserts a custom question.
func (d *DB) AddQuestion(text string, options []string, answer int, category string) (int, error) {
	optsJSON, _ := json.Marshal(options)
	category = strings.TrimSpace(category)
	if category == "" {
		category = "general"
	}
	res, err := d.conn.Exec(
		"INSERT INTO questions (text, options, answer, category) VALUES (?, ?, ?, ?)",
		text, string(optsJSON), answer, category,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// DeleteQuestion removes a question by ID.
func (d *DB) DeleteQuestion(id int) error {
	res, err := d.conn.Exec("DELETE FROM questions WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("question not found")
	}
	return nil
}

// UpdateQuestion updates an existing question.
func (d *DB) UpdateQuestion(id int, text string, options []string, answer int, category string) error {
	optsJSON, _ := json.Marshal(options)
	category = strings.TrimSpace(category)
	if category == "" {
		category = "general"
	}
	res, err := d.conn.Exec(
		"UPDATE questions SET text = ?, options = ?, answer = ?, category = ? WHERE id = ?",
		text, string(optsJSON), answer, category, id,
	)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("question not found")
	}
	return nil
}

// GetCategories returns distinct question categories.
func (d *DB) GetCategories() ([]string, error) {
	rows, err := d.conn.Query("SELECT DISTINCT category FROM questions ORDER BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	if cats == nil {
		cats = []string{}
	}
	return cats, rows.Err()
}

// GetQuestionIDsByCategories returns question IDs filtered by categories in random order.
func (d *DB) GetQuestionIDsByCategories(categories []string) ([]int, error) {
	if len(categories) == 0 {
		return d.GetQuestionIDs()
	}
	placeholders := ""
	args := make([]interface{}, len(categories))
	for i, c := range categories {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = c
	}
	rows, err := d.conn.Query("SELECT id FROM questions WHERE category IN ("+placeholders+") ORDER BY RANDOM()", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeletePlayer removes a player and their answers.
func (d *DB) DeletePlayer(id string) error {
	d.conn.Exec("DELETE FROM answers WHERE player_id = ?", id)
	res, err := d.conn.Exec("DELETE FROM players WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("player not found")
	}
	return nil
}

// GetAnswerStats returns answer statistics for a question.
func (d *DB) GetAnswerStats(questionID int) (total, correct, wrong int) {
	d.conn.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(correct),0) FROM answers WHERE question_id = ?",
		questionID,
	).Scan(&total, &correct)
	wrong = total - correct
	return
}

// ListAllQuestions returns all questions.
func (d *DB) ListAllQuestions() ([]models.Question, error) {
	rows, err := d.conn.Query("SELECT id, text, options, answer, category FROM questions ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var questions []models.Question
	for rows.Next() {
		var q models.Question
		var optsJSON string
		if err := rows.Scan(&q.ID, &q.Text, &optsJSON, &q.Answer, &q.Category); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(optsJSON), &q.Options)
		questions = append(questions, q)
	}
	if questions == nil {
		questions = []models.Question{}
	}
	return questions, rows.Err()
}

// Conn exposes the raw connection for testing.
func (d *DB) Conn() *sql.DB {
	return d.conn
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}
