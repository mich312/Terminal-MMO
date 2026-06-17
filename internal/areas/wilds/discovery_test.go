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

	if len(a.discovered) == 0 {
		t.Fatal("spawn should reveal a starting circle")
	}
	// A cell far from spawn must still be hidden — the world isn't all visible.
	far := [2]int{a.wx + 50, a.wy + 50}
	if a.discovered[far] {
		t.Fatalf("distant cell %v should start hidden", far)
	}
	// fog never blocks movement: collision reads the real generator, not the map.
	if !fogTile().Walkable {
		t.Fatal("fog tiles must stay walkable so they only hide, never wall")
	}

	// Walking to new ground uncovers cells that were previously fogged.
	before := len(a.discovered)
	prev := [2]int{a.wx + discoverR + 2, a.wy}
	if a.discovered[prev] {
		t.Fatalf("cell %v should be hidden before walking toward it", prev)
	}
	a.wx += discoverR + 2
	a.reveal()
	if len(a.discovered) <= before {
		t.Fatal("moving should reveal new cells")
	}
	if !a.discovered[[2]int{a.wx, a.wy}] {
		t.Fatal("the player's own cell must always be discovered")
	}
}
