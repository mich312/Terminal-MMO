package pixel

import (
	"fmt"
	"image"
	"io"
)

// FrameWriter streams a sequence of RGBA frames to a terminal as cell-aligned
// dirty-rect deltas (or full repaints), for either the kitty graphics protocol
// or sixel. It owns the previous-frame buffer so callers just hand it each new
// frame. The zero value is usable once Kitty/CellW/CellH are set; the first
// frame is always sent in full.
type FrameWriter struct {
	Kitty        bool // kitty graphics protocol (else sixel)
	CellW, CellH int  // terminal cell size in pixels, for snapping deltas
	Dither       bool // sixel ordered dithering (ignored for kitty)

	prev   []byte
	pw, ph int
}

// Reset forgets the previous frame so the next WriteFrame repaints in full —
// call it after clearing the screen (e.g. on a resize).
func (f *FrameWriter) Reset() { f.prev = nil }

// WriteFrame transmits img to w. It sends a full repaint when forceFull is set,
// on the first frame or a size change, or when the changed area exceeds half the
// frame (a full frame is then cheaper than the delta overhead); a frame
// identical to the last sends nothing. It returns the number of image-payload
// bytes written (excluding the few cursor-control bytes), for bandwidth
// accounting.
func (f *FrameWriter) WriteFrame(w io.Writer, img *image.RGBA, forceFull bool) (sent int) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	doFull := forceFull || f.prev == nil || W != f.pw || H != f.ph || f.CellW <= 0 || f.CellH <= 0

	if !doFull {
		switch r, changed := DirtyRect(f.prev, img.Pix, W, H); {
		case !changed:
			// static frame — send nothing
		case r.Dx()*r.Dy() > W*H/2:
			doFull = true
		default:
			sr := SnapToCells(r, f.CellW, f.CellH, W, H)
			sub := f.encode(Crop(img, sr))
			writeAt(w, sub, sr.Min.X/f.CellW, sr.Min.Y/f.CellH)
			sent += len(sub)
		}
	}
	if doFull {
		sent += f.writeFull(w, img)
	}

	f.prev = append(f.prev[:0], img.Pix...)
	f.pw, f.ph = W, H
	return sent
}

func (f *FrameWriter) encode(img *image.RGBA) []byte {
	if f.Kitty {
		return EncodeKitty(img, 0, 0)
	}
	return EncodeSixel(img, f.Dither)
}

// writeFull repaints the whole frame at the home cell. For kitty it first
// reclaims prior placements so they don't accumulate in the terminal's memory.
func (f *FrameWriter) writeFull(w io.Writer, img *image.RGBA) int {
	io.WriteString(w, "\x1b7\x1b[H")
	var full []byte
	if f.Kitty {
		io.WriteString(w, "\x1b_Ga=d\x1b\\")
		full = EncodeKitty(img, 0, 0)
	} else {
		full = EncodeSixel(img, f.Dither)
	}
	w.Write(full)
	io.WriteString(w, "\x1b8")
	return len(full)
}

// writeAt positions the cursor at a text cell (0-based col,row) and writes the
// payload, bracketed by cursor save/restore so the placement never scrolls.
func writeAt(w io.Writer, payload []byte, col, row int) {
	io.WriteString(w, "\x1b7")
	fmt.Fprintf(w, "\x1b[%d;%dH", row+1, col+1)
	w.Write(payload)
	io.WriteString(w, "\x1b8")
}
