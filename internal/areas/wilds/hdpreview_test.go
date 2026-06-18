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

// findWells scans a region for settlement centres (well glyph 'W'), returning up
// to n of them (deduplicated by proximity).
func findWells(g *worldgen.Generator, n int) [][2]int {
	const span = 420
	var out [][2]int
	for cy := -span; cy <= span && len(out) < n; cy++ {
		for cx := -span; cx <= span && len(out) < n; cx++ {
			if g.At(cx, cy).Glyph != 'W' {
				continue
			}
			near := false
			for _, p := range out {
				if abs(p[0]-cx) < 60 && abs(p[1]-cy) < 60 {
					near = true
				}
			}
			if !near {
				out = append(out, [2]int{cx, cy})
			}
		}
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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
			wx, wy := ox+lx, oy+ly
			cell := g.At(wx, wy)
			t := CellTile(cell)
			// Overlay world loot the way the live area does, so crops show.
			if it, ok := itemAt(cell, wx, wy); ok {
				if it.ID == "grain" {
					t.Prop, t.PropHex, t.Tex, t.Ground = game.PropCrop, it.Hex, game.TexField, "#86974A"
				} else {
					t.Prop, t.PropHex, t.Ground = game.PropGem, it.Hex, groundColor(cell.Biome)
				}
			}
			row[lx] = t
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
	wells := findWells(g, 3)
	if len(wells) == 0 {
		t.Fatal("no settlements found")
	}
	renderSettlementPNG(t, wells[0][0], wells[0][1], 40, 30, "/tmp/village_closeup_hd.png")
	names := []string{"/tmp/village_hd.png", "/tmp/village2_hd.png", "/tmp/village3_hd.png"}
	for i, w := range wells {
		renderSettlementPNG(t, w[0], w[1], 60, 20, names[i])
	}
}
