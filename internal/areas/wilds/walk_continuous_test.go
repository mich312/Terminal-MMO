package wilds

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

func newWalkArea(t *testing.T) *area {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	w.EnterArea(name, "wilds", 0, 0, "")
	ctx := &game.Ctx{World: w, Store: store.Open(""), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}}
	self, _ := w.Self(name)
	a := game.NewArea("wilds", ctx).(*area)
	a.Init(&self)
	return a
}

// hold steers the area for d, re-asserting the intent and advancing an injected
// clock the way a client holding a movement key does. It returns a Transition if
// walking carried us through a portal, else nil.
func hold(a *area, dx, dy float64, running bool, d time.Duration) game.Area {
	const slice = 30 * time.Millisecond
	defer func() { a.clock = nil }()
	base := time.Now()
	a.lastTick = base
	for elapsed := time.Duration(0); elapsed < d; elapsed += slice {
		at := base.Add(elapsed + slice)
		a.clock = func() time.Time { return at }
		a.body.SetIntent(dx, dy, running, at)
		if next, _ := a.advance(); next != nil {
			return next
		}
	}
	return nil
}

// A movement key steers rather than steps; the body then walks on the clock.
// This is the whole shape of the change, so assert it directly.
func TestWildsKeySteersAndClockWalks(t *testing.T) {
	a := newWalkArea(t)
	startX, startFX := a.wx, a.body.FX

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if a.wx != startX || a.body.FX != startFX {
		t.Errorf("the key alone moved the body from %.2f to %.2f", startFX, a.body.FX)
	}
	if !a.body.Moving() {
		t.Fatal("the key did not set a steering intent")
	}

	hold(a, 1, 0, false, time.Second)
	if a.body.FX <= startFX {
		t.Fatalf("a second of walking east got from %.2f to %.2f", startFX, a.body.FX)
	}
	if tx, ty := a.body.Tile(); tx != a.wx || ty != a.wy {
		t.Errorf("cell (%d,%d) and body (%d,%d) disagree", a.wx, a.wy, tx, ty)
	}
	// The world's copy is what every other client renders from.
	self, _ := a.ctx.World.Self(a.ctx.Name)
	if self.X != a.wx || self.FX != a.body.FX {
		t.Errorf("world has cell %d body %.2f; area has cell %d body %.2f",
			self.X, self.FX, a.wx, a.body.FX)
	}
}

// The discovery circle is the most visible per-step mechanic in the game, and
// it now has to follow the body rather than the keyboard.
func TestWildsWalkingRevealsGround(t *testing.T) {
	a := newWalkArea(t)
	before := a.wx
	hold(a, 1, 0, false, 3*time.Second)
	if a.wx == before {
		t.Skip("walked straight into something solid; nothing to assert")
	}
	if !a.seen(a.wx, a.wy) {
		t.Error("the cell we walked into was not revealed")
	}
	// Ground behind the old discovery edge, uncovered only by having moved.
	if !a.seen(before+discoverR+1, a.wy) && a.wx > before+1 {
		t.Error("walking did not extend the discovery circle")
	}
}

// Standing still must run no per-step work, or reveal/persist/claim bookkeeping
// fires forever at the host's tick rate.
func TestWildsIdleDoesNoWork(t *testing.T) {
	a := newWalkArea(t)
	startX, startY := a.wx, a.wy
	base := time.Now()
	a.lastTick = base
	for i := 1; i <= 30; i++ {
		at := base.Add(time.Duration(i) * 30 * time.Millisecond)
		a.clock = func() time.Time { return at }
		if next, moved := a.advance(); next != nil || moved {
			t.Fatalf("idle tick %d reported movement", i)
		}
	}
	a.clock = nil
	if a.wx != startX || a.wy != startY {
		t.Errorf("idle ticks moved us from (%d,%d) to (%d,%d)", startX, startY, a.wx, a.wy)
	}
}

// Knocked out means knocked out: no walking until you revive at the hub.
func TestWildsDownedCannotWalk(t *testing.T) {
	a := newWalkArea(t)
	startX := a.wx
	a.ctx.World.MutatePlayer(a.ctx.Name, func(p *world.Player) bool {
		p.DownedUntil = time.Now().Add(time.Minute)
		return true
	})
	hold(a, 1, 0, false, time.Second)
	if a.wx != startX {
		t.Errorf("a downed player walked from %d to %d", startX, a.wx)
	}
}

// A stalled host must not fling the body across the overworld on resume.
func TestWildsSkipsAStalledTick(t *testing.T) {
	a := newWalkArea(t)
	startX := a.wx
	base := time.Now()
	a.lastTick = base
	stalled := base.Add(5 * time.Second)
	a.clock = func() time.Time { return stalled }
	a.body.SetIntent(1, 0, false, stalled)
	a.advance()
	a.clock = nil
	if a.wx != startX {
		t.Errorf("a five-second stall walked us from %d to %d; it should have been skipped",
			startX, a.wx)
	}
}

// place must move the body with the cell. Assigning one without the other is how
// a knockback or a portal arrival gets silently walked back by the next tick.
func TestWildsPlaceMovesTheBody(t *testing.T) {
	a := newWalkArea(t)
	a.place(a.wx+40, a.wy+40)
	if tx, ty := a.body.Tile(); tx != a.wx || ty != a.wy {
		t.Errorf("after place, cell is (%d,%d) but body is on (%d,%d)", a.wx, a.wy, tx, ty)
	}
	if a.body.Moving() {
		t.Error("place left a steering intent behind")
	}
}
