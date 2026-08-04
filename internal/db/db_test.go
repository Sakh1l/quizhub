package db

import (
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestQuiz creates a quiz and returns its ID.
func newTestQuiz(t *testing.T, d *DB) int {
	t.Helper()
	id, err := d.CreateQuiz("Test Quiz")
	if err != nil {
		t.Fatalf("CreateQuiz: %v", err)
	}
	return id
}

// addTestQuestion inserts a question into the given quiz and returns its ID.
func addTestQuestion(t *testing.T, d *DB, quizID int) int {
	t.Helper()
	id, err := d.AddQuestion(quizID, "What is 2+2?", []string{"1", "2", "4", "8"}, 2, "math")
	if err != nil {
		t.Fatalf("addTestQuestion: %v", err)
	}
	return id
}

// TestMigrationAndSeed verifies the DB initialises cleanly.
// seed() is intentionally a no-op — admins create questions per quiz.
func TestMigrationAndSeed(t *testing.T) {
	d := newTestDB(t)
	quizzes, err := d.ListQuizzes()
	if err != nil {
		t.Fatalf("ListQuizzes: %v", err)
	}
	if len(quizzes) != 0 {
		t.Errorf("expected 0 quizzes on fresh DB (seed is no-op), got %d", len(quizzes))
	}
}

func TestCreateAndGetPlayer(t *testing.T) {
	d := newTestDB(t)
	p, err := d.CreatePlayer("Alice")
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	if p.Nickname != "Alice" || p.Score != 0 || p.ID == "" {
		t.Errorf("unexpected player: %+v", p)
	}
	got, err := d.GetPlayer(p.ID)
	if err != nil {
		t.Fatalf("GetPlayer: %v", err)
	}
	if got.ID != p.ID || got.Nickname != "Alice" {
		t.Errorf("GetPlayer mismatch: %+v", got)
	}
}

func TestListPlayers(t *testing.T) {
	d := newTestDB(t)
	d.CreatePlayer("Alice")
	d.CreatePlayer("Bob")
	players, err := d.ListPlayers()
	if err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}
	if len(players) != 2 {
		t.Errorf("expected 2 players, got %d", len(players))
	}
}

func TestPlayerCount(t *testing.T) {
	d := newTestDB(t)
	if c := d.PlayerCount(); c != 0 {
		t.Errorf("expected 0 players, got %d", c)
	}
	d.CreatePlayer("Alice")
	if c := d.PlayerCount(); c != 1 {
		t.Errorf("expected 1 player, got %d", c)
	}
}

func TestLeaderboard(t *testing.T) {
	d := newTestDB(t)
	p1, _ := d.CreatePlayer("Alice")
	p2, _ := d.CreatePlayer("Bob")
	quizID := newTestQuiz(t, d)
	qID := addTestQuestion(t, d, quizID) // correct answer is index 2
	if _, err := d.SubmitAnswer(p1.ID, qID, 2, true, 200); err != nil {
		t.Fatalf("SubmitAnswer p1: %v", err)
	}
	if _, err := d.SubmitAnswer(p2.ID, qID, 2, true, 500); err != nil {
		t.Fatalf("SubmitAnswer p2: %v", err)
	}
	entries, err := d.Leaderboard()
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Nickname != "Bob" || entries[0].Rank != 1 {
		t.Errorf("expected Bob at rank 1, got %+v", entries[0])
	}
	if entries[1].Nickname != "Alice" || entries[1].Rank != 2 {
		t.Errorf("expected Alice at rank 2, got %+v", entries[1])
	}
}

func TestGetQuestion(t *testing.T) {
	d := newTestDB(t)
	quizID := newTestQuiz(t, d)
	id := addTestQuestion(t, d, quizID)
	q, err := d.GetQuestion(id)
	if err != nil {
		t.Fatalf("GetQuestion: %v", err)
	}
	if q.Text == "" || len(q.Options) == 0 {
		t.Errorf("empty question: %+v", q)
	}
	if q.QuizID != quizID {
		t.Errorf("expected quiz_id %d, got %d", quizID, q.QuizID)
	}
}

