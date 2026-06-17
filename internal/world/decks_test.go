package world

import (
	"fmt"
	"testing"
	"time"
)

// The wing caps at MaxDecks: creation is refused when full, and an owner
// retiring a deck frees a slot (non-owners can't).
func TestDeckCapAndRetire(t *testing.T) {
	w := New()
	defer w.Close()
	var ids []string
	for i := 0; i < MaxDecks; i++ {
		id := w.CreateDeck("anna", fmt.Sprintf("Talk %d", i), "x")
		if id == "" {
			t.Fatalf("create %d refused before the cap", i)
		}
		ids = append(ids, id)
	}
	if w.DeckCount() != MaxDecks {
		t.Fatalf("count = %d, want %d", w.DeckCount(), MaxDecks)
	}
	if over := w.CreateDeck("bob", "One Too Many", "x"); over != "" {
		t.Fatalf("create past the cap should be refused, got %q", over)
	}
	if w.RemoveDeck(ids[0], "bob") {
		t.Error("a non-owner retired someone else's deck")
	}
	if !w.RemoveDeck(ids[0], "anna") {
		t.Fatal("owner retire failed")
	}
	if w.DeckCount() != MaxDecks-1 {
		t.Fatalf("count after retire = %d, want %d", w.DeckCount(), MaxDecks-1)
	}
	if id := w.CreateDeck("bob", "Now It Fits", "x"); id == "" {
		t.Fatal("create after a retire should be allowed")
	}
}

// CreateDeck and UpdateDeck fire the persist callback with a snapshot; LoadDeck
// restores a persisted deck without re-persisting.
func TestDeckPersistence(t *testing.T) {
	w := New()
	defer w.Close()
	var saved []Deck
	w.SetDeckPersist(func(d Deck) { saved = append(saved, d) })

	id := w.CreateDeck("anna", "Talk", "a\n---\nb")
	if !w.UpdateDeck(id, "anna", "Talk v2", "x") {
		t.Fatal("owner update failed")
	}
	if len(saved) != 2 {
		t.Fatalf("persist fired %d times, want 2", len(saved))
	}
	if saved[0].ID != id || saved[1].Title != "Talk v2" {
		t.Errorf("persisted snapshots wrong: %+v", saved)
	}

	w2 := New()
	defer w2.Close()
	w2.LoadDeck("restored", "bob", "Old Talk", "p\n---\nq\n---\nr", time.Unix(123, 0))
	d, ok := w2.GetDeck("restored")
	if !ok || d.Owner != "bob" || len(d.Slides) != 3 {
		t.Errorf("LoadDeck restored wrong deck: %+v ok=%v", d, ok)
	}
	if decks := w2.Decks(); len(decks) != 1 {
		t.Errorf("restored deck count = %d, want 1", len(decks))
	}
}

func TestCreateAndGetDeck(t *testing.T) {
	w := New()
	defer w.Close()
	id := w.CreateDeck("anna", "My Talk", "# Hi\n---\nslide two")
	d, ok := w.GetDeck(id)
	if !ok {
		t.Fatal("deck not found after create")
	}
	if d.Owner != "anna" || d.Title != "My Talk" {
		t.Errorf("deck meta wrong: %+v", d)
	}
	if len(d.Slides) != 2 {
		t.Errorf("got %d slides, want 2", len(d.Slides))
	}
	if decks := w.Decks(); len(decks) != 1 || decks[0].ID != id {
		t.Errorf("Decks() = %+v", decks)
	}
}

func TestAdvanceDeckOwnerOnly(t *testing.T) {
	w := New()
	defer w.Close()
	id := w.CreateDeck("anna", "T", "a\n---\nb\n---\nc")
	if got := w.AdvanceDeck(id, "anna", 1); got != 1 {
		t.Fatalf("owner advance = %d, want 1", got)
	}
	if got := w.AdvanceDeck(id, "anna", 5); got != 2 { // clamp to last
		t.Fatalf("advance clamps to %d, want 2", got)
	}
	if got := w.AdvanceDeck(id, "bob", -1); got != -1 { // not owner
		t.Fatalf("non-owner advance = %d, want -1", got)
	}
	if d, _ := w.GetDeck(id); d.Current != 2 {
		t.Errorf("non-owner mutated slide: now %d", d.Current)
	}
}

func TestUpdateDeckReSplitsAndClamps(t *testing.T) {
	w := New()
	defer w.Close()
	id := w.CreateDeck("anna", "T", "a\n---\nb\n---\nc")
	w.AdvanceDeck("anna", "anna", 2) // on slide 2
	if !w.UpdateDeck(id, "anna", "T2", "only one slide now") {
		t.Fatal("owner update failed")
	}
	d, _ := w.GetDeck(id)
	if len(d.Slides) != 1 || d.Current != 0 {
		t.Errorf("after shrink: slides=%d current=%d, want 1/0", len(d.Slides), d.Current)
	}
	if w.UpdateDeck(id, "bob", "hax", "x") {
		t.Error("non-owner update should fail")
	}
}
