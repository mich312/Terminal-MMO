package arcade

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// The Hall of Fame plinth: e beside it opens the board (both the glyph panel
// and the HD slide), it shows persisted scores best-first, and any key closes.
func TestHallOfFameBoard(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/a.db")
	st.SaveScore("snake", "ada", 7)
	st.SaveScore("snake", "bob", 12)
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default}

	a := game.NewArea("arcade", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	// Stand beside the plinth (H sits at map (14,8)) and open the board.
	a.X, a.Y = 14, 9
	if !a.Map.NearObject(a.X, a.Y, "scores") {
		t.Fatal("(14,9) should be beside the Hall of Fame plinth")
	}
	if !strings.Contains(a.Hint(), "Hall of Fame") {
		t.Fatalf("hint beside the plinth = %q", a.Hint())
	}
	a.Update(key("e"))
	if !a.boardOpen || !a.CapturesInput() {
		t.Fatal("e beside the plinth should open the board and capture input")
	}

	panel := a.scoresPanel()
	if !strings.Contains(panel, "bob — 12 pts") || !strings.Contains(panel, "ada — 7 pts") {
		t.Fatalf("glyph board missing scores:\n%s", panel)
	}
	if strings.Index(panel, "bob") > strings.Index(panel, "ada") {
		t.Fatal("board must list the record holder first")
	}
	src, _, show := a.HDSlide()
	if !show || !strings.Contains(src, "bob — 12 pts") {
		t.Fatalf("HD slide missing scores (show=%v):\n%s", show, src)
	}

	a.Update(key("q"))
	if a.boardOpen {
		t.Fatal("any key should close the board")
	}
	if _, _, show := a.HDSlide(); show {
		t.Fatal("HD slide should hide once the board is closed")
	}
}

// A finished run lands on the board through the shared SubmitScore path, and a
// hall record is announced to everyone online.
func TestSubmitScoreAnnouncesRecords(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, ch := w.Join("ada")
	st := store.Open(t.TempDir() + "/a.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default}

	game.SubmitScore(ctx, "tetris", 40)
	heard := false
	for len(ch) > 0 {
		if ev := <-ch; ev.Type == world.EventAnnounce && strings.Contains(ev.Detail, "record") {
			heard = true
		}
	}
	if !heard {
		t.Fatal("a first record should be announced")
	}

	game.SubmitScore(ctx, "tetris", 10) // a worse run: on the board logic, no announcement
	for len(ch) > 0 {
		if ev := <-ch; ev.Type == world.EventAnnounce {
			t.Fatalf("non-record announced: %q", ev.Detail)
		}
	}
	if top := st.TopScores("tetris", 1); len(top) != 1 || top[0].Score != 40 {
		t.Fatalf("board = %+v, want ada 40", top)
	}
}
