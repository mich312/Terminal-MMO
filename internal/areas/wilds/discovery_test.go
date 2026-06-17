package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// The overworld starts hidden: only a circle around the spawn is revealed, the
// far world is fog, and walking uncovers more — the explore-to-see mechanic.
func TestDiscoveryStartsHiddenAndGrows(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/w.db"), Name: name, Theme: ui.Default}

	a, ok := game.NewArea("wilds", ctx).(*area)
	if !ok {
		t.Fatal("wilds factory did not return *area")
	}
	self, _ := w.Self(name)
	a.Init(&self)

	if !a.seen(a.wx, a.wy) {
		t.Fatal("the player's own cell must be discovered at spawn")
	}
	// A cell far from spawn must still be hidden — the world isn't all visible.
	if a.seen(a.wx+50, a.wy+50) {
		t.Fatal("a distant cell should start hidden")
	}
	// fog never blocks movement: collision reads the real generator, not the map.
	if !fogTile().Walkable {
		t.Fatal("fog tiles must stay walkable so they only hide, never wall")
	}

	// Walking to new ground uncovers cells that were previously fogged.
	target := [2]int{a.wx + discoverR + 2, a.wy}
	if a.seen(target[0], target[1]) {
		t.Fatalf("cell %v should be hidden before walking toward it", target)
	}
	a.wx += discoverR + 2
	a.reveal()
	if !a.seen(a.wx, a.wy) || !a.seen(target[0], target[1]) {
		t.Fatal("moving should reveal the new ground around the player")
	}
}

// Position and discovered map must survive leaving and re-entering the Wilds:
// a fresh area built with the same store/name resumes both from persistence.
func TestDiscoveryAndPositionPersist(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/w.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default}

	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)
	// Walk into open ground (not onto the gate portal) so the position sticks.
	walk(a, "ss")
	wantX, wantY := a.wx, a.wy
	if _, isPortal := a.portalUnder(wantX, wantY); isPortal {
		t.Skip("walked onto a portal; positional resume is intentionally skipped there")
	}
	probe := [2]int{wantX, wantY}

	// A brand-new area (as if reconnecting) must resume where we left off and
	// remember the ground we uncovered.
	b := game.NewArea("wilds", ctx).(*area)
	b.Init(&self)
	if b.wx != wantX || b.wy != wantY {
		t.Fatalf("resumed at (%d,%d), want (%d,%d)", b.wx, b.wy, wantX, wantY)
	}
	if !b.seen(probe[0], probe[1]) {
		t.Fatal("discovered ground should persist across re-entry")
	}
}

func walk(a *area, keys string) {
	for _, r := range keys {
		dx, dy, steps, ok := game.MoveKey(string(r))
		if !ok {
			continue
		}
		for i := 0; i < steps; i++ {
			nx, ny := a.wx+dx, a.wy+dy
			if !a.fits(nx, ny) {
				break
			}
			a.wx, a.wy = nx, ny
			a.reveal()
		}
		a.persist()
	}
}
