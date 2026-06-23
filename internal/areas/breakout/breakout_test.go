package breakout

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
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "breakout"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestBricksLaidAndStuck(t *testing.T) {
	a := newArea(t)
	want := len(brickRows) * (boardW - 4)
	if len(a.bricks) != want {
		t.Fatalf("laid %d bricks, want %d", len(a.bricks), want)
	}
	if !a.stuck {
		t.Fatal("ball should start stuck to the paddle")
	}
	a.GameTick() // stuck → no movement
	if !a.stuck {
		t.Fatal("a tick should not move a stuck ball")
	}
	a.Update(key("d")) // a move launches it
	if a.stuck {
		t.Fatal("moving should release the ball")
	}
}

func TestBrickBreakScores(t *testing.T) {
	a := newArea(t)
	a.stuck = false
	a.bx, a.by, a.vx, a.vy = 5, 7, 1, -1 // aimed up-right into the brick at (6,6)
	before := len(a.bricks)
	a.GameTick()
	if len(a.bricks) != before-1 {
		t.Fatalf("brick count %d, want %d", len(a.bricks), before-1)
	}
	if a.score != 10 {
		t.Fatalf("score=%d want 10", a.score)
	}
}

func TestLoseLifeOnDrop(t *testing.T) {
	a := newArea(t)
	a.stuck = false
	a.padX = boardW / 2                        // paddle centre…
	a.bx, a.by, a.vx, a.vy = 3, boardH-2, 1, 1 // …ball dropping far to the left of it
	a.GameTick()
	if a.lives != startLife-1 {
		t.Fatalf("lives=%d want %d after a drop", a.lives, startLife-1)
	}
	if !a.stuck {
		t.Fatal("after losing a life the ball should re-stick to the paddle")
	}
}

func TestLeave(t *testing.T) {
	a := newArea(t)
	next, _ := a.Update(key("x"))
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}
