package presentation

import (
	"testing"

	"github.com/durst-group/durstworld/internal/world"
)

// The wing always has at least the create booth, and grows one bay per deck.
func TestBuildWingGrowsWithDecks(t *testing.T) {
	rows0, _, st0 := buildWing(nil)
	if len(st0) != 1 || !st0[0].booth {
		t.Fatalf("empty wing should be just the create booth, got %d stages", len(st0))
	}
	w0 := len([]rune(rows0[0]))

	decks := []world.Deck{
		{ID: "a", Title: "Alpha", Owner: "anna", Slides: []string{"x"}},
		{ID: "b", Title: "Beta", Owner: "bob", Slides: []string{"y"}},
	}
	rows, _, st := buildWing(decks)
	if len(st) != 3 { // 2 decks + booth
		t.Fatalf("got %d stages, want 3", len(st))
	}
	if st[2].booth == false || st[0].booth || st[1].booth {
		t.Errorf("only the last stage should be the booth: %+v", st)
	}
	if st[0].deckID != "a" || st[1].deckID != "b" {
		t.Errorf("stage deck ids wrong: %+v", st)
	}
	if len([]rune(rows[0])) <= w0 {
		t.Errorf("wing should widen with decks: %d !> %d", len([]rune(rows[0])), w0)
	}
}

// Every row of a generated wing is the same width (rectangular grid).
func TestBuildWingRectangular(t *testing.T) {
	rows, _, _ := buildWing([]world.Deck{{ID: "a", Title: "A", Owner: "x", Slides: []string{"s"}}})
	w := len([]rune(rows[0]))
	for i, r := range rows {
		if n := len([]rune(r)); n != w {
			t.Errorf("row %d width %d, want %d", i, n, w)
		}
	}
}

// Each deck stage's lectern sits inside that stage's bounds.
func TestLecternsInsideStages(t *testing.T) {
	_, _, stages := buildWing([]world.Deck{
		{ID: "a", Title: "A", Owner: "x", Slides: []string{"s"}},
		{ID: "b", Title: "B", Owner: "y", Slides: []string{"s"}},
	})
	for _, s := range stages {
		if s.lx < s.x0 || s.lx > s.x1 || s.ly < s.y0 || s.ly > s.y1 {
			t.Errorf("lectern (%d,%d) outside stage %+v", s.lx, s.ly, s)
		}
	}
}
