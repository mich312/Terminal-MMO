// Package ui holds the shared visual language of Durst World: the lipgloss
// styles and a few small rendering helpers. The hex palette itself lives in the
// leaf package palette (shared with markdown); ui builds styles from it.
//
// Color is truecolor-first: the palette is authored as 24-bit hex and each
// SSH session renders through its own *lipgloss.Renderer (see NewTheme),
// which auto-detects the client's terminal and downsamples to 256- or
// 16-color as needed. Code that has no session (tests, init-time globals)
// uses Default, bound to the process renderer.
package ui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/palette"
)

// Hex* alias the shared palette so existing callers (and the styles below)
// keep their names; palette is the single source of these values.
const (
	HexAccent  = palette.Accent
	HexAccent2 = palette.Accent2
	HexBright  = palette.Bright
	HexText    = palette.Text
	HexDim     = palette.Dim
	HexFaint   = palette.Faint
	HexWarn    = palette.Warn
	HexPortalA = palette.PortalA
	HexPortalB = palette.PortalB
	HexBarBg   = palette.BarBg
	HexBarText = palette.BarText
	HexToast   = palette.Toast
	HexPanelBg = palette.PanelBg
	HexCodeBg  = palette.CodeBg
)

// Back-compat color vars used by a few callers directly.
var (
	ColorAccent = lipgloss.Color(HexAccent)
	ColorBarBg  = lipgloss.Color(HexBarBg)
)

// avatarColors are 8 readable truecolor hues for player glyphs.
var avatarColors = []lipgloss.Color{
	"#D97757", // claude clay
	"#FF6B6B", // coral
	"#7BD88F", // green
	"#FFC861", // amber
	"#6FB7FF", // sky
	"#C792EA", // orchid
	"#4FD6BE", // teal
	"#F2E9A0", // pale yellow
	"#A0C7FF", // light blue
	"#FF8FB1", // pink
	"#9B7EDE", // purple
	"#5ED3F3", // cyan
	"#FFB870", // tangerine
	"#8BD450", // lime
	"#E27396", // rose
	"#76C7C0", // seafoam
	"#B5838D", // mauve
	"#F4A259", // ochre
	"#5FA8D3", // steel blue
	"#C0EB75", // chartreuse
	"#FF9F1C", // marigold
	"#E0AAFF", // lilac
}

// AvatarColor returns a deterministic color for a player name.
func AvatarColor(name string) lipgloss.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return avatarColors[h.Sum32()%uint32(len(avatarColors))]
}

// NumAvatarColors is how many avatar colors exist (for /color).
func NumAvatarColors() int { return len(avatarColors) }

// AvatarColorByIndex returns the i-th avatar color, wrapping out-of-range i.
func AvatarColorByIndex(i int) lipgloss.Color {
	n := len(avatarColors)
	return avatarColors[((i%n)+n)%n]
}

// Theme is the full set of styles bound to one renderer (one SSH session, or
// the process default). Build it with NewTheme.
type Theme struct {
	r *lipgloss.Renderer

	Title      lipgloss.Style
	Status     lipgloss.Style
	StatusHint lipgloss.Style
	Panel      lipgloss.Style
	PanelTitle lipgloss.Style

	Wall   lipgloss.Style
	Floor  lipgloss.Style
	Decor  lipgloss.Style
	Object lipgloss.Style
	Label  lipgloss.Style

	PortalA lipgloss.Style
	PortalB lipgloss.Style

	ChatName lipgloss.Style
	ChatText lipgloss.Style
	Toast    lipgloss.Style

	Dim    lipgloss.Style
	Faint  lipgloss.Style
	Bright lipgloss.Style
	Accent lipgloss.Style
	Warn   lipgloss.Style
}

