package game

import (
	"bytes"
	"image"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/pixel"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// renderBenchImage produces one production-sized HD frame the way runHD does,
// so the encode/diff benchmarks operate on a realistic image.
func renderBenchImage(frame int) *image.RGBA {
	tm := bigMap(benchVW, benchVH)
	players := []world.Player{{Name: "you", X: benchVW / 2, Y: benchVH / 2, Color: "#FFC861", LastMoved: time.Now()}}
	cam := Camera{X: 0, Y: 0, W: benchVW, H: benchVH}
	light := Light{X: benchVW / 2, Y: benchVH / 2, Radius: 30}
	return RenderRGBA(nil, tm, players, "you", frame, cam, light, 0, 0, benchScale, false, DefaultStyle())
}

// Typical window: ~1000×600 px (e.g. a 100×30 terminal at 10×20 cells), well
// under the 1920×1200 cap. Cost scales with pixel area, so this brackets the
// low end against the maxed-out production size.
const (
	typVW = 38
	typVH = 23
)

func renderTypicalImage(frame int) *image.RGBA {
	tm := bigMap(typVW, typVH)
	players := []world.Player{{Name: "you", X: typVW / 2, Y: typVH / 2, Color: "#FFC861", LastMoved: time.Now()}}
	cam := Camera{X: 0, Y: 0, W: typVW, H: typVH}
	light := Light{X: typVW / 2, Y: typVH / 2, Radius: 20}
	return RenderRGBA(nil, tm, players, "you", frame, cam, light, 0, 0, benchScale, false, DefaultStyle())
}

// BenchmarkRenderRGBATypical / EncodeSixelTypical: the same hot path at a
// typical window size, for the smooth-end of the FPS range.
func BenchmarkRenderRGBATypical(b *testing.B) {
	tm := bigMap(typVW, typVH)
	players := []world.Player{{Name: "you", X: typVW / 2, Y: typVH / 2, Color: "#FFC861", LastMoved: time.Now()}}
	cam := Camera{X: 0, Y: 0, W: typVW, H: typVH}
	light := Light{X: typVW / 2, Y: typVH / 2, Radius: 20}
	st := DefaultStyle()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderRGBA(nil, tm, players, "you", i, cam, light, 0, 0, benchScale, false, st)
	}
}

func BenchmarkEncodeSixelTypical(b *testing.B) {
	img := renderTypicalImage(0)
	b.ReportAllocs()
	b.ResetTimer()
	var sent int
	for i := 0; i < b.N; i++ {
		out := pixel.EncodeSixel(img, false)
		sent = len(out)
	}
	b.ReportMetric(float64(sent), "payload-bytes")
}

// benchPlayers / benchLight build a single idle hero centered in the window.
func benchHero(px, py int) []world.Player {
	return []world.Player{{Name: "me", X: px, Y: py, Color: "#FFC861", LastMoved: time.Now().Add(-time.Hour)}}
}

// BenchmarkIncrementalStill measures the steady-state cost of a still camera
// (the hangout/idle case): only animated tiles (water, portals, campfires) are
// re-rasterized each frame. Compare against BenchmarkRenderRGBA (full every frame).
func BenchmarkIncrementalStill(b *testing.B) {
	orig := ui.Now
	ui.Now = func() time.Time { return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) }
	defer func() { ui.Now = orig }()
	style := DefaultStyle()
	vw, vh := benchVW, benchVH
	px, py := 100, 100
	ox, oy := px-vw/2, py-vh/2
	light := Light{X: px, Y: py, Radius: 30}
	players := benchHero(px, py)
	win := windowOf(ox, oy, vw, vh)
	var inc IncrementalRenderer
	inc.Render(win, players, "me", 0, light, ox, oy, benchScale, style, true) // prime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inc.Render(win, players, "me", i+1, light, ox, oy, benchScale, style, false)
	}
}

// sparseWindow is a realistic open-Wilds view: mostly static grass with a
// forest patch, one compact pond and a single campfire — so only a small,
// localized fraction of tiles animate (unlike the deliberately busy worldTile).
func sparseWindow(ox, oy, vw, vh int) *TileMap {
	tiles := make([][]Tile, vh)
	for ty := 0; ty < vh; ty++ {
		tiles[ty] = make([]Tile, vw)
		for tx := 0; tx < vw; tx++ {
			wx, wy := ox+tx, oy+ty
			t := Tile{Kind: TileFloor, Walkable: true, Tex: TexGrass, Ground: "#3A7D44"}
			if hashNoise(wx, wy, 0x55) > 0.85 {
				t.Tex, t.Ground, t.Prop, t.PropHex = TexForest, "#2E5E34", PropTree, "#2E5E34"
			}
			if absI(wx-105) <= 3 && absI(wy-103) <= 2 { // a small pond
				t = Tile{Kind: TileFloor, Walkable: true, Tex: TexWater, Ground: "#2E6BFF"}
			}
			if wx == 98 && wy == 98 { // one campfire
				t.Prop, t.PropHex = PropCampfire, "#FF8030"
			}
			tiles[ty][tx] = t
		}
	}
	return &TileMap{W: vw, H: vh, Tiles: tiles}
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// BenchmarkIncrementalStillSparse is the realistic hangout case: standing in open
// terrain, only a pond and a campfire animate. This is where the dirty-tile
// renderer pays off most.
func BenchmarkIncrementalStillSparse(b *testing.B) {
	orig := ui.Now
	ui.Now = func() time.Time { return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) }
	defer func() { ui.Now = orig }()
	style := DefaultStyle()
	vw, vh := benchVW, benchVH
	px, py := 100, 100
	ox, oy := px-vw/2, py-vh/2
	light := Light{X: px, Y: py, Radius: 30}
	players := benchHero(px, py)
	win := sparseWindow(ox, oy, vw, vh)
	var inc IncrementalRenderer
	inc.Render(win, players, "me", 0, light, ox, oy, benchScale, style, true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inc.Render(win, players, "me", i+1, light, ox, oy, benchScale, style, false)
	}
}

