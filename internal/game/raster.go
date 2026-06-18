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
	// Day/night ambient for the ground and prop colors, so the whole HD scene
	// (not just the base terrain that flows through buildGrid) shifts with the
	// time of day, matching the glyph renderer.
	ambHex, ambStr := ui.Ambient(ui.Now())
	amb := mustHex(ambHex)
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
				cols[y][x] = applyLight(tint(style.tint(t.Ground), amb, ambStr), originX+x, originY+y, light)
			}
			if t.Prop != PropNone {
				ph := t.PropHex
				if ph == "" {
					ph = t.Color
				}
				propCols[y][x] = tint(style.tint(ph), amb, ambStr)
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
				// Gather the 8 neighbors' lit ground colors so paintTile can dither
				// the tile's border toward a differing biome — soft seams instead of
				// hard grid edges. Out-of-window neighbors fall back to self (no blend).
				at := func(x, y int) colorful.Color {
					if x < 0 || x >= cam.W || y < 0 || y >= cam.H {
						return cols[vy][vx]
					}
					return cols[y][x]
				}
				nbr := [8]colorful.Color{
					at(vx, vy-1), at(vx+1, vy-1), at(vx+1, vy), at(vx+1, vy+1),
					at(vx, vy+1), at(vx-1, vy+1), at(vx-1, vy), at(vx-1, vy-1),
				}
				paintTile(img, vx*scale, vy*scale, scale, cols[vy][vx],
					texs[vy][vx], props[vy][vx], propCols[vy][vx], originX+vx, originY+vy, frame, style, nbr)
			}
		}
		// Sun-glint / moon-glitter shimmering across water.
		waterGlint(img, texs, cam, scale, frame, originX, originY)
		// Tall props (trees, portals) overhang upward. Draw all the canopy shadows
		// first, then the canopies, so a near tree's shadow never lands on the
		// canopy of one behind it — shadows always stay behind the trees. Within
		// the canopy pass, top rows first so a nearer crown overlaps the one behind.
		shadowMask := make([]uint8, imgW*imgH)
		for vy := 0; vy < cam.H; vy++ {
			for vx := 0; vx < cam.W; vx++ {
				if art, ok := canopyArt(props[vy][vx], originX+vx, originY+vy); ok {
					accumCanopyShadow(shadowMask, imgW, imgH, vx, vy, scale, art)
				} else if vs, ok := buildingArtFor(props[vy][vx]); ok {
					accumBuildingShadow(shadowMask, imgW, imgH, vx, vy, scale, vs[0])
				}
			}
		}
		applyShadowMask(img, shadowMask)
		for vy := 0; vy < cam.H; vy++ {
			for vx := 0; vx < cam.W; vx++ {
				if props[vy][vx] == PropPortal {
					drawStructure(img, vx, vy, scale, propCols[vy][vx], frame, style.Portal, style.Palette)
				} else if art, ok := canopyArt(props[vy][vx], originX+vx, originY+vy); ok {
					drawCanopy(img, vx, vy, scale, propCols[vy][vx], art, originX+vx, originY+vy, amb, ambStr)
				} else if vs, ok := buildingArtFor(props[vy][vx]); ok {
					drawBuilding(img, vx, vy, originX+vx, originY+vy, scale, propCols[vy][vx], vs)
				}
			}
		}
		// Night point lights: emissive props (campfires, portals, lamps, the
		// reactor core, gem loot…) bloom a warm/cool glow pool on the scene after
		// dusk, scaled by how dark it is.
		if _, _, night := sunState(); night > 0.03 {
			apx := scale / tileArtN
			if apx < 1 {
				apx = 1
			}
			for vy := 0; vy < cam.H; vy++ {
				for vx := 0; vx < cam.W; vx++ {
					if col, rad, mult, ok := emitterGlow(props[vy][vx], propCols[vy][vx], frame, originX+vx, originY+vy); ok {
						drawGlow(img, vx*scale+scale/2, vy*scale+scale/2, rad*float64(scale), col, night*mult, apx)
					}
				}
			}
		}
		// Fireflies / bioluminescent motes drifting over woods and swamp at dusk.
		drawFireflies(img, texs, cam, scale, frame, originX, originY)
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
		float64(destW)*0.42, float64(k)*1.3, k, float64(destH))

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

