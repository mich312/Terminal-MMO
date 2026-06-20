package game

import (
	"image"
	"math"
	"strings"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
)

// clamp01 constrains v to [0,1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Style is the art direction for the HD ("real pixel") renderer: the ground and
// prop sprite sets, the multi-tile portal art, a few shading mix factors, and a
// Palette (portal colors + an optional whole-frame recolor). It only drives
// RenderRGBA — the half-block glyph renderer is unaffected.
type Style struct {
	Palette ui.Palette

	Ground map[TileTex][][]string // surface sprites per texture (multiple variants)
	Props  map[TileProp][]string  // decoration sprites per prop
	Portal []string               // multi-tile animated gate art
	Trunk  colorful.Color         // tree-trunk color (prop code 'T')

	GroundLightMix float64 // blend ground 'L' pixels toward white
	GroundDarkMix  float64 // blend ground 'D' pixels toward shadow
	PropShadeMix   float64 // blend prop 'p' pixels toward shadow
	Vignette       float64 // smooth-mode radial edge darkening (0 = none)
}

// tint resolves a tile/prop hex color to a working color. A style's overall
// look (monochrome, neon …) is applied once over the finished frame through
// Palette.Map, so tint itself is just a hex parse.
func (s *Style) tint(hex string) colorful.Color { return mustHex(hex) }

// DefaultStyle is the shipped look: the authored 6×6 tileset in full color, no
// recolor pass. RenderRGBA falls back to it when given a nil style.
func DefaultStyle() *Style {
	return &Style{
		Palette:        ui.Palette{Name: "default", PortalA: ui.HexPortalA, PortalB: ui.HexPortalB},
		Ground:         groundArt,
		Props:          propArt,
		Portal:         portalArt,
		Trunk:          trunkColor,
		GroundLightMix: 0.20,
		GroundDarkMix:  0.26,
		PropShadeMix:   0.35,
		Vignette:       0.35,
	}
}

// StyleByName resolves an art-style name (default | gameboy | neon). The sprite
// sets are shared; the named styles differ only in their palette recolor pass.
// An unknown or empty name falls back to the default.
func StyleByName(name string) *Style {
	s := DefaultStyle()
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gameboy", "gb":
		s.Palette = ui.Palette{Name: "gameboy", PortalA: "#306230", PortalB: "#9BBC0F", Recolor: gameboyRecolor}
	case "neon":
		s.Palette = ui.Palette{Name: "neon", PortalA: "#1B2CFF", PortalB: "#39FFF6", Map: neonMap}
	}
	return s
}

// gbShades is the 4-tone Game Boy DMG green ramp, darkest first.
var gbShades = []colorful.Color{
	mustHex("#0F380F"), mustHex("#306230"), mustHex("#8BAC0F"), mustHex("#9BBC0F"),
}

// The gameboy recolor touches every pixel of every frame, so its perceptual
// luminance and output shades are fully table-driven — no math.Pow or
// colorful.Color in the hot loop. gbLin{R,G,B} fold the sRGB→linear transform
// and the Rec.709 weight into one lookup per channel byte; gbGamma re-encodes the
// summed linear luminance through 1/2.2; gbShadeRGB / gbShadeGridRGB are the ramp
// shades (and their LCD-dot-darkened variants) precomputed as bytes.
const gbGammaN = 1024

var (
	gbLinR, gbLinG, gbLinB [256]float64
	gbGamma                [gbGammaN]float64
	gbShadeRGB             [4][3]uint8
	gbShadeGridRGB         [4][3]uint8
)

