package game

import (
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// RenderRGBA rasterizes the same scene the glyph renderer draws — terrain
// (day/night tint + radial light, via the shared buildGrid) then players —
// into a true RGBA image for the experimental "real pixel" path (kitty
// graphics / sixel).
//
// smooth trades bandwidth for looks: bilinear terrain + a soft vignette is
// prettier but near-incompressible (every pixel differs), so it costs ~15× more
// on the wire; flat tiles keep long runs that zlib/sixel-RLE crush. Avatars are
// always anti-aliased over a soft contact shadow — a small, localized cost.
// Each tile maps to a scale×scale block; the avatar is sized to ~2 tiles tall
// so it sits within its 2×2 footprint, matching the half-block renderer.
func RenderRGBA(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera, light Light, originX, originY, scale int, smooth bool) *image.RGBA {
	if th == nil {
		th = ui.Default
	}
	if scale < 1 {
		scale = 1
	}
	if cam.W <= 0 || cam.H <= 0 {
		cam = Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	}

	grid := buildGrid(th, tm, cam, light, frame)
	cols := make([][]colorful.Color, cam.H)
	for y := 0; y < cam.H; y++ {
		cols[y] = make([]colorful.Color, cam.W)
		for x := 0; x < cam.W; x++ {
			if c := grid[y][x]; c.blank {
				cols[y][x] = shadowColor
			} else {
				cols[y][x] = c.fg
			}
		}
	}

	imgW, imgH := cam.W*scale, cam.H*scale
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	if smooth {
		cxf, cyf := float64(imgW)/2, float64(imgH)/2
		maxD := math.Hypot(cxf, cyf)
		for py := 0; py < imgH; py++ {
			gy := (float64(py)+0.5)/float64(scale) - 0.5
			y0 := clampi(int(math.Floor(gy)), 0, cam.H-1)
			y1 := clampi(y0+1, 0, cam.H-1)
			ty := gy - math.Floor(gy)
			for px := 0; px < imgW; px++ {
				gx := (float64(px)+0.5)/float64(scale) - 0.5
				x0 := clampi(int(math.Floor(gx)), 0, cam.W-1)
				x1 := clampi(x0+1, 0, cam.W-1)
				tx := gx - math.Floor(gx)

				c00, c10 := cols[y0][x0], cols[y0][x1]
				c01, c11 := cols[y1][x0], cols[y1][x1]
				r := bilerp(c00.R, c10.R, c01.R, c11.R, tx, ty)
				g := bilerp(c00.G, c10.G, c01.G, c11.G, tx, ty)
				b := bilerp(c00.B, c10.B, c01.B, c11.B, tx, ty)

				d := math.Hypot(float64(px)-cxf, float64(py)-cyf) / maxD
				v := 1 - 0.12*d*d // gentle vignette
				setPixel(img, px, py, r*v, g*v, b*v)
			}
		}
	} else {
		for vy := 0; vy < cam.H; vy++ {
			for vx := 0; vx < cam.W; vx++ {
				c := cols[vy][vx].Clamped()
				rc := colorfulToRGBA(c)
				for dy := 0; dy < scale; dy++ {
					row := img.Pix[img.PixOffset(vx*scale, vy*scale+dy):]
					for dx := 0; dx < scale; dx++ {
						o := dx * 4
						row[o], row[o+1], row[o+2], row[o+3] = rc.R, rc.G, rc.B, 255
					}
				}
			}
		}
	}

	stampSpritesRGBA(img, players, self, scale, originX, originY)
	return img
}

// stampSpritesRGBA draws every player's avatar, oldest movers first and self
// last, mirroring the glyph renderer's ordering.
func stampSpritesRGBA(img *image.RGBA, players []world.Player, self string, scale, originX, originY int) {
	sorted := make([]world.Player, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name == self {
			return false
		}
		if sorted[j].Name == self {
			return true
		}
		return sorted[i].LastMoved.Before(sorted[j].LastMoved)
	})
	for _, p := range sorted {
		blitAvatar(img, p, p.Name == self, scale, p.X-originX, p.Y-originY)
	}
}

// blitAvatar draws one player: a soft contact shadow, then the bitmap upscaled
// with bilinear alpha for rounded, anti-aliased edges, and a small chevron
// above your own head. (fc,fr) is the footprint top-left in camera cells; the
// sprite is centered horizontally and bottom-aligned on the footprint.
func blitAvatar(img *image.RGBA, p world.Player, isSelf bool, scale, fc, fr int) {
	body := playerColor(p.Color)
	spr := buildSpriteRGBA(body, isSelf)
	bmpW, bmpH := spr.Bounds().Dx(), spr.Bounds().Dy()
	// Size the avatar to ~2 tiles tall so it sits within its 2×2 footprint
	// instead of overhanging; width follows the bitmap's aspect.
	spx := (PlayerH * scale) / bmpH
	if spx < 1 {
		spx = 1
	}
	destW, destH := bmpW*spx, bmpH*spx

	centerX := (fc + PlayerW/2) * scale
	bottomEdge := (fr + PlayerH) * scale
	left := centerX - destW/2
	top := bottomEdge - destH

	// soft elliptical contact shadow at the feet
	drawShadow(img, float64(centerX), float64(bottomEdge)-float64(spx)*0.6,
		float64(destW)*0.46, float64(spx)*1.2)

	drawScaledSprite(img, spr, left, top, destW, destH)

	if isSelf {
		drawChevron(img, centerX, top-spx-spx/2, spx)
	}
}

