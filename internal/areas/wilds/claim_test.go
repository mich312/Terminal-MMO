package wilds

import (
	"image/png"
	"os"
	"strings"
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

// TestClaimGroundTint confirms a claimed parcel tints the sampled window, and
// only within the parcel — the in-world "reads at a glance" marker.
func TestClaimGroundTint(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(""), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	_, _, p, ok := findPlot(a)
	if !ok {
		t.Skip("no settlement plot found within range for this seed")
	}
	// Stand on the plot and reveal the whole window so nothing is fog.
	a.wx, a.wy = p.AX, p.AY
	const vw, vh = 24, 15
	_, ox, oy := a.sample(vw, vh)
	for ly := 0; ly < vh; ly++ {
		for lx := 0; lx < vw; lx++ {
			a.markSeen(ox+lx, oy+ly)
		}
	}

	before, ox, oy := a.sample(vw, vh)
	if !game.ClaimWorkspace(ctx, p.ID, p.AX, p.AY, p.W, p.H) {
		t.Fatal("owner should claim the plot")
	}
	after, _, _ := a.sample(vw, vh)

	c, _ := w.ClaimForPlot(p.ID)
	changed := 0
	for ly := 0; ly < vh; ly++ {
		for lx := 0; lx < vw; lx++ {
			b, af := before.Tiles[ly][lx], after.Tiles[ly][lx]
			if b.Ground == af.Ground && b.Color == af.Color {
				continue
			}
			changed++
			if wx, wy := ox+lx, oy+ly; !c.Covers(wx, wy) {
				t.Errorf("cell (%d,%d) changed but is outside the parcel", wx, wy)
			}
		}
	}
	if changed == 0 {
		t.Error("claiming a plot should tint at least one visible parcel cell")
	}
}

// TestClaimLabel confirms the HD banner label reflects the claim under the body.
func TestClaimLabel(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(""), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	if _, ok := a.ClaimLabel(); ok {
		t.Error("at spawn (no settlement) there should be no claim label")
	}
	_, _, p, ok := findPlot(a)
	if !ok {
		t.Skip("no settlement plot found within range")
	}
	a.wx, a.wy = p.AX, p.AY
	game.ClaimWorkspace(ctx, p.ID, p.AX, p.AY, p.W, p.H)
	label, ok := a.ClaimLabel()
	if !ok || !strings.Contains(label, "Workspace") {
		t.Errorf("ClaimLabel = (%q,%v), want a Workspace label", label, ok)
	}
}

// TestClaimPreview renders a claimed settlement plot to a PNG (the parcel tint
// plus the HD area title + claim banner), gated on DURST_PREVIEW=1.
func TestClaimPreview(t *testing.T) {
	if os.Getenv("DURST_PREVIEW") != "1" {
		t.Skip("set DURST_PREVIEW=1 to write the preview frame")
	}
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(""), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	_, _, p, ok := findPlot(a)
	if !ok {
		t.Skip("no settlement plot found within range")
	}
	game.ClaimWorkspace(ctx, p.ID, p.AX, p.AY, p.W, p.H)
	a.wx, a.wy = p.AX+1, p.AY+p.H+1 // stand just south of the building, inside the parcel

	const vw, vh = 28, 18
	_, ox, oy := a.sample(vw, vh)
	for ly := -2; ly < vh+2; ly++ {
		for lx := -2; lx < vw+2; lx++ {
			a.markSeen(ox+lx, oy+ly)
		}
	}
	tm, ox, oy := a.sample(vw, vh)
	players := []world.Player{{Name: name, X: a.wx, Y: a.wy, Color: "#FFC861", Facing: world.DirS}}
	img := game.RenderRGBA(nil, tm, players, name, 7,
		game.Camera{W: vw, H: vh}, game.Light{}, ox, oy, 32, false, game.DefaultStyle())
	game.DrawAreaTitle(img, a.Name(), 0)
	if label, ok := a.ClaimLabel(); ok {
		game.DrawClaimBanner(img, label)
	}
	_ = os.MkdirAll("../../buildshots", 0o755)
	f, err := os.Create("../../buildshots/claim.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote buildshots/claim.png — %s in %s", p.Kind, p.Settlement)
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
