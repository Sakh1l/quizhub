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
			room_code TEXT,
			current_question_id INTEGER,
			question_index INTEGER NOT NULL DEFAULT 0,
			question_started_at TEXT,
			time_limit INTEGER NOT NULL DEFAULT 15,
			question_ids TEXT NOT NULL DEFAULT '[]'
		)`,
	}

	for _, s := range stmts {
		if _, err := d.conn.Exec(s); err != nil {
			return fmt.Errorf("exec %s: %w", s[:40], err)
		}
	}

	if err := d.addColumnIfMissing("game_state", "room_code", "room_code TEXT"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("game_state", "question_ids", "question_ids TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}

	// Ensure game_state row exists
	if _, err := d.conn.Exec("PRAGMA user_version = 2"); err != nil {
		return err
	}
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO game_state (id, status) VALUES (1, 'lobby')`)
	return err
}

func (d *DB) addColumnIfMissing(table, column, definition string) error {
	rows, err := d.conn.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = d.conn.Exec("ALTER TABLE " + table + " ADD COLUMN " + definition)
	return err
}

func (d *DB) seed() error {
	// No seed data — admin creates questions per quiz
	return nil
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

// GetQuestionIDs returns every question ID in the bank, in random order. Used
// as the fallback question set when the admin hasn't staged a specific
// selection via UpdateSelectedQuestions before starting a game. Contrast with
// ActiveQuestionIDs, which returns the fixed order locked in for the game
// currently in progress.
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

// --- Game state operations ---

// State is the raw persisted game state for the single active room.
type State struct {
	Status        string
	QuestionID    int
	QuestionIndex int
	StartedAt     string // RFC3339, empty if the current question hasn't started
	TimeLimit     int
	RoomCode      string
}

// HasActiveQuestion reports whether a question is currently being asked
// (as opposed to lobby, countdown, reveal, or finished).
func (s State) HasActiveQuestion() bool {
	return s.Status == "question" && s.QuestionID != 0
}

// GetGameState returns the current game state.
func (d *DB) GetGameState() (State, error) {
	var s State
	err := d.conn.QueryRow(
		"SELECT status, COALESCE(current_question_id, 0), question_index, COALESCE(question_started_at, ''), time_limit, COALESCE(room_code, '') FROM game_state WHERE id = 1",
	).Scan(&s.Status, &s.QuestionID, &s.QuestionIndex, &s.StartedAt, &s.TimeLimit, &s.RoomCode)
	return s, err
}

// SetGameState updates the game state.
func (d *DB) SetGameState(s State) error {
	var qIDVal interface{} = s.QuestionID
	if s.QuestionID == 0 {
		qIDVal = nil
	}
	var startedVal interface{} = s.StartedAt
	if s.StartedAt == "" {
		startedVal = nil
	}
	_, err := d.conn.Exec(
		"UPDATE game_state SET status = ?, current_question_id = ?, question_index = ?, question_started_at = ?, time_limit = ? WHERE id = 1",
		s.Status, qIDVal, s.QuestionIndex, startedVal, s.TimeLimit,
	)
	return err
}

// SetTimeLimit stores the configured question duration for the next game.
func (d *DB) SetTimeLimit(timeLimit int) error {
	_, err := d.conn.Exec("UPDATE game_state SET time_limit = ? WHERE id = 1", timeLimit)
	return err
}

// SetRoomCode sets the room code.
func (d *DB) SetRoomCode(code string) error {
	_, err := d.conn.Exec("UPDATE game_state SET room_code = ? WHERE id = 1", code)
	return err
}

// GetRoomCode returns the current room code.
func (d *DB) GetRoomCode() string {
	var code string
	d.conn.QueryRow("SELECT COALESCE(room_code, '') FROM game_state WHERE id = 1").Scan(&code)
	return code
}

// ResetGame clears all players, answers, questions, and resets game state.
func (d *DB) ResetGame() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		"DELETE FROM answers",
		"DELETE FROM players",
		"DELETE FROM questions",
		"UPDATE game_state SET status = 'lobby', room_code = NULL, current_question_id = NULL, question_index = 0, question_started_at = NULL, question_ids = '[]' WHERE id = 1",
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return tx.Commit()
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

// CreateRoom stores/updates the active room code and resets to lobby state.
func (d *DB) CreateRoom(code string) error {
	state, err := d.GetGameState()
	if err != nil {
		return err
	}
	timeLimit := state.TimeLimit
	if timeLimit <= 0 {
		timeLimit = 15
	}
	if state.Status != "lobby" {
		if err := d.SetGameState(State{Status: "lobby", TimeLimit: timeLimit}); err != nil {
			return err
		}
	}
	_, err = d.conn.Exec("UPDATE game_state SET room_code = ?, question_ids = '[]' WHERE id = 1", code)
	return err
}

// StartGame initializes game state at first question.
func (d *DB) StartGame(ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("no questions")
	}
	state, err := d.GetGameState()
	if err != nil {
		return err
	}
	if state.Status != "lobby" {
		return fmt.Errorf("game already active")
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec(
		"UPDATE game_state SET status = 'countdown', current_question_id = ?, question_index = 0, question_started_at = NULL, time_limit = ?, question_ids = ? WHERE id = 1",
		ids[0], state.TimeLimit, string(idsJSON),
	)
	return err
}

// BeginCurrentQuestion marks the current question as active and starts its timer.
func (d *DB) BeginCurrentQuestion() error {
	state, err := d.GetGameState()
	if err != nil {
		return err
	}
	if state.QuestionID == 0 {
		return fmt.Errorf("no current question")
	}
	state.Status = "question"
	state.StartedAt = time.Now().UTC().Format(time.RFC3339)
	return d.SetGameState(state)
}

// ActiveQuestionIDs returns the persisted question order for the active game.
func (d *DB) ActiveQuestionIDs() ([]int, error) {
	var raw string
	if err := d.conn.QueryRow("SELECT COALESCE(question_ids, '[]') FROM game_state WHERE id = 1").Scan(&raw); err != nil {
		return nil, err
	}
	var ids []int
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// NextQuestion advances through the persisted selected question order.
func (d *DB) NextQuestion() (bool, error) {
	state, err := d.GetGameState()
	if err != nil {
		return false, err
	}
	ids, err := d.ActiveQuestionIDs()
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	nextIdx := state.QuestionIndex + 1
	if nextIdx >= len(ids) {
		state.Status = "finished"
		state.StartedAt = ""
		if err := d.SetGameState(state); err != nil {
			return false, err
		}
		return false, nil
	}
	state.Status = "question"
	state.QuestionID = ids[nextIdx]
	state.QuestionIndex = nextIdx
	state.StartedAt = time.Now().UTC().Format(time.RFC3339)
	return true, d.SetGameState(state)
}

func (d *DB) SubmitAnswer(playerID string, questionID, selected int, correct bool, score int) (bool, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	correctInt := 0
	if correct {
		correctInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(
		`INSERT OR IGNORE INTO answers (player_id, question_id, selected, correct, score_earned, answered_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		playerID, questionID, selected, correctInt, score, now,
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return false, tx.Commit()
	}
	if score > 0 {
		if _, err := tx.Exec("UPDATE players SET score = score + ? WHERE id = ?", score, playerID); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}
