package store

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *sqliteStore {
	t.Helper()
	s, err := openSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("openSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func areasOf(t *testing.T, s *sqliteStore, name string) string {
	t.Helper()
	var raw string
	if err := s.db.QueryRow(`SELECT areas_visited FROM players WHERE name = ?`, name).Scan(&raw); err != nil {
		t.Fatalf("read areas_visited: %v", err)
	}
	return raw
}

// RecordAreaVisit appends new areas and dedupes repeats.
func TestRecordAreaVisitAppendsAndDedupes(t *testing.T) {
	s := openTemp(t)
	s.RecordVisit("anna")
	s.RecordAreaVisit("anna", "lobby")
	s.RecordAreaVisit("anna", "wilds")
	s.RecordAreaVisit("anna", "lobby") // repeat — should not duplicate

	if got := areasOf(t, s, "anna"); got != `["lobby","wilds"]` {
		t.Errorf("areas_visited = %s, want [\"lobby\",\"wilds\"]", got)
	}
}

// A corrupt areas_visited blob is preserved, not silently overwritten.
func TestRecordAreaVisitPreservesCorruptBlob(t *testing.T) {
	s := openTemp(t)
	s.RecordVisit("bob")
	if _, err := s.db.Exec(`UPDATE players SET areas_visited = ? WHERE name = ?`, "not json", "bob"); err != nil {
		t.Fatalf("seed corrupt blob: %v", err)
	}

	s.RecordAreaVisit("bob", "lobby") // must not clobber the bad data

	if got := areasOf(t, s, "bob"); got != "not json" {
		t.Errorf("corrupt blob was overwritten to %q", got)
	}
}
