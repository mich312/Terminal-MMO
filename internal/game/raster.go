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
	texs := make([][]TileTex, cam.H)
	for y := 0; y < cam.H; y++ {
		cols[y] = make([]colorful.Color, cam.W)
		texs[y] = make([]TileTex, cam.W)
		for x := 0; x < cam.W; x++ {
			if c := grid[y][x]; c.blank {
				cols[y][x] = shadowColor
			} else {
				cols[y][x] = c.fg
			}
			if tx, ty := cam.X+x, cam.Y+y; ty >= 0 && ty < tm.H && tx >= 0 && tx < tm.W {
				texs[y][x] = tm.Tiles[ty][tx].Tex
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
		// Pixel-perfect tilemap, old-RPG style: each tile is a solid base color
		// with a few clean, biome-specific accent pixels (grass blades, water
		// ripples, sand speckles…). Hard tile edges — no blur, no all-over grain.
		for vy := 0; vy < cam.H; vy++ {
			for vx := 0; vx < cam.W; vx++ {
				paintTile(img, vx*scale, vy*scale, scale, cols[vy][vx],
					texs[vy][vx], originX+vx, originY+vy, frame)
			}
		}
	}

	stampSpritesRGBA(img, players, self, frame, scale, originX, originY)
	return img
}

// stampSpritesRGBA draws every player's avatar, oldest movers first and self
// last, mirroring the glyph renderer's ordering.
func stampSpritesRGBA(img *image.RGBA, players []world.Player, self string, frame, scale, originX, originY int) {
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
		blitAvatar(img, p, p.Name == self, frame, scale, p.X-originX, p.Y-originY)
	}
}

// blitAvatar draws one player as crisp pixel art: a soft contact shadow, then
// the sprite scaled by an integer factor (sharp, nearest-neighbor — no blur)
// and a small chevron above your own head. (fc,fr) is the footprint top-left in
// camera cells; the sprite is centered horizontally and bottom-aligned.
func blitAvatar(img *image.RGBA, p world.Player, isSelf bool, frame, scale, fc, fr int) {
	wf := AvatarWalkFrame(p.LastMoved, frame)
	body := playerColor(p.Color)
	bmp := AvatarBitmap(p.Style, p.Accessory, p.Facing, wf)
	bw, bh := len([]rune(bmp[0])), len(bmp)
	// Integer pixel size keeps edges sharp; aim for ~2 tiles tall.
	k := (PlayerH * scale) / bh
	if k < 1 {
		k = 1
	}
	destW, destH := bw*k, bh*k

	centerX := (fc + PlayerW/2) * scale
	bottomEdge := (fr + PlayerH) * scale
	left := centerX - destW/2
	top := bottomEdge - destH

	// soft elliptical contact shadow at the feet (stays planted while the body
	// bobs, so the step reads as a bounce rather than a slide)
	drawShadow(img, float64(centerX), float64(bottomEdge)-float64(k)*0.6,
		float64(destW)*0.42, float64(k)*1.3)

	if wf == 1 { // mid-stride: lift the body a touch
		bob := k / 2
		if bob < 1 {
			bob = 1
		}
		top -= bob
	}

	for sy := 0; sy < bh; sy++ {
		runes := []rune(bmp[sy])
		for sx := 0; sx < bw && sx < len(runes); sx++ {
			col, opaque := spritePixel(runes[sx], body, isSelf)
			if !opaque {
				continue
			}
			fillRect(img, left+sx*k, top+sy*k, k, k, colorfulToRGBA(col))
		}
	}

	if isSelf {
		drawChevron(img, centerX, top-k-k/2, k)
	}
}

// fillRect paints a w×h block of one color, clipped to the image bounds.
func fillRect(img *image.RGBA, x0, y0, w, h int, c color.RGBA) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			if _, _, _, ok := getPixel(img, x, y); ok {
				img.SetRGBA(x, y, c)
			}
		}
	}
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

// paintTile fills one tile with its base color, then stamps a few clean
// biome-specific accent pixels — the old-RPG tilemap look. Accent positions are
// hashed from the tile's world coords so they're stable as the camera scrolls;
// water ripples animate with the frame.
func paintTile(img *image.RGBA, ox, oy, scale int, base colorful.Color, tex TileTex, wx, wy, frame int) {
	fillRect(img, ox, oy, scale, scale, colorfulToRGBA(base))
	if tex == TexFlat || scale < 3 {
		return
	}
	light := colorfulToRGBA(base.BlendLab(spriteWhite, 0.16).Clamped())
	dark := colorfulToRGBA(base.BlendLab(shadowColor, 0.20).Clamped())
	hp := func(salt int) (int, int) {
		return ox + int(hashNoise(wx, wy, salt)*float64(scale-1)),
			oy + int(hashNoise(wx, wy, salt+97)*float64(scale-1))
	}
	switch tex {
	case TexGrass:
		for i := 0; i < 2; i++ { // little blades
			x, y := hp(i * 5)
			setRGBAClip(img, x, y, dark)
			setRGBAClip(img, x, y-1, dark)
		}
		x, y := hp(40)
		setRGBAClip(img, x, y, light)
	case TexSand:
		for i := 0; i < 3; i++ {
			x, y := hp(i * 3)
			setRGBAClip(img, x, y, dark)
		}
	case TexDirt:
		x, y := hp(1) // pebble
		setRGBAClip(img, x, y, light)
		setRGBAClip(img, x+1, y, light)
		x2, y2 := hp(8)
		setRGBAClip(img, x2, y2, dark)
	case TexForest:
		x, y := hp(2) // dappled light through the canopy
		for _, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}} {
			setRGBAClip(img, x+d[0], y+d[1], light)
		}
	case TexRock:
		x, y := hp(1) // a small chip + crack, not a full hatch
		setRGBAClip(img, x, y, dark)
		setRGBAClip(img, x+1, y+1, dark)
		x2, y2 := hp(9)
		setRGBAClip(img, x2, y2, light)
	case TexWater:
		for iy := 0; iy < scale; iy++ { // moving ripple highlights
			if (iy+wx+frame/3)%4 != 0 {
				continue
			}
			for ix := 0; ix < scale; ix++ {
				if (ix+wy*2+frame/3)%6 < 2 {
					setRGBAClip(img, ox+ix, oy+iy, light)
				}
			}
		}
	}
}

func setRGBAClip(img *image.RGBA, x, y int, c color.RGBA) {
	if _, _, _, ok := getPixel(img, x, y); ok {
		img.SetRGBA(x, y, c)
	}
}

// hashNoise is a cheap deterministic [0,1) value from a few ints (FNV-ish), for
// stable per-tile accent placement.
func hashNoise(vals ...int) float64 {
	var h uint32 = 2166136261
	for _, v := range vals {
		h = (h ^ uint32(v)) * 16777619
	}
	h ^= h >> 13
	h *= 2654435761
	h ^= h >> 16
	return float64(h&0xffff) / 65536
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