// shadowBlocks walks the px-snapped, two-level (core/rim) elliptical shadow for
// a caster, applying the always-on directional lean plus the softened
// golden-hour stretch, and calls emit(x, y, alpha) for each covered pixel.
// Sharing this lets shadows be darkened straight onto the frame, or accumulated
// into a max-coverage mask so overlapping shadows don't stack.
func shadowBlocks(cx, cy, rx, ry float64, px int, height float64, emit func(x, y int, a float64)) {
	if px < 1 {
		px = 1
	}
	// Shadows always lean a bit to one side (a fixed key light), with a gentle
	// extra stretch along the sun's azimuth when it's low at dawn/dusk.
	elev, azX, _ := sunState()
	golden := 0.0
	if elev > 0 {
		golden = (1 - elev) * math.Min(1, elev*4)
	}
	side := 1.0
	if elev > 0 && azX < 0 {
		side = -1 // morning sun in the east → shadow falls west
	}
	reach := 0.28*height + 0.5*golden*math.Abs(azX)*height
	cx += side * reach * 0.5
	rx += reach * 0.5
	bx0 := int(math.Floor((cx-rx)/float64(px))) * px
	bx1 := int(math.Floor((cx+rx)/float64(px))) * px
	by0 := int(math.Floor((cy-ry)/float64(px))) * px
	by1 := int(math.Floor((cy+ry)/float64(px))) * px
	for by := by0; by <= by1; by += px {
		for bx := bx0; bx <= bx1; bx += px {
			// Test the block centre against the ellipse; quantize to two levels.
			nx := (float64(bx) + float64(px)/2 - cx) / rx
			ny := (float64(by) + float64(px)/2 - cy) / ry
			d2 := nx*nx + ny*ny
			if d2 > 1 {
				continue
			}
			a := 0.42
			if d2 > 0.5 {
				a = 0.22 // lighter rim block
			}
			for y := by; y < by+px; y++ {
				for x := bx; x < bx+px; x++ {
					emit(x, y, a)
				}
			}
		}
	}
}

