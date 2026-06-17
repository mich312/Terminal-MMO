package world

import "testing"

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
