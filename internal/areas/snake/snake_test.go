package snake

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
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "snake"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// A fresh run is a length-3 snake heading right, with the avatar (world position)
// riding the head.
func TestInitialRun(t *testing.T) {
	a := newArea(t)
	if len(a.body) != 3 || a.dir != (pt{1, 0}) || a.dead {
		t.Fatalf("bad start: len=%d dir=%v dead=%v", len(a.body), a.dir, a.dead)
	}
	if a.X != a.body[0][0] || a.Y != a.body[0][1] {
		t.Fatalf("avatar (%d,%d) not on the head %v", a.X, a.Y, a.body[0])
	}
}

// One tick slides the whole snake forward one cell without growing.
func TestTickMovesForward(t *testing.T) {
	a := newArea(t)
	head := a.body[0]
	a.GameTick()
	if a.body[0] != (pt{head[0] + 1, head[1]}) {
		t.Fatalf("head moved to %v, want %v", a.body[0], pt{head[0] + 1, head[1]})
	}
	if len(a.body) != 3 {
		t.Fatalf("length changed without food: %d", len(a.body))
	}
	if a.X != a.body[0][0] || a.Y != a.body[0][1] {
		t.Fatal("avatar did not follow the head")
	}
}

// Eating a pellet grows the snake and scores; a new pellet is placed off-snake.
func TestEatGrows(t *testing.T) {
	a := newArea(t)
	// Drop the pellet right in front of the head so the next tick eats it.
	head := a.body[0]
	a.food = pt{head[0] + 1, head[1]}
	a.GameTick()
	if a.score != 1 {
		t.Fatalf("score=%d want 1", a.score)
	}
	if len(a.body) != 4 {
		t.Fatalf("length=%d want 4 after eating", len(a.body))
	}
	if a.onSnake(a.food) {
		t.Fatal("new pellet placed on the snake")
	}
}

// Running into the wall ends the run and records the best score.
func TestWallEndsRun(t *testing.T) {
	a := newArea(t)
	for i := 0; i < boardW; i++ { // march right into the east wall
		a.GameTick()
		if a.dead {
			break
		}
	}
	if !a.dead {
		t.Fatal("snake should have died against the wall")
	}
	// Further ticks are a no-op once dead.
	frozen := append([]pt{}, a.body...)
	a.GameTick()
	if len(a.body) != len(frozen) || a.body[0] != frozen[0] {
		t.Fatal("dead snake kept moving")
	}
}

// You can't reverse straight back onto your own neck.
func TestNoInstantReversal(t *testing.T) {
	a := newArea(t) // heading right
	a.Update(key("a"))
	if a.nextDir == (pt{-1, 0}) {
		t.Fatal("accepted a 180° reversal")
	}
	// A perpendicular turn is fine.
	a.Update(key("s"))
	if a.nextDir != (pt{0, 1}) {
		t.Fatalf("turn not accepted: nextDir=%v", a.nextDir)
	}
}

// 'r' restarts a dead run; 'x' leaves for the Arcade.
func TestRestartAndLeave(t *testing.T) {
	a := newArea(t)
	a.dead = true
	a.score = 9
	if next, _ := a.Update(key("r")); next != game.Area(a) || a.dead || a.score != 0 {
		t.Fatalf("restart failed: dead=%v score=%d", a.dead, a.score)
	}
	next, _ := a.Update(key("x"))
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}