// drawShadow darkens a single elliptical shadow straight onto the frame, snapped
// to a px block grid with two retro alpha steps. height is the caster's height,
// used for the directional/golden-hour stretch. (Used for avatars and bulky
// single-tile props; tree canopies accumulate via a mask instead.)
func drawShadow(img *image.RGBA, cx, cy, rx, ry float64, px int, height float64) {
	shadowBlocks(cx, cy, rx, ry, px, height, func(x, y int, a float64) {
		or, og, ob, ok := getPixel(img, x, y)
		if !ok {
			return
		}
		setPixel8(img, x, y, float64(or)*(1-a), float64(og)*(1-a), float64(ob)*(1-a))
	})
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
func paintTile(img *image.RGBA, ox, oy, scale int, base colorful.Color, tex TileTex, prop TileProp, propCol colorful.Color, wx, wy, frame int, style *Style, nbr [8]colorful.Color) {
	var art []string
	if variants := style.Ground[tex]; len(variants) > 0 {
		idx := int(hashNoise(wx, wy) * float64(len(variants)))
		if tex == TexWater {
			idx = (frame / 4) % len(variants) // ripples animate
		}
		art = variants[idx%len(variants)]
	}
	light := base.BlendLab(spriteWhite, style.GroundLightMix).Clamped()
	dark := base.BlendLab(shadowColor, style.GroundDarkMix).Clamped()
	blitGround(img, ox, oy, scale, art, base, light, dark, nbr, wx, wy)

	// Ground a few bulky props with a soft contact shadow so they don't look
	// pasted onto the terrain (trees get theirs in drawTree).
	if prop == PropHouse || prop == PropBoulder {
		apx := scale / tileArtN
		if apx < 1 {
			apx = 1
		}
		drawShadow(img, float64(ox+scale/2), float64(oy+scale)-float64(apx),
			float64(scale)*0.4, float64(apx)*1.3, apx, float64(scale))
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

// buildingArtFor returns the sprite variants for a village building prop.
func buildingArtFor(p TileProp) ([][]string, bool) {
	a, ok := buildingArt[p]
	return a, ok
}

// bldHash is a stable per-building hash of its world position, used to pick a
// sprite variant and a small color jitter so neighbours don't look identical.
func bldHash(wx, wy int) uint32 {
	return uint32(wx)*73856093 ^ uint32(wy)*19349663
}

// drawBuilding renders a multi-tile village building, bottom-left-anchored on
// tile (vx,vy) so it rises up and extends right from its base. It picks one of
// the type's variants and nudges the color, both keyed on world position (wx,wy)
// so the same building always looks the same. Codes: P wall, p roof, D
// base/door, L window, R trim/cross.
func drawBuilding(img *image.RGBA, vx, vy, wx, wy, scale int, col colorful.Color, variants [][]string) {
	hsh := bldHash(wx, wy)
	art := variants[hsh%uint32(len(variants))]
	if j := float64(int((hsh>>5)%7)-3) * 0.022; j >= 0 { // ±~7% lightness per building
		col = col.BlendLab(spriteWhite, j)
	} else {
		col = col.BlendLab(shadowColor, -j)
	}
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	left := vx * scale
	top := (vy+1)*scale - len(art)*apx
	body := colorfulToRGBA(col)
	roof := colorfulToRGBA(col.BlendLab(shadowColor, 0.45).Clamped())
	base := colorfulToRGBA(col.BlendLab(shadowColor, 0.62).Clamped())
	win := colorfulToRGBA(col.BlendLab(spriteWhite, 0.6).Clamped())
	trim := colorfulToRGBA(col.BlendLab(spriteWhite, 0.32).Clamped())
	for ay, row := range art {
		for ax := 0; ax < len(row); ax++ {
			var c color.RGBA
			ok := true
			switch row[ax] {
			case 'P':
				c = body
			case 'p':
				c = roof
			case 'D':
				c = base
			case 'L':
				c = win
			case 'R':
				c = trim
			default:
				ok = false
			}
			if ok {
				fillRect(img, left+ax*apx, top+ay*apx, apx, apx, c)
			}
		}
	}
}

// accumBuildingShadow records a building's soft contact shadow, centred under
// its footprint, into the shared max-coverage shadow mask.
func accumBuildingShadow(mask []uint8, imgW, imgH, vx, vy, scale int, art []string) {
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	w, h := len(art[0]), len(art)
	cx := float64(vx*scale) + float64(w*apx)/2
	by := float64((vy+1)*scale) - float64(apx)
	shadowBlocks(cx, by, float64(w*apx)*0.5, float64(apx)*1.5, apx, float64(h*apx),
		func(x, y int, a float64) {
			if x < 0 || x >= imgW || y < 0 || y >= imgH {
				return
			}
			if v := uint8(a * 255); v > mask[y*imgW+x] {
				mask[y*imgW+x] = v
			}
		})
}

// canopyArt returns the sprite for a tall flora prop (trees pick a variant by
// position), and whether the prop is a canopy at all — shared by the shadow and
// body passes so both agree on the shape.
func canopyArt(p TileProp, wx, wy int) ([]string, bool) {
	switch p {
	case PropTree:
		return pickTreeArt(wx, wy), true
	case PropAcacia:
		return acaciaArt, true
	case PropPalm:
		return palmArt, true
	case PropFir:
		return firArt, true
	case PropCrag:
		return cragArt, true
	}
	return nil, false
}

// accumCanopyShadow records a tall prop's contact shadow into a max-coverage
// mask (alpha 0..255) instead of darkening directly, so overlapping tree
// shadows don't stack into ever-darker blobs — the densest stand casts a single
// even shadow. applyShadowMask then darkens the frame once.
func accumCanopyShadow(mask []uint8, imgW, imgH, vx, vy, scale int, art []string) {
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	w := len(art[0])
	shadowBlocks(float64(vx*scale+scale/2), float64((vy+1)*scale)-float64(apx),
		float64(w*apx)*0.38, float64(apx)*1.4, apx, float64(len(art)*apx),
		func(x, y int, a float64) {
			if x < 0 || x >= imgW || y < 0 || y >= imgH {
				return
			}
			if v := uint8(a * 255); v > mask[y*imgW+x] {
				mask[y*imgW+x] = v
			}
		})
}

// applyShadowMask darkens the frame once by the accumulated shadow coverage.
func applyShadowMask(img *image.RGBA, mask []uint8) {
	for i, v := range mask {
		if v == 0 {
			continue
		}
		a := float64(v) / 255
		o := i * 4
		img.Pix[o] = uint8(float64(img.Pix[o]) * (1 - a))
		img.Pix[o+1] = uint8(float64(img.Pix[o+1]) * (1 - a))
		img.Pix[o+2] = uint8(float64(img.Pix[o+2]) * (1 - a))
	}
}

// drawCanopy renders a tall flora sprite (art) centered on tile (vx,vy),
// bottom-aligned so the trunk sits on the tile and the crown overhangs upward,
// in its color (P body, p shade/rim, L dapple, W glint/snow, T trunk). The
// contact shadow is drawn separately by drawCanopyShadow.
func drawCanopy(img *image.RGBA, vx, vy, scale int, col colorful.Color, art []string, wx, wy int, amb colorful.Color, ambStr float64) {
	apx := scale / tileArtN
	if apx < 1 {
		apx = 1
	}
	w := len(art[0])
	left := vx*scale + scale/2 - (w*apx)/2
	top := (vy+1)*scale - len(art)*apx

	body := colorfulToRGBA(col)
	shade := colorfulToRGBA(col.BlendLab(shadowColor, 0.34).Clamped())
	dark := colorfulToRGBA(col.BlendLab(shadowColor, 0.52).Clamped())
	dapple := colorfulToRGBA(col.BlendLab(spriteWhite, 0.30).Clamped())
	trunk := colorfulToRGBA(trunkColor)
	for ay, row := range art {
		for ax := 0; ax < len(row); ax++ {
			var c color.RGBA
			ok := true
			switch row[ax] {
			case 'P':
				c = body
			case 'p':
				// The shade pixels form the canopy rim; coherently dither them away
				// so silhouettes feather and neighboring crowns blend into one mass
				// instead of reading as discrete lollipops.
				c = shade
				if valueNoise(wx*tileArtN+ax, wy*tileArtN+ay) < 0.42 {
					ok = false
				}
			case 'L':
				c = dapple
			case 'D':
				c = dark // solid shadow face (no dither) — for rock crags
			case 'W':
				// Snow tip / glint — tinted by the day/night ambient like the rest,
				// so fir caps don't glow pure white at night.
				c = colorfulToRGBA(tint(spriteWhite, amb, ambStr).Clamped())
			case 'T':
				c = trunk
			default:
				ok = false
			}
			if ok {
				fillRect(img, left+ax*apx, top+ay*apx, apx, apx, c)
			}
		}
	}
}

// pickTreeArt chooses a tree variant deterministically by world position, so a
// stand mixes broad, young and conifer shapes without flicker as the camera
// scrolls. Oaks dominate, conifers are common, small trees fill in.
func pickTreeArt(wx, wy int) []string {
	switch r := hashNoise(wx, wy, 0x7733); {
	case r < 0.55:
		return treeArt[0] // broad oak
	case r < 0.80:
		return treeArt[2] // conifer
	default:
		return treeArt[1] // small
	}
}

// portalPixel returns the color for an art pixel of an active gate's swirl. A
// 3-arm spiral (angle + radius from the gate's centre) rotates with the frame
// and pulses outward, banded to three flat colors of the style's portal ramp —
// a chunky, clearly-animated retro gate rather than a smooth gradient.
func portalPixel(ax, ay, frame int, pal ui.Palette) color.RGBA {
	dx, dy := float64(ax)-5.5, float64(ay)-5.5 // portalArt is 12×12 art-pixels
	ang := math.Atan2(dy, dx)
	r := math.Hypot(dx, dy)
	s := 0.5 + 0.5*math.Sin(3*ang+r*0.9-float64(frame)*0.5)
	band := math.Floor(s * 3)
	if band > 2 {
		band = 2
	}
	return colorfulToRGBA(mustHex(string(ui.Blend(pal.PortalA, pal.PortalB, band/2))))
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

// blitGround paints a tile's ground surface like blitTileArt, but additionally
// dithers the tile's outer art-pixels toward a neighboring biome's color when
// it differs — the dual-grid idea (neighbors decide the seam) expressed as a
// live per-pixel blend, so hard grid edges become organic transitions
// (coastlines, forest edges, path shoulders) with no authored transition art.
//
// The dither threshold comes from a low-frequency coherent noise field sampled
// in world space, not white noise: that makes the seam waver in contiguous
// lobes (peninsulas and inlets) rather than salt-and-pepper static, while the
// per-art-pixel blocks keep the look crisply retro. art may be nil for a flat
// surface.
func blitGround(img *image.RGBA, ox, oy, scale int, art []string, base, light, dark colorful.Color, nbr [8]colorful.Color, wx, wy int) {
	// Coverage by penetration depth: most of the edge pixels flip, narrowing as
	// the neighbor reaches deeper in, so lobes round off into peninsulas.
	seamThresh := [seamBand + 1]float64{0.45, 0.68, 0.88}
	for iy := 0; iy < scale; iy++ {
		ay := iy * tileArtN / scale
		for ix := 0; ix < scale; ix++ {
			ax := ix * tileArtN / scale
			col := base
			if art != nil {
				switch art[ay][ax] {
				case 'L':
					col = light
				case 'D':
					col = dark
				}
			}
			if target, depth, ok := edgeNeighbor(ax, ay, base, nbr); ok {
				if valueNoise(wx*tileArtN+ax, wy*tileArtN+ay) > seamThresh[depth] {
					col = target
				}
			}
			img.SetRGBA(ox+ix, oy+iy, colorfulToRGBA(col))
		}
	}
}

// seamBand is how many art-pixels deep a neighboring biome can flood across a
// tile edge (the transition width).
const seamBand = 2

// edgeNeighbor picks the differing neighbor a border art-pixel (ax,ay) should
// dither toward, and that pixel's penetration depth (0 = on the edge) so the
// caller can taper coverage with depth. Returns ok=false for interior pixels or
// when every touched neighbor is the same biome (so interiors stay seamless).
// nbr is ordered N, NE, E, SE, S, SW, W, NW.
func edgeNeighbor(ax, ay int, base colorful.Color, nbr [8]colorful.Color) (colorful.Color, int, bool) {
	dN, dS, dW, dE := ay, tileArtN-1-ay, ax, tileArtN-1-ax
	// A diagonal pixel's depth is the deeper of its two edge distances, so the
	// flood rounds off at corners instead of squaring them.
	depth := [8]int{dN, max(dN, dE), dE, max(dS, dE), dS, max(dS, dW), dW, max(dN, dW)}
	best, bestDepth := -1, seamBand+1
	for i := 0; i < 8; i++ {
		if depth[i] > seamBand || base.DistanceLab(nbr[i]) < 0.07 {
			continue
		}
		if depth[i] < bestDepth {
			best, bestDepth = i, depth[i]
		}
	}
	if best < 0 {
		return base, 0, false
	}
	return nbr[best], bestDepth, true
}

// valueNoise is coherent [0,1) noise for seam dithering: two octaves of
// smoothstep value noise (fbm). The low octave (≈one cell per tile) sets the
// big lobes; the high octave at half amplitude superimposes small notches, so
// boundaries get multi-scale detail — deep bays and tiny jags — instead of one
// uniform scallop rhythm. Thresholding it still yields hard per-pixel blocks,
// so the look stays retro.
func valueNoise(x, y int) float64 {
	// 0.667 + 0.333 keeps the sum in [0,1); seeds differ so octaves don't align.
	return 0.667*latticeNoise(x, y, 7.0, 0x51) + 0.333*latticeNoise(x, y, 3.0, 0xB7)
}

// latticeNoise is one octave of smoothstep-interpolated value noise: hashNoise
// sampled on an integer lattice spaced denom art-pixels apart, seeded by seed.
func latticeNoise(x, y int, denom float64, seed int) float64 {
	fx, fy := float64(x)/denom, float64(y)/denom
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := fx-float64(x0), fy-float64(y0)
	tx = tx * tx * (3 - 2*tx) // smoothstep
	ty = ty * ty * (3 - 2*ty)
	return bilerp(
		hashNoise(x0, y0, seed),
		hashNoise(x0+1, y0, seed),
		hashNoise(x0, y0+1, seed),
		hashNoise(x0+1, y0+1, seed),
		tx, ty)
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
