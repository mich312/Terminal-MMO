package pixel

import (
	"image"
	"image/color"
	"testing"
)

// pokeCell sets one pixel inside cell (col,row) so that whole cell goes dirty.
func pokeCell(img *image.RGBA, cw, ch, col, row int) {
	o := img.PixOffset(col*cw, row*ch)
	img.Pix[o] ^= 0xFF
}

// rectsCoverChangedPixels asserts every differing pixel falls inside some rect.
func rectsCoverChangedPixels(t *testing.T, prev, cur *image.RGBA, rects []image.Rectangle) {
	t.Helper()
	w, h := cur.Bounds().Dx(), cur.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := cur.PixOffset(x, y)
			if prev.Pix[o] == cur.Pix[o] && prev.Pix[o+1] == cur.Pix[o+1] &&
				prev.Pix[o+2] == cur.Pix[o+2] && prev.Pix[o+3] == cur.Pix[o+3] {
				continue
			}
			in := false
			for _, r := range rects {
				if image.Pt(x, y).In(r) {
					in = true
					break
				}
			}
			if !in {
				t.Fatalf("changed pixel (%d,%d) not covered by any delta rect", x, y)
			}
		}
	}
}

// Two far-apart changes must become two compact rectangles — not one bounding
// box spanning the gap between them (the whole-frame-repaint bug).
func TestDirtyCellRectsSplitsScattered(t *testing.T) {
	const cw, ch = 4, 4
	w, h := 16*cw, 12*ch
	prev := solid(w, h, color.RGBA{10, 20, 30, 255})
	cur := solid(w, h, color.RGBA{10, 20, 30, 255})
	pokeCell(cur, cw, ch, 0, 0)   // top-left
	pokeCell(cur, cw, ch, 15, 11) // bottom-right

	rects, changed := DirtyCellRects(prev.Pix, cur.Pix, w, h, cw, ch, 96, 0.5)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if len(rects) != 2 {
		t.Fatalf("got %d rects, want 2 (scattered changes must not merge into one box)", len(rects))
	}
	rectsCoverChangedPixels(t, prev, cur, rects)
	area := 0
	for _, r := range rects {
		area += r.Dx() * r.Dy()
	}
	if frac := float64(area) / float64(w*h); frac > 0.05 {
		t.Fatalf("delta area is %.0f%% of frame; should be tiny", frac*100)
	}
}

// A vertical band of changed cells coalesces into a single tall rectangle.
func TestDirtyCellRectsMergesVertical(t *testing.T) {
	const cw, ch = 4, 4
	w, h := 16*cw, 12*ch
	prev := solid(w, h, color.RGBA{0, 0, 0, 255})
	cur := solid(w, h, color.RGBA{0, 0, 0, 255})
	for row := 0; row < 12; row++ {
		pokeCell(cur, cw, ch, 5, row)
	}
	rects, changed := DirtyCellRects(prev.Pix, cur.Pix, w, h, cw, ch, 96, 0.5)
	if !changed || len(rects) != 1 {
		t.Fatalf("vertical band: got %d rects, want 1", len(rects))
	}
	if r := rects[0]; r.Dy() != h {
		t.Fatalf("merged rect height %d, want full %d", r.Dy(), h)
	}
	rectsCoverChangedPixels(t, prev, cur, rects)
}

// Identical frames report no change.
func TestDirtyCellRectsNoChange(t *testing.T) {
	w, h := 32, 32
	a := solid(w, h, color.RGBA{7, 7, 7, 255})
	if rects, changed := DirtyCellRects(a.Pix, a.Pix, w, h, 4, 4, 96, 0.5); changed || rects != nil {
		t.Fatalf("identical frames: changed=%v rects=%v", changed, rects)
	}
}

// A change covering more than maxFrac of the frame falls back to a full repaint
// (rects=nil, changed=true).
func TestDirtyCellRectsLargeChangeFallsBack(t *testing.T) {
	const cw, ch = 4, 4
	w, h := 16*cw, 12*ch
	prev := solid(w, h, color.RGBA{0, 0, 0, 255})
	cur := solid(w, h, color.RGBA{255, 255, 255, 255}) // every pixel differs
	rects, changed := DirtyCellRects(prev.Pix, cur.Pix, w, h, cw, ch, 96, 0.5)
	if !changed || rects != nil {
		t.Fatalf("whole-frame change should force full repaint: rects=%v changed=%v", rects, changed)
	}
}

// Too many disjoint changes exceed maxRects and fall back to a full repaint.
func TestDirtyCellRectsTooFragmentedFallsBack(t *testing.T) {
	const cw, ch = 2, 2
	cols, rows := 40, 40
	w, h := cols*cw, rows*ch
	prev := solid(w, h, color.RGBA{0, 0, 0, 255})
	cur := solid(w, h, color.RGBA{0, 0, 0, 255})
	// Poke a grid of isolated cells spaced beyond DirtyCellGap so none merge —
	// far more than maxRects islands, forcing a full-repaint fallback.
	for row := 0; row < rows; row += 4 {
		for col := 0; col < cols; col += 4 {
			pokeCell(cur, cw, ch, col, row)
		}
	}
	if rects, changed := DirtyCellRects(prev.Pix, cur.Pix, w, h, cw, ch, 96, 0.9); !changed || rects != nil {
		t.Fatalf("fragmented change should fall back: rects=%d changed=%v", len(rects), changed)
	}
}
