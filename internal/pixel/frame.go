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

	// kitty double-buffering: every transmitted image (delta or full) gets a
	// unique id from nextID and is recorded in liveIDs. A full repaint places the
	// new frame on top, then deletes everything in liveIDs — so the old images are
	// reclaimed only after the new one is on screen, with no blank gap between.
	nextID  int
	liveIDs []int
}

// Reset forgets the previous frame so the next WriteFrame repaints in full —
// call it after clearing the screen (e.g. on a resize). The live kitty ids are
// kept so the next full repaint still deletes the now-stale images.
func (f *FrameWriter) Reset() { f.prev = nil }

// allocID hands out the next kitty image id.
func (f *FrameWriter) allocID() int { f.nextID++; return f.nextID }

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
			var sub []byte
			if f.Kitty {
				id := f.allocID()
				sub = EncodeKitty(Crop(img, sr), id, 0, 0)
				f.liveIDs = append(f.liveIDs, id)
			} else {
				sub = EncodeSixel(Crop(img, sr), f.Dither)
			}
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

// writeFull repaints the whole frame at the home cell. For kitty it double-
// buffers: the new frame is transmitted and displayed first (a fresh id, drawn on
// top of everything), and only then are all previously-live images deleted by id.
// The old delete-all-then-redraw left the window blank between the wipe and the
// new frame landing — a flash every full repaint, worse the larger the frame.
func (f *FrameWriter) writeFull(w io.Writer, img *image.RGBA) int {
	io.WriteString(w, "\x1b7\x1b[H")
	if f.Kitty {
		id := f.allocID()
		full := EncodeKitty(img, id, 0, 0)
		w.Write(full)
		io.WriteString(w, "\x1b8")
		for _, old := range f.liveIDs {
			fmt.Fprintf(w, "\x1b_Ga=d,d=I,i=%d\x1b\\", old)
		}
		f.liveIDs = append(f.liveIDs[:0], id)
		return len(full)
	}
	full := EncodeSixel(img, f.Dither)
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
