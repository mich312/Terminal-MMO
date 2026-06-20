package ui

import (
	"image"

	"github.com/lucasb-eyer/go-colorful"
)

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

	// Recolor, when non-nil, supersedes Map and rewrites the finished frame with
	// full knowledge of pixel position — so a few-tone monochrome style (gameboy)
	// can order-dither between shades and overlay an LCD dot-matrix grid, neither
	// of which a per-pixel color map can express. apx is the on-screen size of one
	// source art pixel (for aligning the grid); salient(px) reports whether pixel
	// index px belongs to a gameplay element (collectible, hat, portal, gate,
	// avatar) so those can be kept on legible, reserved shades. The renderer only
	// builds the salience mask when this is set, so other styles pay nothing.
	Recolor func(img *image.RGBA, apx int, salient func(px int) bool)
}
