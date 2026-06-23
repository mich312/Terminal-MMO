package tetris

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
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "tetris"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestSpawnValid(t *testing.T) {
	a := newArea(t)
	if a.over {
		t.Fatal("a fresh well should not be game over")
	}
	if !a.valid(a.px, a.py, a.rot) {
		t.Fatal("spawned piece is not in a valid position")
	}
}

func TestGravityDrops(t *testing.T) {
	a := newArea(t)
	py := a.py
	a.gravity()
	if a.py != py+1 {
		t.Fatalf("gravity moved piece to py=%d, want %d", a.py, py+1)
	}
}

// A completely full row is detected and removed, scoring one line.
func TestClearLines(t *testing.T) {
	a := newArea(t)
	a.grid = [playH][playW]int{}
	for x := 0; x < playW; x++ {
		a.grid[playH-1][x] = 1
	}
	a.clearLines()
	if a.lines != 1 {
		t.Fatalf("lines=%d want 1", a.lines)
	}
	for x := 0; x < playW; x++ {
		if a.grid[playH-1][x] != 0 {
			t.Fatal("the full row was not cleared")
		}
	}
}

// Stacking to the top ends the game on the next spawn.
func TestGameOverWhenStacked(t *testing.T) {
	a := newArea(t)
	for y := 0; y < playH; y++ {
		for x := 0; x < playW; x++ {
			a.grid[y][x] = 1
		}
	}
	a.spawn()
	if !a.over {
		t.Fatal("spawning into a full well should end the game")
	}
}

func TestRotateAndLeave(t *testing.T) {
	a := newArea(t)
	a.piece, a.rot, a.px, a.py = 2, 0, 3, 3 // a T-piece with room to turn
	a.Update(key("w"))                      // up = rotate
	if a.rot != 1 {
		t.Fatalf("rotation did not advance: rot=%d", a.rot)
	}
	next, _ := a.Update(key("x"))
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}
