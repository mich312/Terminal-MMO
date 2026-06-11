// Package ui holds the shared visual language of Durst World: one palette,
// one set of lipgloss styles, a few small rendering helpers. No other
// package defines colors.
package ui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

// Palette. Restrained: grays, white, one Durst-blue accent, 8 avatar colors.
var (
	ColorAccent   = lipgloss.Color("39")  // Durst blue
	ColorBright   = lipgloss.Color("255") // near-white
	ColorText     = lipgloss.Color("250")
	ColorDim      = lipgloss.Color("242") // walls, decor
	ColorFaint    = lipgloss.Color("236") // floor dots
	ColorWarn     = lipgloss.Color("214")
	ColorPortalA  = lipgloss.Color("39")  // portal pulse phase A
	ColorPortalB  = lipgloss.Color("123") // portal pulse phase B
	ColorBarBg    = lipgloss.Color("236")
	ColorBarText  = lipgloss.Color("252")
	ColorToast    = lipgloss.Color("243") // join/leave one-liners
	ColorPanelBor = lipgloss.Color("39")
)

// avatarColors are 8 readable ANSI 256 colors for player glyphs.
var avatarColors = []lipgloss.Color{
	"203", // coral
	"114", // green
	"215", // amber
	"111", // sky
	"176", // orchid
	"80",  // teal
	"229", // pale yellow
	"153", // light blue
}

// AvatarColor returns a deterministic color for a player name.
func AvatarColor(name string) lipgloss.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return avatarColors[h.Sum32()%uint32(len(avatarColors))]
}

// Shared styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorBright).
			Background(ColorAccent).
			Bold(true).
			Padding(0, 2)

	StatusStyle = lipgloss.NewStyle().
			Foreground(ColorBarText).
			Background(ColorBarBg).
			Padding(0, 1)

	StatusHintStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Background(ColorBarBg).
			Bold(true)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPanelBor).
			Padding(1, 2)

	PanelTitleStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	WallStyle   = lipgloss.NewStyle().Foreground(ColorDim)
	FloorStyle  = lipgloss.NewStyle().Foreground(ColorFaint)
	DecorStyle  = lipgloss.NewStyle().Foreground(ColorDim)
	ObjectStyle = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	LabelStyle  = lipgloss.NewStyle().Foreground(ColorText)

	PortalStyleA = lipgloss.NewStyle().Foreground(ColorPortalA).Bold(true)
	PortalStyleB = lipgloss.NewStyle().Foreground(ColorPortalB).Bold(true)

	ChatNameStyle = lipgloss.NewStyle().Bold(true)
	ChatTextStyle = lipgloss.NewStyle().Foreground(ColorText)
	ToastStyle    = lipgloss.NewStyle().Foreground(ColorToast).Italic(true)

	DimStyle    = lipgloss.NewStyle().Foreground(ColorDim)
	FaintStyle  = lipgloss.NewStyle().Foreground(ColorFaint)
	BrightStyle = lipgloss.NewStyle().Foreground(ColorBright).Bold(true)
	AccentStyle = lipgloss.NewStyle().Foreground(ColorAccent)
	WarnStyle   = lipgloss.NewStyle().Foreground(ColorWarn)
)
