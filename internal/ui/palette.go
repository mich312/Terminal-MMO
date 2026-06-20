package ui

import "github.com/lucasb-eyer/go-colorful"

// Palette names an HD art style's portal colors and an optional whole-frame
// recolor pass. The pixel renderer draws terrain, props and avatars in their
// natural colors, then — if Map is non-nil — runs every finished pixel through
// it in one final pass, which is how the monochrome / neon looks are produced.
// A nil Map leaves the frame untouched (the default, full-color look).
type Palette struct {
	Name    string
	PortalA string // hex, portal swirl phase A
	PortalB string // hex, portal swirl phase B
	Map     func(colorful.Color) colorful.Color

	// MapSalient, when non-nil, supersedes Map and additionally receives whether
	// the pixel belongs to a gameplay-salient element — a collectible, hat,
	// portal, sealed gate or player avatar. A few-tone monochrome style (gameboy)
	// uses it to route those pixels onto reserved, high-contrast shades so they
	// stay legible instead of dissolving into same-luminance terrain. The renderer
	// only builds the per-pixel salience mask when this is set, so the full-color
	// and neon paths pay nothing.
	MapSalient func(c colorful.Color, salient bool) colorful.Color
}