func TestGetQuestionIDs(t *testing.T) {
	d := newTestDB(t)
	quizID := newTestQuiz(t, d)
	ids, err := d.GetQuestionIDs(quizID)
	if err != nil {
		t.Fatalf("GetQuestionIDs (empty): %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs on fresh quiz, got %d", len(ids))
	}
	addTestQuestion(t, d, quizID)
	addTestQuestion(t, d, quizID)
	ids, err = d.GetQuestionIDs(quizID)
	if err != nil {
		t.Fatalf("GetQuestionIDs (after add): %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs after adding 2 questions, got %d", len(ids))
	}

	otherQuizID := newTestQuiz(t, d)
	ids, err = d.GetQuestionIDs(otherQuizID)
	if err != nil {
		t.Fatalf("GetQuestionIDs (other quiz): %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected questions to stay scoped to their own quiz, got %d for unrelated quiz", len(ids))
	}
}

func TestSubmitAnswer(t *testing.T) {
	d := newTestDB(t)
	p, _ := d.CreatePlayer("Alice")
	quizID := newTestQuiz(t, d)
	qID := addTestQuestion(t, d, quizID) // need a real question for FK constraint

	recorded, err := d.SubmitAnswer(p.ID, qID, 1, true, 500)
	if err != nil {
		t.Fatalf("SubmitAnswer: %v", err)
	}
	if !recorded {
		t.Error("expected recorded=true for first answer")
	}
	got, _ := d.GetPlayer(p.ID)
	if got.Score != 500 {
		t.Errorf("expected score 500 after first answer, got %d", got.Score)
	}

	// A duplicate submission for the same player+question must be ignored,
	// including its score, even if it claims a different (nonsensical) result.
	recorded, err = d.SubmitAnswer(p.ID, qID, 2, false, 999)
	if err != nil {
		t.Fatalf("SubmitAnswer duplicate: %v", err)
	}
	if recorded {
		t.Error("expected recorded=false for duplicate answer")
	}
	got, _ = d.GetPlayer(p.ID)
	if got.Score != 500 {
		t.Errorf("expected score to stay 500 after duplicate answer, got %d", got.Score)
	}
}

func TestGameState(t *testing.T) {
	d := newTestDB(t)
	state, err := d.GetGameState()
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}
	if state.Status != "lobby" || state.QuestionID != 0 || state.QuestionIndex != 0 || state.StartedAt != "" || state.TimeLimit != 15 || state.RoomCode != "" {
		t.Errorf("unexpected initial state: %+v", state)
	}
	d.SetGameState(State{Status: "question", QuestionID: 5, QuestionIndex: 2, StartedAt: "2026-01-01T00:00:00Z", TimeLimit: 20})
	state, err = d.GetGameState()
	if err != nil {
		t.Fatalf("GetGameState after set: %v", err)
	}
	if state.Status != "question" || state.QuestionID != 5 || state.QuestionIndex != 2 || state.StartedAt != "2026-01-01T00:00:00Z" || state.TimeLimit != 20 || state.RoomCode != "" {
		t.Errorf("unexpected state: %+v", state)
	}
}

func TestResetGame(t *testing.T) {
	d := newTestDB(t)
	d.CreatePlayer("Alice")
	d.SetGameState(State{Status: "question", QuestionID: 1, QuestionIndex: 0, StartedAt: "2026-01-01T00:00:00Z", TimeLimit: 15})
	if err := d.ResetGame(); err != nil {
		t.Fatalf("ResetGame: %v", err)
	}
	if c := d.PlayerCount(); c != 0 {
		t.Errorf("expected 0 players after reset, got %d", c)
	}
	state, _ := d.GetGameState()
	if state.Status != "lobby" {
		t.Errorf("expected lobby after reset, got %s", state.Status)
	}
}

