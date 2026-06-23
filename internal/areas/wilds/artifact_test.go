package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/world"
)

// A legend is found once: the first hero to stand on it claims it into their
// pack and into the world registry; afterward the spot is empty for everyone.
func TestClaimArtifactOncePerWorld(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)

	hero := newFighter(t, w, "hero", 0, 0)
	cell, ok := hero.artifactCell["durstbane"]
	if !ok {
		t.Skip("durstbane wasn't placed for this seed")
	}
	hero.wx, hero.wy = cell[0], cell[1]

	if _, _, _, ok := hero.artifactUnderBody(); !ok {
		t.Fatal("the hero should see the legend underfoot")
	}
	hero.pickUp()
	if hero.ctx.Inventory["durstbane"] != 1 {
		t.Fatalf("after claim, pack durstbane = %d, want 1", hero.ctx.Inventory["durstbane"])
	}
	if owner, claimed := w.ArtifactClaimed("durstbane"); !claimed || owner != hero.ctx.Name {
		t.Fatalf("world claim: owner=%q claimed=%v, want %s/true", owner, claimed, hero.ctx.Name)
	}

	// A rival arriving at the very same spot finds nothing — it's a legend, gone.
	rival := newFighter(t, w, "rival", 0, 0)
	rival.wx, rival.wy = cell[0], cell[1]
	if _, _, _, ok := rival.artifactUnderBody(); ok {
		t.Fatal("a claimed legend must not appear in the world again")
	}
	rival.pickUp()
	if rival.ctx.Inventory["durstbane"] != 0 {
		t.Fatal("the rival must not be able to claim an already-taken legend")
	}
}

// Every legend lands on standable ground (so it can actually be reached).
func TestArtifactsArePlaceable(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	a := newFighter(t, w, "ada", 0, 0)
	if len(a.artifactCell) == 0 {
		t.Fatal("no legends were placed")
	}
	for id, cell := range a.artifactCell {
		if !a.fits(cell[0], cell[1]) {
			t.Errorf("legend %q at %v is not standable", id, cell)
		}
	}
}
