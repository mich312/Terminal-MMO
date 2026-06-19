package game

import (
	"bytes"
	"image"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/pixel"
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
