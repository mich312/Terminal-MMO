package store

import "testing"

// The leaderboard keeps one row per player, only ever raises it, orders best
// first, and reports exactly the runs that beat every stored score.
func TestHighScores(t *testing.T) {
	s := Open(t.TempDir() + "/scores.db")
	defer s.Close()

	if !s.SaveScore("snake", "ada", 5) {
		t.Fatal("the first score on an empty board is the record")
	}
	if s.SaveScore("snake", "bob", 3) {
		t.Fatal("3 did not beat ada's 5 — no record")
	}
	if !s.SaveScore("snake", "bob", 9) {
		t.Fatal("9 beat ada's 5 — record")
	}
	if s.SaveScore("snake", "ada", 2) {
		t.Fatal("a worse personal run is never a record")
	}

	top := s.TopScores("snake", 5)
	if len(top) != 2 || top[0].Name != "bob" || top[0].Score != 9 || top[1].Name != "ada" || top[1].Score != 5 {
		t.Fatalf("board = %+v, want bob 9 then ada 5 (worse runs never lower a row)", top)
	}

	// Boards are per game.
	if got := s.TopScores("tetris", 5); len(got) != 0 {
		t.Fatalf("tetris board should be empty, got %+v", got)
	}

	// The degraded store stays silent and safe.
	var n noopStore
	if n.SaveScore("snake", "ada", 99) {
		t.Fatal("the no-op store must never announce a record")
	}
	if n.TopScores("snake", 5) != nil {
		t.Fatal("the no-op store returns no rows")
	}
}
