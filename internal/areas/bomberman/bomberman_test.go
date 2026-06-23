package bomberman

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
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "bomberman"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestSpawnCornerClearAndEnemies(t *testing.T) {
	a := newArea(t)
	for _, c := range []pt{{1, 1}, {1, 2}, {2, 1}} {
		if a.soft[c] {
			t.Fatalf("spawn corner cell %v should be clear", c)
		}
	}
	if len(a.enemies) != enemyN {
		t.Fatalf("placed %d enemies, want %d", len(a.enemies), enemyN)
	}
}

func TestDetonateDestroysSoftBlock(t *testing.T) {
	a := newArea(t)
	a.soft = map[pt]bool{{1, 2}: true} // a block one south of the bomb
	a.flames = map[pt]int{}
	a.bombs = map[pt]int{{1, 1}: 1}
	a.detonate(pt{1, 1})
	if a.soft[pt{1, 2}] {
		t.Fatal("the blast should have destroyed the adjacent soft block")
	}
	if a.flames[pt{1, 1}] == 0 || a.flames[pt{1, 2}] == 0 {
		t.Fatal("flame should cover the bomb cell and the blasted block")
	}
	if _, ok := a.bombs[pt{1, 1}]; ok {
		t.Fatal("the bomb should be consumed by its own detonation")
	}
}

func TestPlayerCaughtLosesLife(t *testing.T) {
	a := newArea(t)
	a.X, a.Y = 1, 1
	a.flames = map[pt]int{{1, 1}: 2}
	before := a.lives
	a.checkPlayer()
	if a.lives != before-1 {
		t.Fatalf("lives=%d want %d after standing in flame", a.lives, before-1)
	}
}

func TestWinWhenCleared(t *testing.T) {
	a := newArea(t)
	a.enemies = nil
	a.GameTick()
	if !a.over || !a.won {
		t.Fatalf("clearing every foe should win: over=%v won=%v", a.over, a.won)
	}
}

func TestLeave(t *testing.T) {
	a := newArea(t)
	next, _ := a.Update(key("x"))
	if tr, ok := next.(game.Transition); !ok || tr.To != "arcade" {
		t.Fatalf("x → %#v, want Transition to arcade", next)
	}
}
