package game

import (
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// bigMap builds a realistic wilds-sized window: mixed biomes, water, forest and
// scattered props — the kind of scene the HD renderer actually draws — so the
// render benchmark exercises the seam-dithering and glow paths, not a flat field.
func bigMap(w, h int) *TileMap {
	tiles := make([][]Tile, h)
	for y := 0; y < h; y++ {
		tiles[y] = make([]Tile, w)
		for x := 0; x < w; x++ {
			t := Tile{Kind: TileFloor, Walkable: true}
			switch r := hashNoise(x, y, 0x1); {
			case r < 0.30:
				t.Tex, t.Ground = TexGrass, "#3A7D44"
			case r < 0.45:
				t.Tex, t.Ground, t.Prop, t.PropHex = TexForest, "#2E5E34", PropTree, "#2E5E34"
			case r < 0.60:
				t.Tex, t.Ground = TexWater, "#2E6BFF"
			case r < 0.72:
				t.Tex, t.Ground = TexSand, "#D9C58B"
			case r < 0.85:
				t.Tex, t.Ground = TexSwamp, "#4A5A3A"
			default:
				t.Tex, t.Ground = TexRock, "#7A7A82"
			}
			if hashNoise(x, y, 0x99) > 0.97 {
				t.Prop, t.PropHex = PropCampfire, "#FF8030"
			}
			tiles[y][x] = t
		}
	}
	tiles[h/2][w/2].Prop, tiles[h/2][w/2].PropHex = PropPortal, "#7DF0FF"
	return &TileMap{W: w, H: h, Tiles: tiles}
}

// Production HD dims: hdScale=26 with a viewport capped near 1920×1200 px.
const (
	benchScale = 26
	benchVW    = 73
	benchVH    = 46
)

// BenchmarkRenderRGBA guards the HD render hot path at production size. The
// per-pixel seam dither once made this ~2s/frame; keep an eye on regressions.
func BenchmarkRenderRGBA(b *testing.B) {
	tm := bigMap(benchVW, benchVH)
	players := []world.Player{{Name: "you", X: benchVW / 2, Y: benchVH / 2, Color: "#FFC861", LastMoved: time.Now()}}
	cam := Camera{X: 0, Y: 0, W: benchVW, H: benchVH}
	light := Light{X: benchVW / 2, Y: benchVH / 2, Radius: 30}
	st := DefaultStyle()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderRGBA(nil, tm, players, "you", i, cam, light, 0, 0, benchScale, false, st)
	}
}
