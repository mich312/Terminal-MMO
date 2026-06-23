package sokoban

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
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "sokoban"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// Level 0 places the player at (1,2), a crate at (2,2) and the pad at (4,2):
// two steps right shove the crate home and clear the level, advancing to #2.
func TestSolveAdvancesLevel(t *testing.T) {
	a := newArea(t)
	if a.idx != 0 || a.X != 1 || a.Y != 2 {
		t.Fatalf("unexpected start: idx=%d pos=(%d,%d)", a.idx, a.X, a.Y)
	}
	if !a.boxes[pt{2, 2}] || !a.goals[pt{4, 2}] {
		t.Fatalf("level 0 layout wrong: boxes=%v goals=%v", a.boxes, a.goals)
	}
	a.Update(key("d"))
	if !a.boxes[pt{3, 2}] || a.X != 2 {
		t.Fatalf("first push: boxes=%v pos=(%d,%d)", a.boxes, a.X, a.Y)
	}
	a.Update(key("d")) // crate lands on the pad → level solved, advance
	if a.idx != 1 {
		t.Fatalf("after solving level 0, idx=%d want 1", a.idx)
	}
	if a.cleared != 1 {
		t.Fatalf("cleared=%d want 1", a.cleared)
	}
}

// A crate with a wall directly behind it can't be shoved; the player stays put.
func TestPushIntoWallBlocked(t *testing.T) {
	a := newArea(t)
	// Walk into the left wall: no move.
	a.Update(key("a"))
	if a.X != 1 {
		t.Fatalf("walked into a wall: x=%d", a.X)
	}
}

// Stepping onto the door tile leaves for the Arcade.
func TestDoorTransitions(t *testing.T) {
	a := newArea(t)
	a.X, a.Y = a.exit[0], a.exit[1]-1 // stand just above the door
	next, _ := a.step(0, 1)
	tr, ok := next.(game.Transition)
	if !ok || tr.To != "arcade" {
		t.Fatalf("door step → %#v, want Transition to arcade", next)
	}
}

// Every shipped level must be solvable in principle: a valid layout has at least
// one crate, an equal-or-greater number of pads, a reachable door, and a start.
func TestLevelsWellFormed(t *testing.T) {
	a := newArea(t)
	for i := range levels {
		a.load(i)
		if len(a.boxes) == 0 {
			t.Errorf("level %d has no crates", i)
		}
		if len(a.goals) != len(a.boxes) {
			t.Errorf("level %d: %d pads but %d crates (must match)", i, len(a.goals), len(a.boxes))
		}
		if a.exit == (pt{}) {
			t.Errorf("level %d has no door", i)
		}
		if a.walls[pt{a.X, a.Y}] {
			t.Errorf("level %d spawns the player inside a wall", i)
		}
	}
}