// NewTheme builds the style set for a renderer. Pass bubbletea.MakeRenderer(s)
// for a per-session, auto-detecting theme.
func NewTheme(r *lipgloss.Renderer) *Theme {
	s := r.NewStyle
	return &Theme{
		r: r,
		Title: s().Foreground(lipgloss.Color(HexBright)).
			Background(lipgloss.Color(HexAccent)).Bold(true).Padding(0, 2),
		Status: s().Foreground(lipgloss.Color(HexBarText)).
			Background(lipgloss.Color(HexBarBg)).Padding(0, 1),
		StatusHint: s().Foreground(lipgloss.Color(HexAccent)).
			Background(lipgloss.Color(HexBarBg)).Bold(true),
		Panel: s().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(HexAccent)).
			Background(lipgloss.Color(HexPanelBg)).Padding(1, 2),
		PanelTitle: s().Foreground(lipgloss.Color(HexAccent)).Bold(true),

		Wall:   s().Foreground(lipgloss.Color(HexDim)),
		Floor:  s().Foreground(lipgloss.Color(HexFaint)),
		Decor:  s().Foreground(lipgloss.Color(HexDim)),
		Object: s().Foreground(lipgloss.Color(HexAccent)).Bold(true),
		Label:  s().Foreground(lipgloss.Color(HexText)),

		PortalA: s().Foreground(lipgloss.Color(HexPortalA)).Bold(true),
		PortalB: s().Foreground(lipgloss.Color(HexPortalB)).Bold(true),

		ChatName: s().Bold(true),
		ChatText: s().Foreground(lipgloss.Color(HexText)),
		Toast:    s().Foreground(lipgloss.Color(HexToast)).Italic(true),

		Dim:    s().Foreground(lipgloss.Color(HexDim)),
		Faint:  s().Foreground(lipgloss.Color(HexFaint)),
		Bright: s().Foreground(lipgloss.Color(HexBright)).Bold(true),
		Accent: s().Foreground(lipgloss.Color(HexAccent)),
		Warn:   s().Foreground(lipgloss.Color(HexWarn)),
	}
}

// Fg is a renderer-bound foreground style for an arbitrary color — used by
// the map renderer, which computes per-tile colors (lighting, day/night,
// animation) rather than picking from the fixed style set.
func (t *Theme) Fg(c lipgloss.Color) lipgloss.Style { return t.r.NewStyle().Foreground(c) }

// FgBold is Fg with bold, for objects and portals.
func (t *Theme) FgBold(c lipgloss.Color) lipgloss.Style {
	return t.r.NewStyle().Foreground(c).Bold(true)
}

// FgBg styles a cell with both a foreground and background color — used to
// pack two half-block pixels (top=fg, bottom=bg) into one cell.
func (t *Theme) FgBg(fg, bg lipgloss.Color) lipgloss.Style {
	return t.r.NewStyle().Foreground(fg).Background(bg)
}

// Wrap is a renderer-bound, single-line block style (no color) that pads to
// the full screen width. MaxHeight(1) clips overflow so an over-long bar can
// never wrap and disturb the fixed layout.
func (t *Theme) Wrap(width int) lipgloss.Style {
	return t.r.NewStyle().Width(width).MaxHeight(1)
}

// Bar is Wrap plus the status-bar background.
func (t *Theme) Bar(width int) lipgloss.Style {
	return t.r.NewStyle().Width(width).MaxHeight(1).Background(lipgloss.Color(HexBarBg))
}

// Screen is a presentation "big screen": a rounded accent border with padding
// and no fill, so a rendered markdown slide reads cleanly over the scene.
func (t *Theme) Screen() lipgloss.Style {
	return t.r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(HexAccent)).
		Padding(1, 2)
}

// Default is the process-wide theme for tests and init-time globals. Sessions
// build their own with NewTheme(bubbletea.MakeRenderer(s)).
var Default = NewTheme(lipgloss.DefaultRenderer())

// Back-compat global styles — thin aliases to Default so existing callers
// (areas, input widget) keep compiling. New code should prefer a session
// Theme threaded through game.Ctx.
var (
	TitleStyle      = Default.Title
	StatusStyle     = Default.Status
	StatusHintStyle = Default.StatusHint
	PanelStyle      = Default.Panel
	PanelTitleStyle = Default.PanelTitle

	WallStyle   = Default.Wall
	FloorStyle  = Default.Floor
	DecorStyle  = Default.Decor
	ObjectStyle = Default.Object
	LabelStyle  = Default.Label

	PortalStyleA = Default.PortalA
	PortalStyleB = Default.PortalB

	ChatNameStyle = Default.ChatName
	ChatTextStyle = Default.ChatText
	ToastStyle    = Default.Toast

	DimStyle    = Default.Dim
	FaintStyle  = Default.Faint
	BrightStyle = Default.Bright
	AccentStyle = Default.Accent
	WarnStyle   = Default.Warn
)

// Blend mixes two hex colors in CIE-Lab space and returns a lipgloss color.
// t=0 yields a, t=1 yields b.
func Blend(a, b string, t float64) lipgloss.Color {
	ca, err1 := colorful.Hex(a)
	cb, err2 := colorful.Hex(b)
	if err1 != nil || err2 != nil {
		return lipgloss.Color(a)
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return lipgloss.Color(ca.BlendLab(cb, t).Clamped().Hex())
}