func init() {
	lin := func(u float64) float64 {
		if u <= 0.04045 {
			return u / 12.92
		}
		return math.Pow((u+0.055)/1.055, 2.4)
	}
	for i := 0; i < 256; i++ {
		u := lin(float64(i) / 255)
		gbLinR[i] = 0.2126 * u
		gbLinG[i] = 0.7152 * u
		gbLinB[i] = 0.0722 * u
	}
	for i := 0; i < gbGammaN; i++ {
		gbGamma[i] = math.Pow(float64(i)/(gbGammaN-1), 1.0/2.2)
	}
	for i, s := range gbShades {
		r, g, b := f2b(s.R), f2b(s.G), f2b(s.B)
		gbShadeRGB[i] = [3]uint8{r, g, b}
		d := s.BlendLab(shadowColor, gbLCDGrid).Clamped()
		gbShadeGridRGB[i] = [3]uint8{f2b(d.R), f2b(d.G), f2b(d.B)}
	}
}

// gbLumaBytes is the perceptual lightness (0..1) of an sRGB pixel, table-driven:
// linearize+weight each channel, sum, then re-encode through 1/2.2. A hard split
// on the raw (gamma-encoded) channels crushes shadows together and washes
// highlights out, dumping most natural terrain into a single shade; the perceptual
// scale puts the ramp thresholds at visually even steps.
func gbLumaBytes(r, g, b uint8) float64 {
	y := gbLinR[r] + gbLinG[g] + gbLinB[b] // linear luminance, 0..1
	return gbGamma[int(y*(gbGammaN-1))]
}

// gbLuma is gbLumaBytes for a colorful.Color (the map/test entry points).
func gbLuma(c colorful.Color) float64 { return gbLumaBytes(f2b(c.R), f2b(c.G), f2b(c.B)) }

