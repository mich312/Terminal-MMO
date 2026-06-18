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
		s.Palette = ui.Palette{Name: "gameboy", PortalA: "#306230", PortalB: "#9BBC0F", Map: gameboyMap}
	case "neon":
		s.Palette = ui.Palette{Name: "neon", PortalA: "#1B2CFF", PortalB: "#39FFF6", Map: neonMap}
	}
	return s
}

// gbShades is the 4-tone Game Boy DMG green ramp, darkest first.
var gbShades = []colorful.Color{
	mustHex("#0F380F"), mustHex("#306230"), mustHex("#8BAC0F"), mustHex("#9BBC0F"),
}

// gameboyMap maps any color to the nearest DMG green by luminance.
func gameboyMap(c colorful.Color) colorful.Color {
	l := 0.299*c.R + 0.587*c.G + 0.114*c.B
	idx := int(l * float64(len(gbShades)))
	if idx >= len(gbShades) {
		idx = len(gbShades) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return gbShades[idx]
}

// neonMap pushes saturation and a slight lift for a synthwave glow.
func neonMap(c colorful.Color) colorful.Color {
	h, s, l := c.Hsl()
	s = math.Min(1, s*1.6+0.12)
	l = math.Min(0.86, l*1.04+0.04)
	return colorful.Hsl(h, s, l).Clamped()
}
