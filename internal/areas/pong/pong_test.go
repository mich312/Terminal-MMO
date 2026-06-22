package pong

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
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "pong"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestBallAdvances(t *testing.T) {
	a := newArea(t)
	bx, by := a.bx, a.by
	a.GameTick()
	if a.bx == bx && a.by == by {
		t.Fatal("ball did not move on a tick")
	}
}

func TestPaddleClamp(t *testing.T) {
	a := newArea(t)
	for i := 0; i < 50; i++ {
		a.Update(key("w"))
	}
	if a.playerY < 1 {
		t.Fatalf("paddle escaped the top: y=%d", a.playerY)
	}
	for i := 0; i < 50; i++ {
		a.Update(key("s"))
	}
	if a.playerY+padH > boardH-1 {
		t.Fatalf("paddle escaped the bottom: y=%d", a.playerY)
	}
}

func TestMatchEnds(t *testing.T) {
	a := newArea(t)
	a.youScore = winScore
	a.score(1)
	if !a.over || !a.win {
		t.Fatalf("reaching %d should win the match: over=%v win=%v", winScore, a.over, a.win)
	}
	a.over, a.win = false, false
	a.youScore, a.aiScore = 0, winScore
	a.score(-1)
	if !a.over || a.win {
		t.Fatalf("the house reaching %d should end the match as a loss", winScore)
	}
}

func TestLeave(t *testing.T) {
	a := newArea(t)
	next, _ := a.Update(key("x"))
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}