// gameboyMap maps any color to the nearest DMG green by luminance — the plain
// 4-tone collapse, kept as the salience-unaware fallback and for tests.
func gameboyMap(c colorful.Color) colorful.Color {
	idx := int(gbLuma(c) * float64(len(gbShades)))
	if idx >= len(gbShades) {
		idx = len(gbShades) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return gbShades[idx]
}

// gameboyShadeLevel places a color on the DMG ramp as a continuous level. Terrain
// spans the upper three shades (level 1..3, the fractional part driving dither);
// salient sprites take a bright 2-tone interior (level 2..3). The darkest shade
// (level 0) is never returned here — it is reserved by the recolor for the sprite
// outline, the tone that makes gameplay elements read against any background.
func gameboyShadeLevel(c colorful.Color, salient bool) float64 {
	l := gbLuma(c)
	if salient {
		if l >= gbSpriteSplit {
			return 3 // lightest — sprite highlight
		}
		return 2 // mid-light — sprite body
	}
	lvl := 1 + 2*clamp01(l)
	if lvl > 3 {
		lvl = 3
	}
	return lvl // terrain: shade 1 (water) → 2 (grass) → 3 (sand)
}

// gameboyMapSalient is the un-dithered, outline-free shade for a color —
// gameboyShadeLevel rounded to the nearest DMG shade. Retained as the simple,
// position-independent mapping (and the unit of the readability tests); the
// rendered look adds dithering and sprite outlines on top in gameboyRecolor.
func gameboyMapSalient(c colorful.Color, salient bool) colorful.Color {
	idx := int(gameboyShadeLevel(c, salient) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(gbShades) {
		idx = len(gbShades) - 1
	}
	return gbShades[idx]
}

// gbSpriteSplit is the perceptual-luma split between a sprite's dark and light
// shade, biased dark so sprites skew bright and pop against the terrain.
const gbSpriteSplit = 0.42

// gbBayer is the normalized 4×4 ordered-dither matrix (thresholds in 0..1). The
// fractional ramp level is compared against it per pixel, so a terrain tone
// halfway between two shades becomes a fine checker of both — the authentic Game
// Boy way to fake intermediate shades from a 4-tone palette.
var gbBayer = [4][4]float64{
	{0.5 / 16, 8.5 / 16, 2.5 / 16, 10.5 / 16},
	{12.5 / 16, 4.5 / 16, 14.5 / 16, 6.5 / 16},
	{3.5 / 16, 11.5 / 16, 1.5 / 16, 9.5 / 16},
	{15.5 / 16, 7.5 / 16, 13.5 / 16, 5.5 / 16},
}

// gbLCDGrid is how far an LCD dot-matrix grid line is blended toward shadow —
// a faint lattice on the source-pixel boundaries that evokes the DMG screen.
const gbLCDGrid = 0.16

// gameboyRecolor rewrites a finished frame into the DMG look: each pixel is
// placed on the 4-tone ramp by gameboyShadeLevel, ordered-dithered between
// adjacent shades (so terrain gains apparent tones the 4-tone palette can't hold
// flatly), then a faint dot-matrix grid is laid on the art-pixel lattice for the
// handheld-LCD feel. apx is the on-screen size of one source art pixel; salient
// flags gameplay pixels, which stay crisp (their integer level never dithers).
func gameboyRecolor(img *image.RGBA, apx int, salient func(px int) bool) {
	b := img.Bounds()
	W, H := b.Dx(), b.Dy()
	p := img.Pix
	for i := 0; i+3 < len(p); i += 4 {
		pi := i / 4
		x, y := pi%W, pi/W
		l := gbLumaBytes(p[i], p[i+1], p[i+2])

		var idx int
		if salient(pi) {
			// Sprites get a dark outline (the reserved darkest shade) wherever they
			// meet terrain, with a bright 2-tone interior. The outline is what makes
			// items read: terrain never uses shade 0, so a collectible, hat, portal or
			// avatar is always ringed by a tone no background can match — legible over
			// light terrain (where reserving a light shade failed, DMG shades 2 and 3
			// being nearly identical) and dark terrain alike.
			switch {
			case gbSpriteEdge(salient, x, y, W, H, apx):
				idx = 0
			case l >= gbSpriteSplit:
				idx = 3
			default:
				idx = 2
			}
		} else {
			// Terrain spans the upper three shades (level 1..3): water reads dark,
			// grass mid, sand bright. Ordered-dither at the source-art-pixel grid (the
			// Game Boy's native pixel) blends between *adjacent* shades, so the texture
			// is a calm DMG stipple rather than a high-contrast checker — and the
			// chunky art-pixel cells keep sixel RLE runs long.
			lvl := 1 + 2*l
			if lvl > 3 {
				lvl = 3
			}
			idx = int(lvl)
			if idx < 3 && lvl-float64(idx) > gbBayer[(y/apx)&3][(x/apx)&3] {
				idx++
			}
		}

		rgb := gbShadeRGB[idx]
		// LCD dot-matrix: darken just the lattice intersections (a sparse dot at each
		// source-pixel corner). Dots, not full grid lines, keep the long horizontal
		// runs sixel RLE relies on, so the handheld texture is nearly free on the wire.
		if apx > 1 && x%apx == 0 && y%apx == 0 {
			rgb = gbShadeGridRGB[idx]
		}
		p[i], p[i+1], p[i+2] = rgb[0], rgb[1], rgb[2]
	}
}

// gbSpriteEdge reports whether a salient pixel sits on the sprite's border — a
// cardinal neighbour d away is outside the sprite (or the image) — so it can be
// drawn as the dark outline that keeps gameplay elements legible on any terrain.
func gbSpriteEdge(salient func(px int) bool, x, y, W, H, d int) bool {
	return x-d < 0 || !salient(y*W+x-d) ||
		x+d >= W || !salient(y*W+x+d) ||
		y-d < 0 || !salient((y-d)*W+x) ||
		y+d >= H || !salient((y+d)*W+x)
}

// neonMap pushes saturation and a slight lift for a synthwave glow.
func neonMap(c colorful.Color) colorful.Color {
	h, s, l := c.Hsl()
	s = math.Min(1, s*1.6+0.12)
	l = math.Min(0.86, l*1.04+0.04)
	return colorful.Hsl(h, s, l).Clamped()
}
