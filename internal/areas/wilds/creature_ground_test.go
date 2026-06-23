package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// A creature standing on a tile must not turn that tile's HD surface into the
// species color. Plain ground carries no explicit Ground, so the HD renderer
// falls back to Tile.Color for its surface — and the glyph client needs
// Tile.Color set to the species hue. The area must therefore pin the ground
// color before recoloring, or the animal renders inside a colored box and the
// hue bleeds into neighbouring tiles' seam dither as it moves. (Regression for
// the "animals have a background color" / "terrain glitches near animals" bugs.)
func TestCreatureKeepsBiomeGround(t *testing.T) {
	g := worldgen.New(worldSeed)

	// Find a plain walkable ground cell — one CellTile leaves Ground empty, which
	// is exactly the case the bug needed.
	var cx, cy int
	var found bool
	for r := 0; r < 200 && !found; r++ {
		for d := -r; d <= r && !found; d++ {
			for _, p := range [][2]int{{r, d}, {-r, d}, {d, r}, {d, -r}} {
				cell := g.At(p[0], p[1])
				ct := CellTile(cell)
				if cell.Walkable && !cell.Object && cell.Portal == "" && ct.Ground == "" && ct.Color != "" {
					cx, cy, found = p[0], p[1], true
					break
				}
			}
		}
	}
	if !found {
		t.Fatal("no plain ground cell found to test on")
	}
	biomeGround := CellTile(g.At(cx, cy)).Color

	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/w.db"), Name: name, Theme: ui.Default}

	a := &area{ctx: ctx, gen: g, discovered: map[[2]int]uint64{}, dirty: map[[2]int]bool{}}
	a.wx, a.wy = cx, cy
	for dy := -12; dy <= 12; dy++ {
		for dx := -12; dx <= 12; dx++ {
			a.markSeen(cx+dx, cy+dy)
		}
	}

	sp, ok := game.SpeciesByKind("rabbit")
	if !ok {
		t.Fatal("rabbit species missing")
	}
	w.SpawnCreature(world.Creature{ID: "c1", Kind: "rabbit", Area: "wilds", X: cx, Y: cy})

	tm, ox, oy := a.sample(31, 31)
	tile := tm.Tiles[cy-oy][cx-ox]

	// The glyph client still gets the species letter…
	if tile.Color != sp.Hex {
		t.Errorf("glyph color = %q, want species %q", tile.Color, sp.Hex)
	}
	// …but the HD surface must stay the biome ground, never the species hue.
	if tile.Ground == "" {
		t.Fatal("creature tile has empty Ground: HD would fall back to the species color as the surface")
	}
	if tile.Ground == sp.Hex {
		t.Errorf("creature tile Ground = species hue %q (the colored-box bug)", sp.Hex)
	}
	if tile.Ground != biomeGround {
		t.Errorf("creature tile Ground = %q, want biome ground %q", tile.Ground, biomeGround)
	}
}
