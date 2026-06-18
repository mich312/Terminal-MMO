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
//
// style selects the art style (sprite sets, shading, palette); a nil style uses
// DefaultStyle. A non-nil style.Palette.Map recolors the finished frame in one
// final pass (the basis for the monochrome / neon looks).
func RenderRGBA(th *ui.Theme, tm *TileMap, players []world.Player, self string, frame int, cam Camera, light Light, originX, originY, scale int, smooth bool, style *Style) *image.RGBA {
	if th == nil {
		th = ui.Default
	}
	if style == nil {
		style = DefaultStyle()
	}
	if scale < 1 {
		scale = 1
	}
	if cam.W <= 0 || cam.H <= 0 {
		cam = Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	}

	grid := buildGrid(th, tm, cam, light, frame, originX, originY)
	cols := make([][]colorful.Color, cam.H)
	texs := make([][]TileTex, cam.H)
	props := make([][]TileProp, cam.H)
	propCols := make([][]colorful.Color, cam.H)
	for y := 0; y < cam.H; y++ {
		cols[y] = make([]colorful.Color, cam.W)
		texs[y] = make([]TileTex, cam.W)
		props[y] = make([]TileProp, cam.W)
		propCols[y] = make([]colorful.Color, cam.W)
		for x := 0; x < cam.W; x++ {
			if c := grid[y][x]; c.blank {
				cols[y][x] = shadowColor
			} else {
				cols[y][x] = c.fg
			}
			tx, ty := cam.X+x, cam.Y+y
			if ty < 0 || ty >= tm.H || tx < 0 || tx >= tm.W {
				continue
			}
			t := tm.Tiles[ty][tx]
			texs[y][x] = t.Tex
			props[y][x] = t.Prop
			// A prop's ground is colored separately from the glyph's color so the
			// flower-glyph stays red in the text renderer while HD draws grass.
			if t.Ground != "" {
				// Lighting is evaluated in absolute world coordinates (see buildGrid):
				// tx,ty index the window tile, originX+x/originY+y is its world cell.
				cols[y][x] = applyLight(style.tint(t.Ground), originX+x, originY+y, light)
			}
			if t.Prop != PropNone {
				ph := t.PropHex
				if ph == "" {
					ph = t.Color
				}
				propCols[y][x] = style.tint(ph)
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
				v := 1 - style.Vignette*d*d // gentle vignette
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
					texs[vy][vx], props[vy][vx], propCols[vy][vx], originX+vx, originY+vy, frame, style)
			}
		}
		// Portals are multi-tile animated gates drawn over the terrain so they can
		// overhang upward. (Houses are single-tile props, drawn by paintTile.)
		for vy := 0; vy < cam.H; vy++ {
			for vx := 0; vx < cam.W; vx++ {
				if props[vy][vx] == PropPortal {
					drawStructure(img, vx, vy, scale, propCols[vy][vx], frame, style.Portal, style.Palette)
				}
			}
		}
	}

	stampSpritesRGBA(img, players, self, frame, scale, originX, originY)
	if m := style.Palette.Map; m != nil {
		applyColorMap(img, m)
	}
	return img
}