// buildSpriteRGBA renders the avatar bitmap to a 6×8 RGBA: opaque pixels carry
// their shaded body color (alpha 255), transparent ones are zero — which, with
// alpha 0/1, doubles as a premultiplied buffer for clean bilinear edges.
func buildSpriteRGBA(body colorful.Color, isSelf bool) *image.RGBA {
	w, h := len(avatarBitmap[0]), len(avatarBitmap)
	spr := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		runes := []rune(avatarBitmap[y])
		for x := 0; x < w; x++ {
			col, opaque := spritePixel(runes[x], body, isSelf)
			if !opaque {
				continue
			}
			spr.SetRGBA(x, y, colorfulToRGBA(col))
		}
	}
	return spr
}

// drawScaledSprite upscales a premultiplied RGBA into img with bilinear
// sampling and alpha compositing, so the avatar's edges are smooth.
func drawScaledSprite(img *image.RGBA, spr *image.RGBA, dx, dy, destW, destH int) {
	sw, sh := spr.Bounds().Dx(), spr.Bounds().Dy()
	for j := 0; j < destH; j++ {
		sv := (float64(j)+0.5)/float64(destH)*float64(sh) - 0.5
		for i := 0; i < destW; i++ {
			su := (float64(i)+0.5)/float64(destW)*float64(sw) - 0.5
			r, g, b, a := sampleSpritePM(spr, su, sv)
			if a <= 0.004 {
				continue
			}
			px, py := dx+i, dy+j
			or, og, ob, ok := getPixel(img, px, py)
			if !ok {
				continue
			}
			setPixel8(img, px, py,
				r+float64(or)*(1-a),
				g+float64(og)*(1-a),
				b+float64(ob)*(1-a))
		}
	}
}

// sampleSpritePM bilinearly samples a premultiplied sprite, returning
// premultiplied r,g,b in 0..255 and coverage a in 0..1.
func sampleSpritePM(spr *image.RGBA, u, v float64) (r, g, b, a float64) {
	w, h := spr.Bounds().Dx(), spr.Bounds().Dy()
	x0 := clampi(int(math.Floor(u)), 0, w-1)
	x1 := clampi(x0+1, 0, w-1)
	y0 := clampi(int(math.Floor(v)), 0, h-1)
	y1 := clampi(y0+1, 0, h-1)
	tx := u - math.Floor(u)
	ty := v - math.Floor(v)
	if tx < 0 {
		tx = 0
	}
	if ty < 0 {
		ty = 0
	}
	r00, g00, b00, a00 := texel(spr, x0, y0)
	r10, g10, b10, a10 := texel(spr, x1, y0)
	r01, g01, b01, a01 := texel(spr, x0, y1)
	r11, g11, b11, a11 := texel(spr, x1, y1)
	r = bilerp(r00, r10, r01, r11, tx, ty)
	g = bilerp(g00, g10, g01, g11, tx, ty)
	b = bilerp(b00, b10, b01, b11, tx, ty)
	a = bilerp(a00, a10, a01, a11, tx, ty)
	return
}

func texel(spr *image.RGBA, x, y int) (r, g, b, a float64) {
	o := spr.PixOffset(x, y)
	return float64(spr.Pix[o]), float64(spr.Pix[o+1]), float64(spr.Pix[o+2]), float64(spr.Pix[o+3]) / 255
}

// drawShadow darkens an elliptical patch toward the ground color, softly.
func drawShadow(img *image.RGBA, cx, cy, rx, ry float64) {
	for y := int(cy - ry); y <= int(cy+ry); y++ {
		for x := int(cx - rx); x <= int(cx+rx); x++ {
			nx := (float64(x) - cx) / rx
			ny := (float64(y) - cy) / ry
			d2 := nx*nx + ny*ny
			if d2 > 1 {
				continue
			}
			or, og, ob, ok := getPixel(img, x, y)
			if !ok {
				continue
			}
			a := 0.4 * (1 - d2)
			setPixel8(img, x, y,
				float64(or)*(1-a), float64(og)*(1-a), float64(ob)*(1-a))
		}
	}
}

// drawChevron marks the local player with a small white downward triangle.
func drawChevron(img *image.RGBA, cx, baseY, spx int) {
	hw := spx + spx/2
	if hw < 2 {
		hw = 2
	}
	white := color.RGBA{0xF5, 0xF7, 0xFA, 255}
	for dy := 0; dy < hw; dy++ {
		span := hw - dy
		for dx := -span; dx <= span; dx++ {
			if _, _, _, ok := getPixel(img, cx+dx, baseY+dy); ok {
				img.SetRGBA(cx+dx, baseY+dy, white)
			}
		}
	}
}

func bilerp(c00, c10, c01, c11, tx, ty float64) float64 {
	top := c00 + (c10-c00)*tx
	bot := c01 + (c11-c01)*tx
	return top + (bot-top)*ty
}

func setPixel(img *image.RGBA, x, y int, r, g, b float64) {
	img.SetRGBA(x, y, color.RGBA{f2b(r), f2b(g), f2b(b), 255})
}

func setPixel8(img *image.RGBA, x, y int, r, g, b float64) {
	img.SetRGBA(x, y, color.RGBA{f2b(r / 255), f2b(g / 255), f2b(b / 255), 255})
}

func getPixel(img *image.RGBA, x, y int) (r, g, b uint8, ok bool) {
	bnd := img.Bounds()
	if x < bnd.Min.X || x >= bnd.Max.X || y < bnd.Min.Y || y >= bnd.Max.Y {
		return 0, 0, 0, false
	}
	o := img.PixOffset(x, y)
	return img.Pix[o], img.Pix[o+1], img.Pix[o+2], true
}

func colorfulToRGBA(c colorful.Color) color.RGBA {
	c = c.Clamped()
	return color.RGBA{f2b(c.R), f2b(c.G), f2b(c.B), 255}
}

// f2b clamps a 0..1 float to a 0..255 byte.
func f2b(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v*255 + 0.5)
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
