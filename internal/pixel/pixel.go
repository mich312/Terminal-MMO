// Package pixel encodes RGBA frames for terminal graphics protocols — the
// kitty graphics protocol and sixel — and supports delta updates (dirty-rect
// diff + crop) so callers can re-send only the changed region. Driven by the
// live HD SSH renderer (cmd/durstworld/hd.go).
package pixel

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
)

// EncodeKitty transmits img as RGBA (f=32), zlib-compressed (o=z), base64'd and
// chunked at 4096 bytes per APC escape, displayed at the cursor without moving
// it (a=T, C=1). q=2 suppresses the terminal's per-image acknowledgements so they
// don't land in the input stream. When id > 0 the image is tagged with that id
// (i=) so the caller can later delete it precisely — the basis for flicker-free
// double-buffering. When cols/rows > 0 the image is display-scaled to fill that
// many text cells (c=,r=) — the basis for a fixed internal render resolution.
func EncodeKitty(img *image.RGBA, id, cols, rows int) []byte {
	var zb bytes.Buffer
	// BestSpeed (level 1): a full frame re-compresses on every camera scroll, so
	// compression time is on the hot path for movement. Flat pixel-art runs still
	// pack well at level 1, and it's several times faster than the default level.
	zw, _ := zlib.NewWriterLevel(&zb, zlib.BestSpeed)
	zw.Write(img.Pix)
	zw.Close()

	b64 := base64.StdEncoding.EncodeToString(zb.Bytes())
	bounds := img.Bounds()
	var buf bytes.Buffer
	const chunk = 4096
	first := true
	for len(b64) > 0 {
		n := min(chunk, len(b64))
		part := b64[:n]
		b64 = b64[n:]
		more := 0
		if len(b64) > 0 {
			more = 1
		}
		buf.WriteString("\x1b_G")
		if first {
			fmt.Fprintf(&buf, "f=32,o=z,s=%d,v=%d,a=T,C=1,q=2,", bounds.Dx(), bounds.Dy())
			if id > 0 {
				fmt.Fprintf(&buf, "i=%d,", id)
			}
			if cols > 0 && rows > 0 {
				fmt.Fprintf(&buf, "c=%d,r=%d,", cols, rows)
			}
			first = false
		}
		fmt.Fprintf(&buf, "m=%d;%s\x1b\\", more, part)
	}
	return buf.Bytes()
}

var bayer4 = [4][4]int{
	{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5},
}

// EncodeSixel quantizes img to a 6×6×6 (216) color cube and emits sixel bands.
// dither applies ordered (Bayer) dithering to soften gradient banding — keep it
// ON for smooth shading and OFF for flat tiles, where the per-pixel noise would
// destroy the long runs sixel RLE relies on (it bloats a flat frame ~10×).
func EncodeSixel(img *image.RGBA, dither bool) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	idx := make([]uint8, w*h)
	used := make([]bool, 216)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := img.PixOffset(x, y)
			p := q6(img.Pix[o], x, y, dither)*36 + q6(img.Pix[o+1], x, y, dither)*6 + q6(img.Pix[o+2], x, y, dither)
			idx[y*w+x] = uint8(p)
			used[p] = true
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "\x1bPq\"1;1;%d;%d", w, h)
	for p := 0; p < 216; p++ {
		if !used[p] {
			continue
		}
		fmt.Fprintf(&buf, "#%d;2;%d;%d;%d", p, ((p/36)%6)*20, ((p/6)%6)*20, (p%6)*20)
	}

	for band := 0; band < h; band += 6 {
		rows := min(6, h-band)
		present := map[uint8]bool{}
		for r := 0; r < rows; r++ {
			base := (band + r) * w
			for x := 0; x < w; x++ {
				present[idx[base+x]] = true
			}
		}
		for c := range present {
			fmt.Fprintf(&buf, "#%d", c)
			var prev byte
			var run int
			flush := func() {
				if run == 0 {
					return
				}
				if run > 3 {
					fmt.Fprintf(&buf, "!%d", run)
					buf.WriteByte(prev)
				} else {
					for i := 0; i < run; i++ {
						buf.WriteByte(prev)
					}
				}
			}
			for x := 0; x < w; x++ {
				var bits byte
				for r := 0; r < rows; r++ {
					if idx[(band+r)*w+x] == c {
						bits |= 1 << uint(r)
					}
				}
				ch := byte('?') + bits
				if ch == prev {
					run++
				} else {
					flush()
					prev, run = ch, 1
				}
			}
			flush()
			buf.WriteByte('$')
		}
		buf.WriteByte('-')
	}
	buf.WriteString("\x1b\\")
	return buf.Bytes()
}

// q6 maps a channel to a 0..5 level, optionally with ordered (Bayer) dithering.
func q6(v byte, x, y int, dither bool) int {
	t := float64(v) / 255 * 5
	if dither {
		t += float64(bayer4[y&3][x&3])/16 - 0.5
	}
	lvl := int(t + 0.5)
	if lvl < 0 {
		return 0
	}
	if lvl > 5 {
		return 5
	}
	return lvl
}