func TestResetGameKeepsQuizLibrary(t *testing.T) {
	d := newTestDB(t)
	quizID := newTestQuiz(t, d)
	addTestQuestion(t, d, quizID)
	d.CreatePlayer("Alice")
	code := "ABC123"
	if err := d.CreateRoom(code, quizID); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if err := d.ResetGame(); err != nil {
		t.Fatalf("ResetGame: %v", err)
	}

	questions, err := d.ListQuestionsByQuiz(quizID)
	if err != nil {
		t.Fatalf("ListQuestionsByQuiz: %v", err)
	}
	if len(questions) != 1 {
		t.Errorf("expected the quiz's question to survive reset, got %d", len(questions))
	}
	state, err := d.GetGameState()
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}
	if state.QuizID != 0 || state.RoomCode != "" {
		t.Errorf("expected reset to clear the active quiz/room, got %+v", state)
	}
}

func TestAddQuestion(t *testing.T) {
	d := newTestDB(t)
	quizID := newTestQuiz(t, d)
	before, err := d.ListQuestionsByQuiz(quizID)
	if err != nil {
		t.Fatalf("ListQuestionsByQuiz: %v", err)
	}
	id, err := d.AddQuestion(quizID, "Custom Q?", []string{"A", "B"}, 0, "custom")
	if err != nil {
		t.Fatalf("AddQuestion: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
	after, err := d.ListQuestionsByQuiz(quizID)
	if err != nil {
		t.Fatalf("ListQuestionsByQuiz: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("expected %d questions, got %d", len(before)+1, len(after))
	}
}

func TestSeedIdempotent(t *testing.T) {
	d := newTestDB(t)
	before, _ := d.ListQuizzes()
	d.seed()
	after, _ := d.ListQuizzes()
	if len(before) != len(after) {
		t.Errorf("seed not idempotent: before=%d, after=%d", len(before), len(after))
	}
}

func TestQuizCRUD(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateQuiz("Trivia Night")
	if err != nil {
		t.Fatalf("CreateQuiz: %v", err)
	}
	quiz, err := d.GetQuiz(id)
	if err != nil {
		t.Fatalf("GetQuiz: %v", err)
	}
	if quiz.Title != "Trivia Night" || quiz.QuestionCount != 0 {
		t.Errorf("unexpected quiz: %+v", quiz)
	}

	addTestQuestion(t, d, id)
	addTestQuestion(t, d, id)
	quiz, _ = d.GetQuiz(id)
	if quiz.QuestionCount != 2 {
		t.Errorf("expected question_count 2, got %d", quiz.QuestionCount)
	}

	quizzes, err := d.ListQuizzes()
	if err != nil {
		t.Fatalf("ListQuizzes: %v", err)
	}
	if len(quizzes) != 1 {
		t.Fatalf("expected 1 quiz, got %d", len(quizzes))
	}

	if err := d.RenameQuiz(id, "Renamed Night"); err != nil {
		t.Fatalf("RenameQuiz: %v", err)
	}
	quiz, _ = d.GetQuiz(id)
	if quiz.Title != "Renamed Night" {
		t.Errorf("expected renamed title, got %q", quiz.Title)
	}

	dupID, err := d.DuplicateQuiz(id)
	if err != nil {
		t.Fatalf("DuplicateQuiz: %v", err)
	}
	dup, err := d.GetQuiz(dupID)
	if err != nil {
		t.Fatalf("GetQuiz(dup): %v", err)
	}
	if dup.Title != "Renamed Night (Copy)" || dup.QuestionCount != 2 {
		t.Errorf("unexpected duplicate: %+v", dup)
	}

	if err := d.DeleteQuiz(id); err != nil {
		t.Fatalf("DeleteQuiz: %v", err)
	}
	if _, err := d.GetQuiz(id); err == nil {
		t.Error("expected error getting deleted quiz")
	}
	remaining, _ := d.ListQuestionsByQuiz(id)
	if len(remaining) != 0 {
		t.Errorf("expected deleted quiz's questions to be gone, got %d", len(remaining))
	}

	// The duplicate is untouched by deleting the original.
	if _, err := d.GetQuiz(dupID); err != nil {
		t.Errorf("expected duplicate quiz to survive original's deletion: %v", err)
	}
}

func TestDeleteQuizNotFound(t *testing.T) {
	d := newTestDB(t)
	if err := d.DeleteQuiz(999); err == nil {
		t.Error("expected error deleting nonexistent quiz")
	}
}
