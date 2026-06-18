package wilds

import (
	"image/png"
	"math"
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

// findWorksite locates a worksite tile (quarry floor, lumber stump, or jetty)
// near a village centre, for a close-up of the harvest sprites.
func findWorksite(g *worldgen.Generator, cx, cy int) (int, int, bool) {
	for r := 8; r < 32; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) != r && abs(dy) != r {
					continue
				}
				c := g.At(cx+dx, cy+dy)
				if (c.Biome == worldgen.Mountain && c.Glyph == '·') ||
					(c.Biome == worldgen.Path && c.Glyph == 'u') {
					return cx + dx, cy + dy, true
				}
			}
		}
	}
	return 0, 0, false
}

// findTown scans for a stone-walled city by its cobbled market square (unique to
// cities) and returns its centre, preferring one bordered by terrain (forest or
// water) so the way the footprint conforms to the land is visible.
func findTown(g *worldgen.Generator) (int, int, bool) {
	const span = 800
	// near reports terrain features in the surrounding ring, to favour a city
	// whose footprint has to bend around woods or water.
	near := func(cx, cy int) int {
		feat := 0
		for a := 0; a < 360; a += 6 {
			rad := float64(a) * math.Pi / 180
			for _, r := range []int{14, 20, 26} {
				dx := int(float64(r) * math.Cos(rad))
				dy := int(float64(r) * math.Sin(rad))
				switch g.At(cx+dx, cy+dy).Biome {
				case worldgen.Forest, worldgen.Water, worldgen.Deep, worldgen.Hill:
					feat++
				}
			}
		}
		return feat
	}
	best := [2]int{}
	bestFeat, found := 0, false
	seen := map[[2]int]bool{}
	for cy := -span; cy <= span; cy++ {
		for cx := -span; cx <= span; cx++ {
			c := g.At(cx, cy)
			if c.Glyph != '·' || c.Color != "#A89B82" {
				continue
			}
			key := [2]int{cx / 60, cy / 60}
			if seen[key] {
				continue
			}
			seen[key] = true
			if f := near(cx, cy); f > bestFeat {
				best, bestFeat, found = [2]int{cx, cy}, f, true
			}
			if bestFeat > 40 {
				return best[0], best[1], true
			}
		}
	}
	return best[0], best[1], found
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
				switch it.ID {
				case "grain":
					t.Prop, t.PropHex, t.Tex, t.Ground = game.PropCrop, it.Hex, game.TexField, "#86974A"
				case "stone":
					t.Prop, t.PropHex = game.PropStone, it.Hex
				case "wood":
					t.Prop, t.PropHex = game.PropLog, it.Hex
				case "fish":
					t.Prop, t.PropHex = game.PropFish, it.Hex
				default:
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
	renderSettlementPNG(t, wells[0][0], wells[0][1], 56, 22, "/tmp/village_closeup_hd.png")
	// Find and render a stone-walled town (well near a '#' stone wall).
	if tx, ty, ok := findTown(g); ok {
		renderSettlementPNG(t, tx, ty, 130, 9, "/tmp/town_hd.png")
		renderSettlementPNG(t, tx, ty, 70, 17, "/tmp/town_closeup_hd.png")
	} else {
		t.Log("no town found in range")
	}
	// A tight view on the first worksite found, to check the harvest sprites.
	if qx, qy, ok := findWorksite(g, wells[0][0], wells[0][1]); ok {
		renderSettlementPNG(t, qx, qy, 18, 48, "/tmp/worksite_hd.png")
	}
	names := []string{"/tmp/village_hd.png", "/tmp/village2_hd.png", "/tmp/village3_hd.png"}
	for i, w := range wells {
		renderSettlementPNG(t, w[0], w[1], 96, 13, names[i])
	}
}
