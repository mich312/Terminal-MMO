package wilds

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

// findApproachableItem locates an item that has a walkable cardinal neighbor to
// step in from, returning the item cell and the step direction toward it.
func findApproachableItem(a *area) (ix, iy, dx, dy int, ok bool) {
	dirs := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for r := 1; r < 160; r++ {
		for x := -r; x <= r; x++ {
			for y := -r; y <= r; y++ {
				if _, has := itemAt(a.gen.At(x, y), x, y); !has {
					continue
				}
				for _, d := range dirs {
					ax, ay := x-d[0], y-d[1] // approach tile
					if a.gen.Walkable(ax, ay) && game.CanStep(a.gen.Walkable, ax, ay, d[0], d[1]) {
						return x, y, d[0], d[1], true
					}
				}
			}
		}
	}
	return 0, 0, 0, 0, false
}

// findItemByID locates the first cell carrying a specific item id.
func findItemByID(a *area, id string) (int, int, bool) {
	for r := 1; r < 240; r++ {
		for x := -r; x <= r; x++ {
			for y := -r; y <= r; y++ {
				if it, ok := itemAt(a.gen.At(x, y), x, y); ok && it.ID == id {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// Foraging a mushroom unlocks the matching "shroom" accessory (collect-to-wear),
// and the unlock persists — so some loot doubles as an outfit.
func TestForagingMushroomUnlocksShroom(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/m.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default, Hats: map[int]bool{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	mx, my, ok := findItemByID(a, "mushroom")
	if !ok {
		t.Skip("no mushroom within scan radius for this seed")
	}
	shroom, ok := game.AccessoryIndex("shroom")
	if !ok {
		t.Fatal("shroom accessory should exist")
	}
	if ctx.Hats[shroom] {
		t.Fatal("shroom should start locked")
	}

	a.wx, a.wy = mx, my
	a.collectItem()

	if !ctx.Hats[shroom] {
		t.Fatal("foraging a mushroom should unlock the shroom accessory")
	}
	if !st.LoadHats(name)[shroom] {
		t.Fatal("the shroom unlock should persist to the store")
	}
}

// Walking onto an item collects it automatically — no 'e' press — and removes it
// from the world, just like a manual pickup.
func TestItemAutoPickupOnMove(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/w.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	ix, iy, dx, dy, ok := findApproachableItem(a)
	if !ok {
		t.Fatal("expected an approachable item near the origin")
	}
	it, _ := itemAt(a.gen.At(ix, iy), ix, iy)

	// Stand on the approach tile and step toward the item — no pickUp() call.
	a.wx, a.wy = ix-dx, iy-dy
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(stepKey(dx, dy))}
	a.Update(key)

	if a.wx != ix || a.wy != iy {
		t.Fatalf("player at (%d,%d) did not walk onto item at (%d,%d)", a.wx, a.wy, ix, iy)
	}
	if ctx.Inventory[it.ID] != 1 {
		t.Fatalf("inventory[%s] = %d, want 1 after walking over it", it.ID, ctx.Inventory[it.ID])
	}
	if _, _, _, still := a.itemUnderBody(); still {
		t.Fatal("item should be gone after walking over it")
	}
}

// stepKey maps a cardinal direction to the WASD key the wilds move handler reads.
func stepKey(dx, dy int) string {
	switch {
	case dx > 0:
		return "d"
	case dx < 0:
		return "a"
	case dy > 0:
		return "s"
	default:
		return "w"
	}
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

// Hats of the same type never bunch up: the jittered-grid placement keeps any
// two same-hat finds at least 2*hatMargin tiles apart, so you never stumble on
// the same hat a few blocks away. Scan a wide swath and check every pair.
func TestHatsDoNotCluster(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/w.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default, Hats: map[int]bool{}}
	a := game.NewArea("wilds", ctx).(*area)

	// Collect every hat in a large window, keyed by type.
	byType := map[int][][2]int{}
	const span = 400
	for x := -span; x <= span; x++ {
		for y := -span; y <= span; y++ {
			if h, ok := hatAt(a.gen.At(x, y), x, y); ok {
				byType[h.idx] = append(byType[h.idx], [2]int{x, y})
			}
		}
	}
	if len(byType) == 0 {
		t.Fatal("no hats found in a 800×800 window — placement is too sparse")
	}
	const minSpacing = 2 * hatMargin
	for idx, pts := range byType {
		for i := 0; i < len(pts); i++ {
			for j := i + 1; j < len(pts); j++ {
				dx, dy := pts[i][0]-pts[j][0], pts[i][1]-pts[j][1]
				if dx*dx+dy*dy < minSpacing*minSpacing {
					t.Errorf("hat %d at %v and %v are %d² apart, want ≥ %d² (clustered)",
						idx, pts[i], pts[j], dx*dx+dy*dy, minSpacing*minSpacing)
				}
			}
		}
	}
}

// Finding a hat unlocks it, equips it, and persists ownership across a
// reconnect — the gated find-to-wear mechanic.
func TestHatPickupUnlocksAndEquips(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	st := store.Open(t.TempDir() + "/w.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default, Hats: map[int]bool{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	// Locate a hat somewhere in the world (rare, so scan wide).
	var hx, hy, hidx int
	found := false
	for r := 1; r < 320 && !found; r++ {
		for x := -r; x <= r && !found; x++ {
			for y := -r; y <= r && !found; y++ {
				if h, ok := hatAt(a.gen.At(x, y), x, y); ok {
					hx, hy, hidx, found = x, y, h.idx, true
				}
			}
		}
	}
	if !found {
		t.Skip("no hat within scan radius for this seed")
	}

	a.wx, a.wy = hx, hy
	a.pickUp()
	if !ctx.Hats[hidx] {
		t.Fatalf("hat %d should be unlocked after pickup", hidx)
	}
	if cur, _ := w.Self(name); cur.Accessory != hidx {
		t.Fatalf("avatar accessory = %d, want equipped %d", cur.Accessory, hidx)
	}
	if !st.LoadHats(name)[hidx] {
		t.Fatal("hat ownership should persist to the store")
	}
}
