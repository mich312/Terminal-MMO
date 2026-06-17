// Package palette is the single source of Durst World's truecolor hex palette.
// It is a leaf package (no imports) so both ui (which builds lipgloss styles
// from it) and markdown (which colors slide spans) can share the same values
// without duplicating literals or creating an import cycle.
//
// Restrained: deep slate, near-white, the Durst blue→cyan accent ramp, one
// warn amber, plus the panel/code backgrounds and a link blue.
package palette

const (
	Accent  = "#2E8BFF" // Durst blue
	Accent2 = "#7DF0FF" // cyan tip of the accent ramp
	Bright  = "#F5F7FA" // near-white
	Text    = "#C2CBD6"
	Dim     = "#6B7480" // walls, decor
	Faint   = "#333A45" // floor dots
	Warn    = "#FFB454"
	PortalA = "#2E8BFF" // portal pulse phase A
	PortalB = "#7DF0FF" // portal pulse phase B
	BarBg   = "#1B2027"
	BarText = "#C2CBD6"
	Toast   = "#8A93A0" // join/leave one-liners
	PanelBg = "#11151B"
	CodeBg  = "#171C28" // code-block / inline-code background
	Link    = "#6FB7FF" // markdown links
)
