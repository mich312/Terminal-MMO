package wilds

import (
	"image/png"
	"os"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// TestBuildPreview renders the real Wilds sample() path with player-built
// structures and an active build ghost, so the placements layer can be reviewed
// as an actual HD frame. It writes a PNG only when DURST_PREVIEW=1 (otherwise it
// just exercises the overlay code without panicking).
func TestBuildPreview(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("steurer")
	st := store.Open(t.TempDir() + "/w.db")
	ctx := &game.Ctx{World: w, Store: st, Name: name, Theme: ui.Default, Inventory: map[string]int{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	const vw, vh = 24, 15
	tm, ox, oy := a.sample(vw, vh) // sample once to fix the window origin
	// Reveal the whole window so nothing is fog, then place structures on the
	// first buildable cells we find (skipping the player's own footprint).
	for ly := 0; ly < vh; ly++ {
		for lx := 0; lx < vw; lx++ {
			a.markSeen(ox+lx, oy+ly)
		}
	}

	placed, kinds := 0, []string{"fence", "fence", "fence", "workbench", "chest", "lamppost"}
	for ly := 2; ly < vh-2 && placed < len(kinds); ly++ {
		for lx := 3; lx < vw-3 && placed < len(kinds); lx += 2 {
			x, y := ox+lx, oy+ly
			if a.canBuildAt(x, y) {
				w.Place("wilds", world.Placement{X: x, Y: y, Kind: kinds[placed], Owner: name})
				placed++
			}
		}
	}

	// Turn on build mode with a ghost on a free cell next to the player.
	a.building, a.buildSel = true, 0
	a.bx, a.by = a.wx+2, a.wy

	tm, ox, oy = a.sample(vw, vh)
	if placed == 0 {
		t.Skip("no buildable cell near spawn this seed; overlay code still exercised")
	}
	if os.Getenv("DURST_PREVIEW") != "1" {
		return
	}

	players := []world.Player{{Name: name, X: a.wx, Y: a.wy, Color: "#FFC861", Facing: world.DirS}}
	img := game.RenderRGBA(nil, tm, players, name, 7,
		game.Camera{W: vw, H: vh}, game.Light{}, ox, oy, 32, false, game.DefaultStyle())
	_ = os.MkdirAll("../../buildshots", 0o755)
	f, err := os.Create("../../buildshots/real-build.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote buildshots/real-build.png (%d structures placed)", placed)
}
