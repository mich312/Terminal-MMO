package game

import (
	"bytes"
	"image"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// worldTile is a world-coordinate-deterministic tile (mixed biomes, water,
// forest, scattered campfires, a fixed portal), so a window sampled at any origin
// is consistent as the camera pans — the property the incremental renderer relies
// on. Mirrors perfbench's bigMap content but as a pure function of (wx,wy).
func worldTile(wx, wy int) Tile {
	t := Tile{Kind: TileFloor, Walkable: true}
	switch r := hashNoise(wx, wy, 0x1); {
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
	if hashNoise(wx, wy, 0x99) > 0.95 {
		t.Prop, t.PropHex = PropCampfire, "#FF8030" // emissive: exercises glow overhang
	}
	if wx == 3 && wy == 2 {
		t.Prop, t.PropHex = PropPortal, "#7DF0FF" // animated swirl
	}
	return t
}

func windowOf(ox, oy, vw, vh int) *TileMap {
	tiles := make([][]Tile, vh)
	for ty := 0; ty < vh; ty++ {
		tiles[ty] = make([]Tile, vw)
		for tx := 0; tx < vw; tx++ {
			tiles[ty][tx] = worldTile(ox+tx, oy+ty)
		}
	}
	return &TileMap{W: vw, H: vh, Tiles: tiles}
}

// TestIncrementalMatchesFull is the renderer's correctness contract: for a
// scripted path of stills, pans, diagonal pans and avatar movement — at day,
// dusk and night (different animation regimes) — the incremental buffer must be
// byte-identical to a fresh full RenderRGBA every frame. If it ever diverges the
// player would see stale pixels, so this guards the whole optimization.
func TestIncrementalMatchesFull(t *testing.T) {
	orig := ui.Now
	defer func() { ui.Now = orig }()
	style := DefaultStyle()
	const vw, vh, scale = 22, 16, 18
	idle := time.Now().Add(-time.Hour) // far past → deterministic idle walk frame

	times := map[string]time.Time{
		"day":   time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC),
		"dusk":  time.Date(2026, 6, 16, 19, 30, 0, 0, time.UTC),
		"night": time.Date(2026, 6, 16, 23, 0, 0, 0, time.UTC),
	}

	// Scripted steps: each advances the frame and moves the self/npc avatars.
	type step struct {
		dpx, dpy, dnx, dny int
		forceFull          bool
	}
	steps := []step{
		{}, {}, {}, // still: animation only
		{dpx: 1}, {dpx: 1}, {dpx: 1, forceFull: true}, // walk east (full repaint mid-run)
		{dpx: 1, dpy: 1}, {dpx: 1, dpy: 1}, {dpx: -1, dpy: 1}, // diagonal pans
		{dnx: 1}, {dnx: 1, dny: 1}, {dnx: -1}, // npc moves, camera still
		{}, {forceFull: true}, {dpx: -1, dpy: -1}, // resync then pan back
	}

	for name, clock := range times {
		t.Run(name, func(t *testing.T) {
			ui.Now = func() time.Time { return clock }
			var inc IncrementalRenderer
			px, py := 8, 6 // self world position; camera centers on it
			nx, ny := 11, 9
			for i, s := range steps {
				px, py, nx, ny = px+s.dpx, py+s.dpy, nx+s.dnx, ny+s.dny
				frame := i
				ox, oy := px-vw/2, py-vh/2
				light := Light{X: px, Y: py, Radius: 18}
				players := []world.Player{
					{Name: "me", X: px, Y: py, Color: "#FFC861", Facing: world.DirS, LastMoved: idle},
					{Name: "bob", X: nx, Y: ny, Color: "#7DF0FF", Facing: world.DirE, LastMoved: idle},
				}
				win := windowOf(ox, oy, vw, vh)
				want := RenderRGBA(nil, win, players, "me", frame, Camera{W: vw, H: vh}, light, ox, oy, scale, false, style)
				got := inc.Render(win, players, "me", frame, light, ox, oy, scale, style, s.forceFull)
				if !bytes.Equal(got.Pix, want.Pix) {
					t.Fatalf("step %d (frame %d, origin %d,%d): incremental frame differs from full render at %s — %d px differ",
						i, frame, ox, oy, name, countDiff(got, want))
				}
			}
		})
	}
}

func countDiff(a, b *image.RGBA) int {
	n := 0
	for i := 0; i+3 < len(a.Pix) && i+3 < len(b.Pix); i += 4 {
		if a.Pix[i] != b.Pix[i] || a.Pix[i+1] != b.Pix[i+1] || a.Pix[i+2] != b.Pix[i+2] {
			n++
		}
	}
	return n
}