// applyColorMap remaps every opaque pixel through m — one pass over the finished
// frame, so a style can recolor terrain, props and avatars together without
// touching the per-element draw paths.
func applyColorMap(img *image.RGBA, m func(colorful.Color) colorful.Color) {
	p := img.Pix
	for i := 0; i+3 < len(p); i += 4 {
		c := colorful.Color{R: float64(p[i]) / 255, G: float64(p[i+1]) / 255, B: float64(p[i+2]) / 255}
		c = m(c).Clamped()
		p[i], p[i+1], p[i+2] = f2b(c.R), f2b(c.G), f2b(c.B)
	}
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

// tileArtN is the authored tile resolution (6×6 art-pixels per tile).
const tileArtN = 6

// paintTile draws one tile: the ground surface sprite (a shade pattern colored
// by base) nearest-upscaled to the on-screen tile size, then an optional prop
// sprite over it. Sharp pixels throughout.
func paintTile(img *image.RGBA, ox, oy, scale int, base colorful.Color, tex TileTex, prop TileProp, propCol colorful.Color, wx, wy, frame int, style *Style) {
	baseRGBA := colorfulToRGBA(base)
	variants := style.Ground[tex]
	if len(variants) == 0 { // flat / untextured
		fillRect(img, ox, oy, scale, scale, baseRGBA)
	} else {
		idx := int(hashNoise(wx, wy) * float64(len(variants)))
		if tex == TexWater {
			idx = (frame / 4) % len(variants) // ripples animate
		}
		art := variants[idx%len(variants)]
		light := colorfulToRGBA(base.BlendLab(spriteWhite, style.GroundLightMix).Clamped())
		dark := colorfulToRGBA(base.BlendLab(shadowColor, style.GroundDarkMix).Clamped())
		blitTileArt(img, ox, oy, scale, art, func(r byte) (color.RGBA, bool) {
			switch r {
			case 'L':
				return light, true
			case 'D':
				return dark, true
			default:
				return baseRGBA, true // 'B' and ' '
			}
		})
	}

	if art, ok := style.Props[prop]; ok {
		// A richer prop palette than plain fill/shade: outline (o), shades (p, D),
		// highlights (L, W), trunk (T) and an animated emissive glow (G) that
		// pulses with the frame so screens, lamps and the reactor core come alive.
		// Per-tile phase keeps lamps/screens from blinking in unison.
		glow := propCol.BlendLab(spriteWhite, 0.45+0.4*math.Sin(float64(frame)*0.3+float64(wx*7+wy*3)))
		paint := func(r byte) (color.RGBA, bool) {
			switch r {
			case 'P':
				return colorfulToRGBA(propCol), true
			case 'p':
				return colorfulToRGBA(propCol.BlendLab(shadowColor, style.PropShadeMix).Clamped()), true
			case 'D':
				return colorfulToRGBA(propCol.BlendLab(shadowColor, 0.42).Clamped()), true
			case 'o':
				return colorfulToRGBA(propCol.BlendLab(shadowColor, 0.6).Clamped()), true
			case 'L':
				return colorfulToRGBA(propCol.BlendLab(spriteWhite, 0.4).Clamped()), true
			case 'W':
				return colorfulToRGBA(propCol.BlendLab(spriteWhite, 0.72).Clamped()), true
			case 'G':
				return colorfulToRGBA(glow.Clamped()), true
			case 'T':
				return colorfulToRGBA(style.Trunk), true
			default:
				return color.RGBA{}, false // '.' transparent
			}
		}
		blitTileArt(img, ox, oy, scale, art, paint)
	}
}

// drawStructure renders a multi-tile sprite (house or portal) centered on tile
// (vx,vy), bottom-aligned so its base sits on the tile and it overhangs upward.
// 'R'/'P' take the structure color; '@' is the animated portal swirl.
func drawStructure(img *image.RGBA, vx, vy, scale int, col colorful.Color, frame int, art []string, pal ui.Palette) {
	apx := scale / tileArtN // art-pixel size, matching the avatar's
	if apx < 1 {
		apx = 1
	}
	left := vx*scale + scale/2 - (len(art[0])*apx)/2
	top := (vy+1)*scale - len(art)*apx

	body := colorfulToRGBA(col)
	roof := colorfulToRGBA(col.BlendLab(shadowColor, 0.38).Clamped())
	win := colorfulToRGBA(col.BlendLab(spriteWhite, 0.45).Clamped())
	base := colorfulToRGBA(col.BlendLab(shadowColor, 0.6).Clamped())
	ring := colorfulToRGBA(col.BlendLab(spriteWhite, 0.2).Clamped())

	for ay, row := range art {
		for ax := 0; ax < len(row); ax++ {
			var c color.RGBA
			ok := true
			switch row[ax] {
			case 'P':
				c = body
			case 'R':
				c = ring
			case 'p':
				c = roof
			case 'L':
				c = win
			case 'D':
				c = base
			case '@':
				c = portalPixel(ax, ay, frame, pal)
			default:
				ok = false
			}
			if ok {
				fillRect(img, left+ax*apx, top+ay*apx, apx, apx, c)
			}
		}
	}
}

// portalPixel returns a swirling portal color for an art pixel at the given
// frame — diagonal bands of the style's portal ramp drift to read as an active
// gate.
func portalPixel(ax, ay, frame int, pal ui.Palette) color.RGBA {
	s := 0.5 + 0.5*math.Sin(float64(ax+ay)*0.7-float64(frame)*0.45)
	return colorfulToRGBA(mustHex(string(ui.Blend(pal.PortalA, pal.PortalB, s))))
}

// blitTileArt nearest-upscales a tileArtN×tileArtN art grid into the scale×scale
// tile at (ox,oy), coloring each art rune via paint; paint's second return is
// false for transparent runes.
func blitTileArt(img *image.RGBA, ox, oy, scale int, art []string, paint func(byte) (color.RGBA, bool)) {
	for iy := 0; iy < scale; iy++ {
		row := art[iy*tileArtN/scale]
		for ix := 0; ix < scale; ix++ {
			c, ok := paint(row[ix*tileArtN/scale])
			if ok {
				img.SetRGBA(ox+ix, oy+iy, c)
			}
		}
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
