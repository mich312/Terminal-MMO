package pixel

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// The first frame is always a full repaint, regardless of forceFull.
func TestFrameWriterFirstFrameIsFull(t *testing.T) {
	for _, kitty := range []bool{true, false} {
		fw := &FrameWriter{Kitty: kitty, CellW: 2, CellH: 2}
		var buf bytes.Buffer
		sent := fw.WriteFrame(&buf, solid(8, 6, color.RGBA{10, 20, 30, 255}), false)
		if sent == 0 || buf.Len() == 0 {
			t.Fatalf("kitty=%v: first frame sent nothing (sent=%d, bytes=%d)", kitty, sent, buf.Len())
		}
	}
}

// An unchanged, non-forced frame transmits nothing at all.
func TestFrameWriterStaticSendsNothing(t *testing.T) {
	fw := &FrameWriter{Kitty: true, CellW: 2, CellH: 2}
	img := solid(8, 6, color.RGBA{10, 20, 30, 255})
	fw.WriteFrame(&bytes.Buffer{}, img, false) // prime

	var buf bytes.Buffer
	if sent := fw.WriteFrame(&buf, solid(8, 6, color.RGBA{10, 20, 30, 255}), false); sent != 0 {
		t.Fatalf("static frame reported %d bytes sent", sent)
	}
	if buf.Len() != 0 {
		t.Fatalf("static frame wrote %d bytes, want 0", buf.Len())
	}
}

// A small localized change emits a cursor-positioned delta, not a full repaint.
func TestFrameWriterSmallChangeIsDelta(t *testing.T) {
	fw := &FrameWriter{Kitty: true, CellW: 2, CellH: 2}
	base := solid(16, 12, color.RGBA{10, 20, 30, 255})
	fw.WriteFrame(&bytes.Buffer{}, base, false) // prime

	next := solid(16, 12, color.RGBA{10, 20, 30, 255})
	o := next.PixOffset(0, 0) // poke one corner pixel
	next.Pix[o] = 200

	var buf bytes.Buffer
	fw.WriteFrame(&buf, next, false)
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("\x1b7")) || !bytes.Contains(buf.Bytes(), []byte("\x1b[1;1H")) {
		t.Fatalf("delta frame missing cursor-positioned write: %q", out)
	}
	// A full kitty repaint reclaims placements (a=d); a delta must not.
	if bytes.Contains(buf.Bytes(), []byte("a=d")) {
		t.Fatalf("delta frame unexpectedly did a full repaint: %q", out)
	}
}

// A size change forces a full repaint even when not explicitly requested.
func TestFrameWriterResizeForcesFull(t *testing.T) {
	fw := &FrameWriter{Kitty: true, CellW: 2, CellH: 2}
	fw.WriteFrame(&bytes.Buffer{}, solid(8, 6, color.RGBA{1, 2, 3, 255}), false)

	var buf bytes.Buffer
	fw.WriteFrame(&buf, solid(10, 8, color.RGBA{1, 2, 3, 255}), false)
	if !bytes.Contains(buf.Bytes(), []byte("a=d")) {
		t.Fatalf("resize did not trigger a full repaint: %q", buf.String())
	}
}
