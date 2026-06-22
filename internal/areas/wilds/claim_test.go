package wilds

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// findPlot rings outward from the origin for the nearest settlement building cell.
func findPlot(a *area) (int, int, worldgen.Plot, bool) {
	for r := 1; r <= 900; r++ {
		for dx := -r; dx <= r; dx++ {
			for _, dy := range [2]int{-r, r} {
				if p, ok := a.gen.PlotAt(dx, dy); ok {
					return dx, dy, p, true
				}
			}
		}
		for dy := -r + 1; dy <= r-1; dy++ {
			for _, dx := range [2]int{-r, r} {
				if p, ok := a.gen.PlotAt(dx, dy); ok {
					return dx, dy, p, true
				}
			}
		}
	}
	return 0, 0, worldgen.Plot{}, false
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// TestWildsClaimRelease drives the build-mode claim/release wiring: e over a
// settlement building deeds it; x over your own plot releases it.
func TestWildsClaimRelease(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(""), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	bx, by, p, ok := findPlot(a)
	if !ok {
		t.Skip("no settlement plot found within range for this seed")
	}

	// Enter build mode and put the ghost on the building.
	a.building = true
	a.bx, a.by = bx, by

	// e → claim it.
	a.Update(key("e"))
	c, mine, held := game.WorkspaceAt(ctx, bx, by)
	if !held || !mine || c.PlotID != p.ID {
		t.Fatalf("after e, claim = (%+v, mine=%v, held=%v), want ada holding %s", c, mine, held, p.ID)
	}

	// A stranger can't build inside the new parcel.
	stranger := &game.Ctx{World: w, Store: store.Open(""), Name: "mallory"}
	if okb, who := game.BuildRight(stranger, bx, by); okb || who != "ada" {
		t.Errorf("stranger BuildRight = (%v,%q), want blocked by ada", okb, who)
	}

	// x → release it.
	a.Update(key("x"))
	if _, _, held := game.WorkspaceAt(ctx, bx, by); held {
		t.Error("after x, the plot should be unclaimed")
	}
}

// TestWildsClaimBlocksForeignBuild confirms a held parcel stops another player
// placing on its open ground via the area's own canBuildAt.
func TestWildsClaimBlocksForeignBuild(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	owner, _ := w.Join("ada")
	octx := &game.Ctx{World: w, Store: store.Open(""), Name: owner, Theme: ui.Default,
		Inventory: map[string]int{}}
	oa := game.NewArea("wilds", octx).(*area)
	os, _ := w.Self(owner)
	oa.Init(&os)

	bx, by, p, ok := findPlot(oa)
	if !ok {
		t.Skip("no settlement plot found within range for this seed")
	}
	if !game.ClaimWorkspace(octx, p.ID, p.AX, p.AY, p.W, p.H) {
		t.Fatal("owner should claim the free plot")
	}

	// A second player's area in the same world: a parcel cell is not buildable.
	other, _ := w.Join("bob")
	bctx := &game.Ctx{World: w, Store: store.Open(""), Name: other, Theme: ui.Default,
		Inventory: map[string]int{}}
	ba := game.NewArea("wilds", bctx).(*area)
	bs, _ := w.Self(other)
	ba.Init(&bs)
	ba.markSeen(bx, by)
	if ba.canBuildAt(bx, by) {
		t.Error("a foreign player should not be able to build inside ada's parcel")
	}
}
