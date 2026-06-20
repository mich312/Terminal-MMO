package game

import (
	"math"
	"strings"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
)

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
		s.Palette = ui.Palette{Name: "gameboy", PortalA: "#306230", PortalB: "#9BBC0F", MapSalient: gameboyMapSalient}
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

// gameboyMapSalient collapses to the DMG ramp but splits the ramp by salience so
// gameplay elements stay readable: terrain and scenery use the two middle shades
// (a mid-tone backdrop), while collectibles, hats, portals, gates and avatars are
// rendered as crisp high-contrast 2-tone (the darkest and lightest shades) — the
// classic Game Boy background/sprite separation. Because the two sets share no
// shade, an item can never vanish into same-luminance terrain.
func gameboyMapSalient(c colorful.Color, salient bool) colorful.Color {
	l := gbLuma(c)
	if salient {
		// Lean bright: a lower split sends more of a sprite to the light shade so
		// items read as bright shapes, with only their shadows/outline going dark.
		if l >= gbSpriteSplit {
			return gbShades[3] // lightest — sprite highlight
		}
		return gbShades[0] // darkest — sprite body/outline
	}
	if l >= gbTerrainSplit {
		return gbShades[2] // mid-light terrain
	}
	return gbShades[1] // mid-dark terrain
}

// Tuned split points (on the perceptual gbLuma scale) between the two terrain
// shades and the two sprite shades. The sprite split is biased dark so sprites
// skew toward the bright shade and pop; terrain splits at the perceptual midpoint.
const (
	gbTerrainSplit = 0.50
	gbSpriteSplit  = 0.42
)

// neonMap pushes saturation and a slight lift for a synthwave glow.
func neonMap(c colorful.Color) colorful.Color {
	h, s, l := c.Hsl()
	s = math.Min(1, s*1.6+0.12)
	l = math.Min(0.86, l*1.04+0.04)
	return colorful.Hsl(h, s, l).Clamped()
}
