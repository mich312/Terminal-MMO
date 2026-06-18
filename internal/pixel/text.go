package pixel

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/durst-group/durstworld/internal/markdown"
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
		case '│', '┃', '┆', '┊':
			b.WriteByte('|')
		case '─', '━', '┄', '┈':
			b.WriteByte('-')
		case '┼', '├', '┤', '┬', '┴', '┌', '┐', '└', '┘', '╳':
			b.WriteByte('+')
		case '☐':
			b.WriteString("[ ]")
		case '☑', '☒':
			b.WriteString("[x]")
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

// DrawSlidePanel composites a presentation slide onto the frame: the slide's
// markdown (parsed and syntax-highlighted by the markdown package) drawn into a
// translucent dark card with an accent border, plus a dim footer, centered near
// the top so the avatar stays visible.
func DrawSlidePanel(img *image.RGBA, src, footer string) {
	W := img.Bounds().Dx()
	scale := W / 360
	if scale < 2 {
		scale = 2
	}
	if scale > 5 {
		scale = 5
	}
	cols := (W * 5 / 6) / (7 * scale)
	if cols < 18 {
		cols = 18
	}
	if cols > 64 {
		cols = 64
	}
	lines := markdown.Render(src, cols)
	if footer != "" {
		lines = append(lines, markdown.Line{}, markdown.Line{{Text: footer, Color: "#9AA3AD"}})
	}

	maxW := 0
	for _, ln := range lines {
		if w := lineWidth(ln, scale); w > maxW {
			maxW = w
		}
	}
	pad := 5 * scale
	lh := lineH * scale
	pw := maxW + pad*2
	ph := len(lines)*lh + pad*2
	if pw > W-6 {
		pw = W - 6
	}
	ox := (W - pw) / 2
	oy := img.Bounds().Dy() / 12
	if oy < 4 {
		oy = 4
	}
	fillCard(img, ox, oy, pw, ph)

	codeBg := color.RGBA{24, 30, 44, 255}
	bar := color.RGBA{0x2E, 0x8B, 0xFF, 255}
	ix0, ix1 := ox+3, ox+pw-3
	ty := oy + pad
	for _, ln := range lines {
		if ln.IsCode() {
			fillRect(img, ix0, ty-2, ix1-ix0, lh, codeBg)
			fillRect(img, ix0, ty-2, scale, lh, bar)
		}
		drawSpanLine(img, ox+pad, ty, scale, ln)
		ty += lh
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			setIfInside(img, x+i, y+j, c)
		}
	}
}

func lineWidth(ln markdown.Line, scale int) int {
	w := 0
	for _, sp := range ln {
		w += TextWidth(fold(sp.Text), scale)
	}
	return w
}

func drawSpanLine(img *image.RGBA, x, y, scale int, ln markdown.Line) {
	white := color.RGBA{0xEC, 0xF1, 0xF8, 255}
	for _, sp := range ln {
		txt := fold(sp.Text)
		col := white
		if sp.Color != "" {
			col = hexRGBA(sp.Color)
		}
		DrawText(img, x, y, scale, txt, col)
		if sp.Bold { // the bitmap font is single-weight — thicken to fake bold
			DrawText(img, x+1, y, scale, txt, col)
		}
		w := TextWidth(txt, scale)
		if sp.Strike {
			sy := y + (13*scale)/2
			for i := 0; i < w; i++ {
				setIfInside(img, x+i, sy, col)
			}
		}
		if sp.Underline {
			uy := y + 12*scale
			for i := 0; i < w; i++ {
				setIfInside(img, x+i, uy, col)
			}
		}
		x += w
	}
}

func hexRGBA(s string) color.RGBA {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.RGBA{0xEC, 0xF1, 0xF8, 255}
	}
	var r, g, b int
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.RGBA{0xEC, 0xF1, 0xF8, 255}
	}
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

// fillCard draws a translucent dark rectangle with a 2px accent border.
// DrawPanel composites a bordered, translucent dark card — the base for HD UI
// overlays (the character and inventory panels). Exported wrapper over the
// slide card so the game package can build HUD panels onto a frame.
func DrawPanel(img *image.RGBA, x, y, w, h int) { fillCard(img, x, y, w, h) }

// Shade darkens a rectangle toward near-black by t∈[0,1] — backs the HUD bar
// and toasts so text stays legible over the scene.
func Shade(img *image.RGBA, x, y, w, h int, t float64) {
	dark := color.RGBA{8, 11, 16, 255}
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			if inside(img, x+i, y+j) {
				img.SetRGBA(x+i, y+j, blend(img.RGBAAt(x+i, y+j), dark, t))
			}
		}
	}
}

// cardUnit is the chunky-pixel size for panel chrome, matched to the bitmap
// font's scale so the borders read at the same resolution as the text.
func cardUnit(img *image.RGBA) int {
	u := img.Bounds().Dx() / 540
	if u < 2 {
		u = 2
	}
	if u > 3 {
		u = 3
	}
	return u
}

func fillCard(img *image.RGBA, x, y, w, h int) {
	u := cardUnit(img)
	bt := 2 * u
	bg := color.RGBA{14, 18, 26, 255}
	for j := bt; j < h-bt; j++ {
		for i := bt; i < w-bt; i++ {
			if inside(img, x+i, y+j) {
				img.SetRGBA(x+i, y+j, blend(img.RGBAAt(x+i, y+j), bg, 0.95))
			}
		}
	}
	drawFrame(img, x, y, w, h, u)
}

// Frame draws just the chunky pixel border (no fill) over an already-shaded
// region — used to give the HUD bar and toasts the same chrome as the panels.
func Frame(img *image.RGBA, x, y, w, h int) { drawFrame(img, x, y, w, h, cardUnit(img)) }

// drawFrame paints a chunky beveled border in u-sized pixel blocks: a dark
// outer outline, a two-tone bevel (lit top/left, shaded bottom/right) for a
// raised retro look, and notched (chamfered) corners.
func drawFrame(img *image.RGBA, x, y, w, h, u int) {
	edge := color.RGBA{0x10, 0x16, 0x22, 255}
	hi := color.RGBA{0x5C, 0xA6, 0xFF, 255}
	lo := color.RGBA{0x1C, 0x52, 0x9E, 255}
	bt := 2 * u
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			if i >= bt && i < w-bt && j >= bt && j < h-bt {
				continue // interior
			}
			if (i < u || i >= w-u) && (j < u || j >= h-u) {
				continue // notch the outer corners for a chamfered look
			}
			px, py := x+i, y+j
			if !inside(img, px, py) {
				continue
			}
			c := hi
			if i >= w-bt || j >= h-bt {
				c = lo // shaded bottom/right bevel
			}
			if i < u || j < u || i >= w-u || j >= h-u {
				c = edge // dark outer outline
			}
			img.SetRGBA(px, py, c)
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
