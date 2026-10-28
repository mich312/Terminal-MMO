// Package ui holds the shared visual language of Durst World: the lipgloss
// styles and a few small rendering helpers. The hex palette itself lives in the
// leaf package palette (shared with markdown); ui builds styles from it.
//
// Color is truecolor-first: the palette is authored as 24-bit hex and styles
// always render full-fidelity ANSI. Downsampling to the client's terminal
// (256- or 16-color) happens at the output layer: Bubble Tea v2 detects each
// SSH session's color profile from its PTY and environment and converts the
// frame as it writes it, so one Theme serves every session.
package ui

import (
	"hash/fnv"
	"image/color"

	"charm.land/lipgloss/v2"
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

// Color is a palette color as authored: a "#rrggbb" hex string, or "" for none
// (see Transparent). Lip Gloss v2 turned lipgloss.Color from a string type into
// a constructor returning an opaque image/color.Color; avatar colors need to
// stay comparable and persistable as text, so this keeps the string identity
// and still satisfies color.Color, so it can be handed to a style directly.
type Color string

// RGBA implements color.Color by parsing the hex string through Lip Gloss.
func (c Color) RGBA() (r, g, b, a uint32) { return lipgloss.Color(string(c)).RGBA() }

// lip converts a color for a Lip Gloss style: a Color goes through
// lipgloss.Color so "" stays "no color" rather than becoming opaque black.
func lip(c color.Color) color.Color {
	if pc, ok := c.(Color); ok {
		return lipgloss.Color(string(pc))
	}
	return c
}

// avatarColors are 8 readable truecolor hues for player glyphs.
var avatarColors = []Color{
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
func AvatarColor(name string) Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return avatarColors[h.Sum32()%uint32(len(avatarColors))]
}

// NumAvatarColors is how many avatar colors exist (for /color).
func NumAvatarColors() int { return len(avatarColors) }

// AvatarColorByIndex returns the i-th avatar color, wrapping out-of-range i.
func AvatarColorByIndex(i int) Color {
	n := len(avatarColors)
	return avatarColors[((i%n)+n)%n]
}

// AvatarColorIndex returns the index of a color in the avatar palette, or 0 if
// it isn't one of them (e.g. a custom color).
func AvatarColorIndex(c Color) int {
	for i, ac := range avatarColors {
		if ac == c {
			return i
		}
	}
	return 0
}

// Theme is the full set of styles. Lip Gloss v2 styles are plain values with no
// renderer attached, so a single Theme (Default) serves every session; the
// type stays so callers keep threading it through game.Ctx.
type Theme struct {
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

// NewTheme builds the style set.
func NewTheme() *Theme {
	s := lipgloss.NewStyle
	return &Theme{
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

// Fg is a foreground style for an arbitrary color — used by the map renderer,
// which computes per-tile colors (lighting, day/night, animation) rather than
// picking from the fixed style set.
func (t *Theme) Fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(lip(c)) }

// FgBold is Fg with bold, for objects and portals.
func (t *Theme) FgBold(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lip(c)).Bold(true)
}

// FgBg styles a cell with both a foreground and background color — used to
// pack two half-block pixels (top=fg, bottom=bg) into one cell.
func (t *Theme) FgBg(fg, bg color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lip(fg)).Background(lip(bg))
}

// PanelWidth is Panel sized the way Lip Gloss v1 sized it: w covers content and
// padding, and the border is drawn outside it. Lip Gloss v2 counts the border
// inside Width, so without this every bordered panel would shrink by two cells.
func (t *Theme) PanelWidth(w int) lipgloss.Style {
	return t.Panel.Width(w + t.Panel.GetHorizontalBorderSize())
}

// Wrap is a single-line block style (no color) that pads to the full screen
// width. MaxHeight(1) clips overflow so an over-long bar can never wrap and
// disturb the fixed layout.
func (t *Theme) Wrap(width int) lipgloss.Style {
	return lipgloss.NewStyle().Width(width).MaxHeight(1)
}

// Bar is Wrap plus the status-bar background.
func (t *Theme) Bar(width int) lipgloss.Style {
	return lipgloss.NewStyle().Width(width).MaxHeight(1).Background(lipgloss.Color(HexBarBg))
}

// Screen is a presentation "big screen": a rounded accent border with padding
// and no fill, so a rendered markdown slide reads cleanly over the scene.
func (t *Theme) Screen() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(HexAccent)).
		Padding(1, 2)
}

// Default is the process-wide theme, shared by every session (styles carry no
// per-terminal state in Lip Gloss v2; see the package comment).
var Default = NewTheme()

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

// Blend mixes two hex colors in CIE-Lab space and returns a palette Color.
// t=0 yields a, t=1 yields b.
func Blend(a, b string, t float64) Color {
	ca, err1 := colorful.Hex(a)
	cb, err2 := colorful.Hex(b)
	if err1 != nil || err2 != nil {
		return Color(a)
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return Color(ca.BlendLab(cb, t).Clamped().Hex())
}