// DirtyCellGap is how many clean cells may sit between two changed cells before
// they're still merged into one delta rectangle — bridging tiny gaps trades a
// few clean pixels for far fewer sub-images. DirtyCellRects coalesces on the
// terminal cell grid, so scattered animation (water glint, fireflies, glow
// pulses) costs a few small rectangles instead of one frame-spanning box.
const DirtyCellGap = 2

// DirtyCellRects diffs prev vs cur on the cell grid (cw×ch pixels per cell) and
// returns a small set of cell-aligned, pixel-coordinate rectangles covering
// every changed pixel — the basis for multi-region deltas. It returns
// changed=false when the frames are identical. When the changes are too large
// or too fragmented to beat a full repaint (covered area exceeds maxFrac of the
// frame, or more than maxRects rectangles are needed) it returns rects=nil with
// changed=true, signalling the caller to repaint in full.
func DirtyCellRects(prev, cur []byte, w, h, cw, ch, maxRects int, maxFrac float64) (rects []image.Rectangle, changed bool) {
	if cw < 1 || ch < 1 {
		if _, ok := DirtyRect(prev, cur, w, h); !ok {
			return nil, false
		}
		return nil, true
	}
	cols := (w + cw - 1) / cw
	rows := (h + ch - 1) / ch
	dirty := make([]bool, cols*rows)
	for y := 0; y < h; y++ {
		base := y * w * 4
		rowOff := (y / ch) * cols
		for x := 0; x < w; {
			cell := rowOff + x/cw
			if dirty[cell] {
				x = (x/cw + 1) * cw // already dirty — skip to the next cell
				continue
			}
			o := base + x*4
			if prev[o] != cur[o] || prev[o+1] != cur[o+1] || prev[o+2] != cur[o+2] || prev[o+3] != cur[o+3] {
				dirty[cell] = true
				x = (x/cw + 1) * cw
				continue
			}
			x++
		}
	}

	// Coalesce dirty cells: per-row horizontal runs (bridging gaps up to
	// DirtyCellGap), then extend a run downward whenever the row below has an
	// identical column span — so vertical bands (a lake, a tall structure) become
	// one rectangle rather than one per row.
	type crect struct{ c0, c1, r0, r1 int }
	var done, open []crect
	for r := 0; r < rows; r++ {
		var runs [][2]int
		for c := 0; c < cols; {
			if !dirty[r*cols+c] {
				c++
				continue
			}
			c0, last := c, c
			c++
			for c < cols {
				if dirty[r*cols+c] {
					last = c
					c++
					continue
				}
				g := 1
				for c+g < cols && !dirty[r*cols+c+g] {
					g++
				}
				if g <= DirtyCellGap && c+g < cols {
					c += g // bridge a short clean gap and keep the run going
					continue
				}
				break
			}
			runs = append(runs, [2]int{c0, last})
		}
		next := make([]crect, 0, len(runs))
		used := make([]bool, len(open))
		for _, rn := range runs {
			ext := -1
			for i, o := range open {
				if !used[i] && o.c0 == rn[0] && o.c1 == rn[1] {
					ext = i
					break
				}
			}
			if ext >= 0 {
				used[ext] = true
				o := open[ext]
				o.r1 = r
				next = append(next, o)
			} else {
				next = append(next, crect{rn[0], rn[1], r, r})
			}
		}
		for i, o := range open {
			if !used[i] {
				done = append(done, o)
			}
		}
		open = next
		if len(done)+len(open) > maxRects {
			return nil, true // too fragmented — a full repaint is cheaper
		}
	}
	done = append(done, open...)
	if len(done) == 0 {
		return nil, false
	}
	if len(done) > maxRects {
		return nil, true
	}

	area := 0
	rects = make([]image.Rectangle, len(done))
	for i, d := range done {
		x0, y0 := d.c0*cw, d.r0*ch
		x1, y1 := min((d.c1+1)*cw, w), min((d.r1+1)*ch, h)
		rects[i] = image.Rect(x0, y0, x1, y1)
		area += (x1 - x0) * (y1 - y0)
	}
	if float64(area) > float64(w*h)*maxFrac {
		return nil, true // changed area too large — repaint in full
	}
	return rects, true
}

// DirtyRect returns the bounding box of pixels that differ between two RGBA
// buffers of the same w×h, and whether anything changed.
func DirtyRect(prev, cur []byte, w, h int) (image.Rectangle, bool) {
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		base := y * w * 4
		for x := 0; x < w; x++ {
			o := base + x*4
			if prev[o] != cur[o] || prev[o+1] != cur[o+1] || prev[o+2] != cur[o+2] || prev[o+3] != cur[o+3] {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

// Crop copies a rectangle out of img into a new tightly-packed RGBA.
func Crop(img *image.RGBA, r image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		so := img.PixOffset(r.Min.X, r.Min.Y+y)
		do := out.PixOffset(0, y)
		copy(out.Pix[do:do+r.Dx()*4], img.Pix[so:so+r.Dx()*4])
	}
	return out
}
