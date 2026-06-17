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

// Position round-trips, and an absent one reports ok=false.
func TestPositionRoundTrip(t *testing.T) {
	s := openTemp(t)
	if _, _, ok := s.LoadPosition("ada", "wilds"); ok {
		t.Fatal("no position should exist yet")
	}
	s.SavePosition("ada", "wilds", -12, 34)
	s.SavePosition("ada", "wilds", -12, 99) // upsert overwrites
	x, y, ok := s.LoadPosition("ada", "wilds")
	if !ok || x != -12 || y != 99 {
		t.Fatalf("got (%d,%d,%v), want (-12,99,true)", x, y, ok)
	}
}

// Discovery masks round-trip — including a full chunk (all 64 bits set), which
// stores as a negative int64 and must come back as the same uint64 bits.
func TestDiscoveryRoundTrip(t *testing.T) {
	s := openTemp(t)
	s.SaveDiscovery("ada", 0, 0, 0xFFFFFFFFFFFFFFFF) // full chunk
	s.SaveDiscovery("ada", -3, 5, 0x8000000000000001) // high + low bit
	got := s.LoadDiscovery("ada")
	if got[[2]int{0, 0}] != 0xFFFFFFFFFFFFFFFF {
		t.Errorf("full chunk = %#x, want all bits set", got[[2]int{0, 0}])
	}
	if got[[2]int{-3, 5}] != 0x8000000000000001 {
		t.Errorf("partial chunk = %#x, want 0x8000000000000001", got[[2]int{-3, 5}])
	}
}

// Inventory accumulates per item and collected cells are remembered.
func TestInventoryAndCollected(t *testing.T) {
	s := openTemp(t)
	s.AddItem("ada", "berry")
	s.AddItem("ada", "berry")
	s.AddItem("ada", "shell")
	inv := s.LoadInventory("ada")
	if inv["berry"] != 2 || inv["shell"] != 1 {
		t.Fatalf("inventory = %v, want berry:2 shell:1", inv)
	}
	if s.LoadCollected("ada")[[2]int{5, -9}] {
		t.Fatal("(5,-9) should not be collected yet")
	}
	s.MarkCollected("ada", 5, -9)
	s.MarkCollected("ada", 5, -9) // idempotent
	if !s.LoadCollected("ada")[[2]int{5, -9}] {
		t.Fatal("(5,-9) should be collected after marking")
	}
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

// Decks round-trip through SQLite, ordered by created_at, and an edit upserts
// in place while preserving the original creation time.
func TestDeckSaveLoad(t *testing.T) {
	s := openTemp(t)
	s.SaveDeck("d1", "anna", "Talk", "# Hi", 100)
	s.SaveDeck("d2", "bob", "Other", "x", 50)

	decks := s.LoadDecks()
	if len(decks) != 2 {
		t.Fatalf("got %d decks, want 2", len(decks))
	}
	if decks[0].ID != "d2" || decks[1].ID != "d1" { // oldest first
		t.Errorf("order = %s,%s; want d2,d1", decks[0].ID, decks[1].ID)
	}
	if decks[1].Owner != "anna" || decks[1].Source != "# Hi" {
		t.Errorf("d1 record wrong: %+v", decks[1])
	}

	s.SaveDeck("d1", "anna", "Talk v2", "# Edited", 999) // edit
	decks = s.LoadDecks()
	if len(decks) != 2 {
		t.Fatalf("edit changed deck count to %d", len(decks))
	}
	var d1 DeckRecord
	for _, d := range decks {
		if d.ID == "d1" {
			d1 = d
		}
	}
	if d1.Title != "Talk v2" || d1.Source != "# Edited" {
		t.Errorf("edit not applied: %+v", d1)
	}
	if d1.Created != 100 {
		t.Errorf("edit changed created_at to %d, want 100", d1.Created)
	}
}

func TestDeleteDeck(t *testing.T) {
	s := openTemp(t)
	s.SaveDeck("d1", "anna", "T", "x", 1)
	s.SaveDeck("d2", "bob", "U", "y", 2)
	s.DeleteDeck("d1")
	decks := s.LoadDecks()
	if len(decks) != 1 || decks[0].ID != "d2" {
		t.Errorf("after delete, decks = %+v, want just d2", decks)
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
