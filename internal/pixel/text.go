package pixel

import (
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// fold maps the common non-ASCII punctuation that turns up in slides to ASCII,
// since the 7×13 bitmap font only covers ASCII (anything else would draw as a
// blank box).
func fold(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '—', '–', '―', '‒', '•', '·', '◦', '▸', '‣':
			b.WriteByte('-')
		case '“', '”', '„':
			b.WriteByte('"')
		case '‘', '’', '‚':
			b.WriteByte('\'')
		case '…':
			b.WriteString("...")
		default:
			if r < 128 {
				b.WriteRune(r)
			} else {
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

// HD pixel mode has no terminal cells to print into — text on the big screen is
// drawn straight into the RGBA frame with a bitmap font (basicfont's 7×13), so
// it composites with the rasterized scene. Glyphs are nearest-scaled up for a
// crisp, chunky look that matches the pixel art.

var slideFace = basicfont.Face7x13

// lineH is the 7×13 font's line advance (a touch of leading).
const lineH = 15

// TextWidth is the pixel width of s drawn at the given integer scale.
func TextWidth(s string, scale int) int {
	return font.MeasureString(slideFace, s).Ceil() * scale
}

// DrawText draws s with its top-left at (x,y), nearest-scaled by scale, in col.
func DrawText(dst *image.RGBA, x, y, scale int, s string, col color.Color) {
	if scale < 1 {
		scale = 1
	}
	w := font.MeasureString(slideFace, s).Ceil()
	if w <= 0 {
		return
	}
	h := slideFace.Ascent + slideFace.Descent
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	d := &font.Drawer{
		Dst: tmp, Src: image.NewUniform(col), Face: slideFace,
		Dot: fixed.Point26_6{X: 0, Y: fixed.I(slideFace.Ascent)},
	}
	d.DrawString(s)
	for ty := 0; ty < h; ty++ {
		for tx := 0; tx < w; tx++ {
			if _, _, _, a := tmp.At(tx, ty).RGBA(); a == 0 {
				continue
			}
			c := tmp.RGBAAt(tx, ty)
			for ky := 0; ky < scale; ky++ {
				for kx := 0; kx < scale; kx++ {
					setIfInside(dst, x+tx*scale+kx, y+ty*scale+ky, c)
				}
			}
		}
	}
}

// DrawSlidePanel composites a presentation slide onto the frame: a translucent
// dark card with an accent border, the deck title (accent), the slide body
// (white) and a footer (dim), centered near the top so the avatar stays visible.
func DrawSlidePanel(img *image.RGBA, title string, body []string, footer string) {
	W := img.Bounds().Dx()
	scale := W / 360
	if scale < 2 {
		scale = 2
	}
	if scale > 5 {
		scale = 5
	}
	white := color.RGBA{0xEC, 0xF1, 0xF8, 255}
	accent := color.RGBA{0x7D, 0xF0, 0xFF, 255}
	dim := color.RGBA{0x9A, 0xA3, 0xAD, 255}

	type row struct {
		s   string
		col color.RGBA
	}
	var rows []row
	if title != "" {
		rows = append(rows, row{fold(title), accent}, row{"", white})
	}
	for _, l := range body {
		rows = append(rows, row{fold(l), white})
	}
	if footer != "" {
		rows = append(rows, row{"", white}, row{fold(footer), dim})
	}
	if len(rows) == 0 {
		return
	}

	maxW := 0
	for _, r := range rows {
		if w := TextWidth(r.s, scale); w > maxW {
			maxW = w
		}
	}
	pad := 5 * scale
	lh := lineH * scale
	pw := maxW + pad*2
	ph := len(rows)*lh + pad*2
	if pw > W-6 {
		pw = W - 6
	}
	ox := (W - pw) / 2
	oy := img.Bounds().Dy() / 12
	if oy < 4 {
		oy = 4
	}
	fillCard(img, ox, oy, pw, ph)

	ty := oy + pad
	for _, r := range rows {
		tx := ox + (pw-TextWidth(r.s, scale))/2
		DrawText(img, tx, ty, scale, r.s, r.col)
		ty += lh
	}
}

// fillCard draws a translucent dark rectangle with a 2px accent border.
func fillCard(img *image.RGBA, x, y, w, h int) {
	bg := color.RGBA{14, 18, 26, 255}
	border := color.RGBA{0x2E, 0x8B, 0xFF, 255}
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			px, py := x+i, y+j
			if !inside(img, px, py) {
				continue
			}
			if i < 2 || j < 2 || i >= w-2 || j >= h-2 {
				img.SetRGBA(px, py, border)
			} else {
				img.SetRGBA(px, py, blend(img.RGBAAt(px, py), bg, 0.9))
			}
		}
	}
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		255,
	}
}

func inside(img *image.RGBA, x, y int) bool {
	b := img.Bounds()
	return x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y
}

func setIfInside(img *image.RGBA, x, y int, c color.RGBA) {
	if inside(img, x, y) {
		img.SetRGBA(x, y, c)
	}
}
