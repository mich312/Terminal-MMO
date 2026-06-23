package twenty48

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

func newArea(t *testing.T) *area {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("tester")
	ctx := &game.Ctx{World: w, Name: name}
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "2048"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestCompressMerges(t *testing.T) {
	out, gained, moved := compress([size]int{2, 2, 0, 0})
	if out != [size]int{4, 0, 0, 0} || gained != 4 || !moved {
		t.Fatalf("compress(2,2,0,0) = %v gained=%d moved=%v", out, gained, moved)
	}
	out, _, moved = compress([size]int{2, 2, 2, 2})
	if out != [size]int{4, 4, 0, 0} || !moved {
		t.Fatalf("compress(2,2,2,2) = %v (want 4,4,0,0)", out)
	}
	if _, _, moved := compress([size]int{2, 4, 8, 16}); moved {
		t.Fatal("a line with no merges or gaps must report not-moved")
	}
}

func TestSlideMergesAndSpawns(t *testing.T) {
	a := newArea(t)
	a.grid = [size][size]int{}
	a.grid[0] = [size]int{2, 2, 0, 0}
	a.score = 0
	if !a.slide("left") {
		t.Fatal("a merging slide should report moved")
	}
	if a.grid[0][0] != 4 {
		t.Fatalf("row after left-slide = %v, want a 4 at the head", a.grid[0])
	}
	if a.score != 4 {
		t.Fatalf("score=%d want 4 from the merge", a.score)
	}
}

func TestMovesLeft(t *testing.T) {
	a := newArea(t)
	// A full board with no equal neighbours has no moves.
	full := [size][size]int{
		{2, 4, 2, 4}, {4, 2, 4, 2}, {2, 4, 2, 4}, {4, 2, 4, 2},
	}
	a.grid = full
	if a.movesLeft() {
		t.Fatal("checkerboard full board should have no moves")
	}
	a.grid[3][3] = 4 // create a vertical pair
	if !a.movesLeft() {
		t.Fatal("a mergeable pair should count as a move")
	}
}

func TestLeave(t *testing.T) {
	a := newArea(t)
	next, _ := a.Update(key("x"))
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}
