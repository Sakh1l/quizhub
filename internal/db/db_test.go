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

func TestMigrationAndSeed(t *testing.T) {
	d := newTestDB(t)

	count := d.QuestionCount()
	if count != 15 {
		t.Errorf("expected 15 seeded questions, got %d", count)
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

func TestUpdatePlayerScore(t *testing.T) {
	d := newTestDB(t)

	p, _ := d.CreatePlayer("Alice")
	d.UpdatePlayerScore(p.ID, 500)

	got, _ := d.GetPlayer(p.ID)
	if got.Score != 500 {
		t.Errorf("expected score 500, got %d", got.Score)
	}

	d.UpdatePlayerScore(p.ID, 300)
	got, _ = d.GetPlayer(p.ID)
	if got.Score != 800 {
		t.Errorf("expected score 800, got %d", got.Score)
	}
}

func TestLeaderboard(t *testing.T) {
	d := newTestDB(t)

	p1, _ := d.CreatePlayer("Alice")
	p2, _ := d.CreatePlayer("Bob")
	d.UpdatePlayerScore(p1.ID, 200)
	d.UpdatePlayerScore(p2.ID, 500)

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

	q, err := d.GetQuestion(1)
	if err != nil {
		t.Fatalf("GetQuestion: %v", err)
	}
	if q.Text == "" || len(q.Options) == 0 {
		t.Errorf("empty question: %+v", q)
	}
}

func TestGetQuestionIDs(t *testing.T) {
	d := newTestDB(t)

	ids, err := d.GetQuestionIDs()
	if err != nil {
		t.Fatalf("GetQuestionIDs: %v", err)
	}
	if len(ids) != 15 {
		t.Errorf("expected 15 IDs, got %d", len(ids))
	}
}

func TestRecordAnswer(t *testing.T) {
	d := newTestDB(t)

	p, _ := d.CreatePlayer("Alice")

	// First answer should succeed
	recorded, err := d.RecordAnswer(p.ID, 1, 1, true, 500)
	if err != nil {
		t.Fatalf("RecordAnswer: %v", err)
	}
	if !recorded {
		t.Error("expected recorded=true for first answer")
	}

	// Duplicate should be ignored
	recorded, err = d.RecordAnswer(p.ID, 1, 2, false, 0)
	if err != nil {
		t.Fatalf("RecordAnswer duplicate: %v", err)
	}
	if recorded {
		t.Error("expected recorded=false for duplicate answer")
	}
}

func TestHasAnswered(t *testing.T) {
	d := newTestDB(t)

	p, _ := d.CreatePlayer("Alice")

	if d.HasAnswered(p.ID, 1) {
		t.Error("should not have answered yet")
	}

	d.RecordAnswer(p.ID, 1, 0, false, 0)

	if !d.HasAnswered(p.ID, 1) {
		t.Error("should have answered now")
	}
}

func TestGameState(t *testing.T) {
	d := newTestDB(t)

	status, qID, qIdx, startedAt, timeLimit, err := d.GetGameState()
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}
	if status != "lobby" || qID != 0 || qIdx != 0 || startedAt != "" || timeLimit != 15 {
		t.Errorf("unexpected initial state: %s, %d, %d, %s, %d", status, qID, qIdx, startedAt, timeLimit)
	}

	d.SetGameState("question", 5, 2, "2026-01-01T00:00:00Z", 20)

	status, qID, qIdx, startedAt, timeLimit, err = d.GetGameState()
	if err != nil {
		t.Fatalf("GetGameState after set: %v", err)
	}
	if status != "question" || qID != 5 || qIdx != 2 || startedAt != "2026-01-01T00:00:00Z" || timeLimit != 20 {
		t.Errorf("unexpected state: %s, %d, %d, %s, %d", status, qID, qIdx, startedAt, timeLimit)
	}
}

func TestResetGame(t *testing.T) {
	d := newTestDB(t)

	d.CreatePlayer("Alice")
	d.SetGameState("question", 1, 0, "2026-01-01T00:00:00Z", 15)

	if err := d.ResetGame(); err != nil {
		t.Fatalf("ResetGame: %v", err)
	}

	if c := d.PlayerCount(); c != 0 {
		t.Errorf("expected 0 players after reset, got %d", c)
	}

	status, _, _, _, _, _ := d.GetGameState()
	if status != "lobby" {
		t.Errorf("expected lobby after reset, got %s", status)
	}
}

func TestAddQuestion(t *testing.T) {
	d := newTestDB(t)

	before := d.QuestionCount()
	id, err := d.AddQuestion("Custom Q?", []string{"A", "B"}, 0, "custom")
	if err != nil {
		t.Fatalf("AddQuestion: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
	after := d.QuestionCount()
	if after != before+1 {
		t.Errorf("expected %d questions, got %d", before+1, after)
	}
}

func TestSeedIdempotent(t *testing.T) {
	d := newTestDB(t)

	before := d.QuestionCount()
	// Running seed again should not add more
	d.seed()
	after := d.QuestionCount()

	if before != after {
		t.Errorf("seed not idempotent: before=%d, after=%d", before, after)
	}
}
