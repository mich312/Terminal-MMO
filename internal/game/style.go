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

// gbLuma is the perceptual lightness of a color, 0..1. It linearizes the sRGB
// channels, takes the Rec.709 luminance, then re-encodes through a gamma so the
// value is spaced by perceived brightness — a hard split on the raw (gamma-
// encoded) channels crushes shadows together and washes highlights out, which
// left most natural terrain dumped into a single shade. The re-encode puts the
// ramp thresholds at visually even steps so the four tones are actually used.
func gbLuma(c colorful.Color) float64 {
	lin := func(u float64) float64 {
		if u <= 0.04045 {
			return u / 12.92
		}
		return math.Pow((u+0.055)/1.055, 2.4)
	}
	y := 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
	return math.Pow(y, 1.0/2.2)
}

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

// gameboyShadeLevel places a color on the DMG ramp as a continuous level, split
// by salience so gameplay elements stay readable: terrain and scenery ride the
// two middle shades (level 1..2, the fractional part driving dithering), while
// collectibles, hats, portals, gates and avatars snap to the reserved dark/light
// ends (level 0 or 3) as crisp high-contrast sprites — the classic Game Boy
// background/sprite separation. The two sets share no shade, so an item can never
// vanish into same-luminance terrain.
func gameboyShadeLevel(c colorful.Color, salient bool) float64 {
	l := gbLuma(c)
	if salient {
		// Lean bright: a lower split sends more of a sprite to the light shade so
		// items read as bright shapes, with only their shadows/outline going dark.
		if l >= gbSpriteSplit {
			return 3 // lightest — sprite highlight
		}
		return 0 // darkest — sprite body/outline
	}
	return 1 + clamp01(l) // mid-tone terrain band, shade 1 → 2
}

// gameboyMapSalient is the un-dithered shade for a color — gameboyShadeLevel
// rounded to the nearest DMG shade. Retained as the simple, position-independent
// mapping (and the unit of the readability/separation tests).
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
	W := img.Bounds().Dx()
	p := img.Pix
	for i := 0; i+3 < len(p); i += 4 {
		pi := i / 4
		x, y := pi%W, pi/W
		c := colorful.Color{R: float64(p[i]) / 255, G: float64(p[i+1]) / 255, B: float64(p[i+2]) / 255}

		lvl := gameboyShadeLevel(c, salient(pi))
		idx := int(lvl)
		// Dither at the source-art-pixel grid (the Game Boy's native pixel), not
		// per device pixel: each art pixel becomes one uniform shade and adjacent
		// ones checker, which is both the authentic chunky DMG dither and far
		// kinder to sixel RLE (runs stay ~apx long instead of alternating every px).
		if frac := lvl - float64(idx); frac > gbBayer[(y/apx)&3][(x/apx)&3] {
			idx++ // order-dither up to the next shade
		}
		if idx < 0 {
			idx = 0
		}
		if idx >= len(gbShades) {
			idx = len(gbShades) - 1
		}
		out := gbShades[idx]

		// LCD dot-matrix: darken just the lattice intersections (a sparse dot at
		// each source-pixel corner). Dots rather than full grid lines keep the long
		// horizontal runs sixel RLE relies on, so the handheld texture costs a
		// fraction of the bandwidth a full grid would.
		if apx > 1 && x%apx == 0 && y%apx == 0 {
			out = out.BlendLab(shadowColor, gbLCDGrid).Clamped()
		}
		p[i], p[i+1], p[i+2] = f2b(out.R), f2b(out.G), f2b(out.B)
	}
}

// neonMap pushes saturation and a slight lift for a synthwave glow.
func neonMap(c colorful.Color) colorful.Color {
	h, s, l := c.Hsl()
	s = math.Min(1, s*1.6+0.12)
	l = math.Min(0.86, l*1.04+0.04)
	return colorful.Hsl(h, s, l).Clamped()
}
