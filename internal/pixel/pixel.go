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
// it (a=T, C=1). When cols/rows > 0 the image is display-scaled to fill that
// many text cells (c=,r=) — the basis for a fixed internal render resolution.
func EncodeKitty(img *image.RGBA, cols, rows int) []byte {
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
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
			fmt.Fprintf(&buf, "f=32,o=z,s=%d,v=%d,a=T,C=1,", bounds.Dx(), bounds.Dy())
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

// SnapToCells expands a pixel rect outward to the text-cell grid so a kitty/
// sixel sub-image placed at the cell aligns exactly with the dirty region.
func SnapToCells(r image.Rectangle, cw, ch, w, h int) image.Rectangle {
	x0 := (r.Min.X / cw) * cw
	y0 := (r.Min.Y / ch) * ch
	x1 := min(((r.Max.X+cw-1)/cw)*cw, w)
	y1 := min(((r.Max.Y+ch-1)/ch)*ch, h)
	return image.Rect(x0, y0, x1, y1)
}