// BenchmarkIncrementalPan measures a walking camera (panning one tile/frame):
// the kept pixels are shifted and only the newly-revealed strip, band-crossings
// and animated tiles are re-rasterized.
func BenchmarkIncrementalPan(b *testing.B) {
	orig := ui.Now
	ui.Now = func() time.Time { return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) }
	defer func() { ui.Now = orig }()
	style := DefaultStyle()
	vw, vh := benchVW, benchVH
	px, py := 100, 100
	var inc IncrementalRenderer
	inc.Render(windowOf(px-vw/2, py-vh/2, vw, vh), benchHero(px, py), "me", 0,
		Light{X: px, Y: py, Radius: 30}, px-vw/2, py-vh/2, benchScale, style, true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		px++ // walk east
		ox, oy := px-vw/2, py-vh/2
		win := windowOf(ox, oy, vw, vh)
		players := benchHero(px, py)
		light := Light{X: px, Y: py, Radius: 30}
		b.StartTimer()
		inc.Render(win, players, "me", i+1, light, ox, oy, benchScale, style, false)
	}
}

// BenchmarkEncodeSixelFull measures sixel encoding of a whole production frame —
// the cost paid on every full repaint and every camera pan (walking).
func BenchmarkEncodeSixelFull(b *testing.B) {
	img := renderBenchImage(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pixel.EncodeSixel(img, false)
	}
}

// BenchmarkEncodeKittyFull measures kitty (zlib BestSpeed) encoding of a whole
// production frame.
func BenchmarkEncodeKittyFull(b *testing.B) {
	img := renderBenchImage(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pixel.EncodeKitty(img, 1, 0, 0)
	}
}

// BenchmarkDirtyDiff measures the cell-aligned dirty-rect diff over two
// production frames (the per-frame cost of deciding what changed).
func BenchmarkDirtyDiff(b *testing.B) {
	a := renderBenchImage(0)
	c := renderBenchImage(1) // a later animation frame: scattered glints/fireflies differ
	const cw, ch = 10, 20
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pixel.DirtyCellRects(a.Pix, c.Pix, benchVW*benchScale, benchVH*benchScale, cw, ch, 96, 0.5)
	}
}

// BenchmarkWriteFrameIdle is a static scene: full diff, nothing emitted — the
// floor cost a session pays per animation tick when nothing visible changed.
func BenchmarkWriteFrameIdle(b *testing.B) {
	img := renderBenchImage(0)
	fw := &pixel.FrameWriter{CellW: 10, CellH: 20}
	var buf bytes.Buffer
	fw.WriteFrame(&buf, img, true) // prime prev
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = fw.WriteFrame(&buf, img, false)
	}
}

// BenchmarkWriteFrameAnimDelta is a still camera with ambient animation — the
// walking-still case: only scattered cells change, so a few small sixel deltas
// go out. Reports bytes/op via the returned payload size.
func BenchmarkWriteFrameAnimDelta(b *testing.B) {
	fw := &pixel.FrameWriter{CellW: 10, CellH: 20}
	var buf bytes.Buffer
	fw.WriteFrame(&buf, renderBenchImage(0), true)
	imgs := []*image.RGBA{renderBenchImage(0), renderBenchImage(1)}
	b.ReportAllocs()
	b.ResetTimer()
	var sent int
	for i := 0; i < b.N; i++ {
		buf.Reset()
		sent = fw.WriteFrame(&buf, imgs[i&1], false)
	}
	b.ReportMetric(float64(sent), "payload-bytes")
}

// BenchmarkWriteFramePanFull is a moving camera (walking): the whole frame is
// dirty, so a full sixel repaint goes out. Reports the per-frame payload size.
func BenchmarkWriteFramePanFull(b *testing.B) {
	fw := &pixel.FrameWriter{CellW: 10, CellH: 20}
	var buf bytes.Buffer
	full := renderBenchImage(0)
	fw.WriteFrame(&buf, full, true)
	b.ReportAllocs()
	b.ResetTimer()
	var sent int
	for i := 0; i < b.N; i++ {
		buf.Reset()
		sent = fw.WriteFrame(&buf, full, true) // forceFull = camera pan
	}
	b.ReportMetric(float64(sent), "payload-bytes")
}
