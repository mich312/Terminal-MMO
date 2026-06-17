package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// findItem locates the first scattered item near the origin (there's always one
// within a modest radius given the scatter rate).
func findItem(a *area) (int, int, bool) {
	for r := 1; r < 160; r++ {
		for x := -r; x <= r; x++ {
			for y := -r; y <= r; y++ {
				if _, ok := itemAt(a.gen.At(x, y), x, y); ok {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// Scatter is deterministic and biome-appropriate, and picking an item up adds
// it to the inventory, persists it, and removes it from the world for that
// player (across a reconnect).
func TestItemPickupAndPersist(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/w.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default}

	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	ix, iy, ok := findItem(a)
	if !ok {
		t.Fatal("expected at least one item near the origin")
	}
	it, ok := itemAt(a.gen.At(ix, iy), ix, iy)
	if !ok {
		t.Fatal("itemAt disagreed with findItem")
	}
	if it2, _ := itemAt(a.gen.At(ix, iy), ix, iy); it2.ID != it.ID {
		t.Fatal("itemAt is not deterministic")
	}

	// Stand on it and harvest.
	a.wx, a.wy = ix, iy
	a.pickUp()
	if ctx.Inventory[it.ID] != 1 {
		t.Fatalf("inventory[%s] = %d, want 1", it.ID, ctx.Inventory[it.ID])
	}
	if _, _, _, still := a.itemUnderBody(); still {
		t.Fatal("item should be gone after pickup")
	}

	// Reconnect: a fresh session reloads inventory + collected from the store.
	ctx2 := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default,
		Inventory: st.LoadInventory(name)}
	b := game.NewArea("wilds", ctx2).(*area)
	b.Init(&self)
	if ctx2.Inventory[it.ID] != 1 {
		t.Fatalf("persisted inventory[%s] = %d, want 1", it.ID, ctx2.Inventory[it.ID])
	}
	b.wx, b.wy = ix, iy
	if _, _, _, still := b.itemUnderBody(); still {
		t.Fatal("harvested cell should stay empty after reconnect")
	}
}
