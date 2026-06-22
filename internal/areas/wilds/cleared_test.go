package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// findBlockingForest scans outward for a forest tree cell (blocks movement).
func findBlockingForest(g *worldgen.Generator) (int, int, bool) {
	for r := 2; r <= 400; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx != -r && dx != r && dy != -r && dy != r {
					continue
				}
				c := g.At(dx, dy)
				if c.Biome == worldgen.Forest && !c.Walkable {
					return dx, dy, true
				}
			}
		}
	}
	return 0, 0, false
}

// TestClearedMakesForestBuildable confirms the overlay turns a blocking tree cell
// into walkable, buildable ground that renders as a clearing.
func TestClearedMakesForestBuildable(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(""), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	x, y, ok := findBlockingForest(a.gen)
	if !ok {
		t.Skip("no forest tree found for this seed")
	}
	a.markSeen(x, y)

	// Before clearing: a tree blocks movement and building.
	if a.walkableAt(x, y) {
		t.Fatal("a standing tree should block movement")
	}
	if a.canBuildAt(x, y) {
		t.Fatal("you can't build on a standing tree")
	}

	if !game.ClearGround(ctx, x, y) {
		t.Fatal("clearing open frontier should succeed")
	}

	// After clearing: walkable and buildable.
	if !a.walkableAt(x, y) {
		t.Error("a cleared cell should be walkable")
	}
	if !a.canBuildAt(x, y) {
		t.Error("a cleared cell should be buildable")
	}

	// And it renders as a walkable clearing in the sampled window.
	a.wx, a.wy = x-3, y // put it in view, off the body
	const vw, vh = 16, 11
	_, ox, oy := a.sample(vw, vh)
	for ly := 0; ly < vh; ly++ {
		for lx := 0; lx < vw; lx++ {
			a.markSeen(ox+lx, oy+ly)
		}
	}
	tm, ox, oy := a.sample(vw, vh)
	lx, ly := x-ox, y-oy
	if lx >= 0 && lx < vw && ly >= 0 && ly < vh {
		if !tm.Tiles[ly][lx].Walkable {
			t.Error("the cleared cell should render as walkable ground")
		}
	}
}
