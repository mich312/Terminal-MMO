package wilds

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// findWell scans a region for a settlement centre (a well glyph 'W'), returning
// the first whose "fenced" status matches wantFence.
func findWell(g *worldgen.Generator, wantFence bool) (int, int, bool) {
	const span = 400
	for cy := -span; cy <= span; cy++ {
		for cx := -span; cx <= span; cx++ {
			if g.At(cx, cy).Glyph != 'W' {
				continue
			}
			fenced := false
			for dy := -16; dy <= 16 && !fenced; dy++ {
				for dx := -16; dx <= 16; dx++ {
					if g.At(cx+dx, cy+dy).Glyph == '=' {
						fenced = true
						break
					}
				}
			}
			if fenced == wantFence {
				return cx, cy, true
			}
		}
	}
	return 0, 0, false
}

// renderSettlementPNG rasterizes a full-visibility window around (cx,cy) into a
// PNG — the same HD pixel scene the live sixel/kitty client draws.
func renderSettlementPNG(t *testing.T, cx, cy, win, scale int, path string) {
	g := worldgen.New(worldSeed)
	ox, oy := cx-win/2, cy-win/2
	tiles := make([][]game.Tile, win)
	for ly := 0; ly < win; ly++ {
		row := make([]game.Tile, win)
		for lx := 0; lx < win; lx++ {
			row[lx] = CellTile(g.At(ox+lx, oy+ly))
		}
		tiles[ly] = row
	}
	tm := &game.TileMap{W: win, H: win, Tiles: tiles}
	cam := game.Camera{X: 0, Y: 0, W: win, H: win}
	// Match the live client: flat tiles (smooth=false), uniform light.
	img := game.RenderRGBA(ui.Default, tm, []world.Player{}, "", 0, cam, game.Light{}, ox, oy, scale, false, nil)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%dx%d px) for settlement at (%d,%d)", path, win*scale, win*scale, cx, cy)
}

// TestHDPreview renders a village and a hamlet to PNGs under /tmp so the HD look
// can be eyeballed. Run with:
//
//	go test ./internal/areas/wilds -run HDPreview -v
func TestHDPreview(t *testing.T) {
	// Pin the clock to midday so the day/night tint doesn't darken the preview.
	old := ui.Now
	ui.Now = func() time.Time { return time.Date(2026, 6, 18, 13, 0, 0, 0, time.UTC) }
	defer func() { ui.Now = old }()

	g := worldgen.New(worldSeed)
	if cx, cy, ok := findWell(g, true); ok {
		renderSettlementPNG(t, cx, cy, 34, 26, "/tmp/village_hd.png")
	} else {
		t.Error("no fenced village found")
	}
	if cx, cy, ok := findWell(g, false); ok {
		renderSettlementPNG(t, cx, cy, 28, 26, "/tmp/hamlet_hd.png")
	} else {
		t.Error("no hamlet found")
	}
}
