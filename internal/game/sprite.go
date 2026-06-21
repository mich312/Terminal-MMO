package game

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Sprite bitmaps live in avatar.go (AvatarBitmap), used by the HD pixel
// renderer and the character-panel preview. The glyph renderer can't fit a
// recognizable face in a single-tile footprint, so it draws each player as one
// colored token (their name initial) instead.

var (
	spriteWhite = mustHex("#F5F7FA")
	spriteBlack = mustHex("#0E1116")
	hatMain     = mustHex("#FFD166") // accessory colors (H / h)
	hatShade    = mustHex("#C9962E")
)

// playerColor resolves a player's avatar color to RGB. Avatar colors are hex
// strings, so we parse them directly with colorful.Hex — unlike
// colorful.MakeColor(lipgloss.Color), which relies on the ambient lipgloss
// color profile and returns black when none is detected (e.g. headless, or the
// HD renderer's server-side process). Falls back for non-hex colors.
func playerColor(c lipgloss.Color) colorful.Color {
	if hc, err := colorful.Hex(string(c)); err == nil {
		return hc
	}
	if cf, ok := colorful.MakeColor(c); ok {
		return cf
	}
	return colorful.Color{R: 0.5, G: 0.5, B: 0.5}
}

// stampSprite draws one player's avatar onto the grid. (fc,fr) is the player's
// single-tile footprint cell. A 1×1 body has no room for a half-block sprite,
// so each player is one colored token — their name initial reversed onto the
// body color — with a chevron above your own head to mark you.
func stampSprite(grid [][]rcell, th *ui.Theme, p world.Player, isSelf bool, frame, fc, fr int) {
	body := playerColor(p.Color)
	cell := rcell{ch: nameInitial(p.Name), fg: spriteBlack, bg: body, hasBg: true, bold: true}
	if isSelf {
		putCell(grid, fr-1, fc, rcell{ch: '▾', fg: spriteWhite, bold: true})
		cell.fg = spriteWhite
		cell.bg = body.BlendLab(spriteBlack, 0.15).Clamped()
	}
	putCell(grid, fr, fc, cell)
}

// accessoryColors returns the main/shade colors for an accessory: its own Hex
// (shade derived by darkening), or the default gold when unspecified. Worn
// avatars, ground loot and inventory icons all draw through this so a hat looks
// the same everywhere.
func accessoryColors(accessory int) (main, shade colorful.Color) {
	a := accessories[wrapIdx(accessory, len(accessories))]
	if a.Hex == "" {
		return hatMain, hatShade
	}
	main = mustHex(a.Hex)
	return main, main.BlendLab(spriteBlack, 0.42).Clamped()
}

// AccessoryColor is an accessory's display hex (the default gold when it has
// none), so other packages (the Wilds' hat loot) color it consistently.
func AccessoryColor(accessory int) string {
	if a := accessories[wrapIdx(accessory, len(accessories))]; a.Hex != "" {
		return a.Hex
	}
	return "#FFD166"
}

// accessoryLights gives the wearables that shine — bioluminescent caps and
// crystal trophies — the glow they cast around the player after dark: a portable
// light is the first of the powers a worn thing can grant. Keyed by name so a new
// luminous wearable is one row; absent means it stays a plain cosmetic.
var accessoryLights = map[string]struct {
	hex    string
	radius float64 // in tiles; keep ≤ overhangTiles so the incremental render stays exact
}{
	"glowcap": {"#9BF7AE", 3.4}, // the lantern-plant: the brightest, a warm green
	"shroom":  {"#D7A0FF", 3.0}, // a glowing mushroom cap
	"circlet": {"#A8E8FF", 2.7}, // a geode's crystal: cooler and fainter
}

// AccessoryLight returns the night glow a worn accessory casts — its color and
// radius in tiles — with ok=false for a non-luminous one. The HD renderer blooms
// it around the wearer after dusk, so a glowing wearable lights your way.
func AccessoryLight(accessory int) (colorful.Color, float64, bool) {
	if accessory <= 0 || accessory >= len(accessories) {
		return colorful.Color{}, 0, false
	}
	l, ok := accessoryLights[accessories[accessory].Name]
	if !ok {
		return colorful.Color{}, 0, false
	}
	return mustHex(l.hex), l.radius, true
}

// spritePixel resolves a bitmap code to a color (and whether it's opaque),
// shading relative to the player's body color. accMain/accShade color the
// accessory pixels (H/h) so each hat keeps its own hue.
func spritePixel(code rune, body, accMain, accShade colorful.Color, isSelf bool) (colorful.Color, bool) {
	b := body
	if isSelf {
		b = body.BlendLab(spriteWhite, 0.15).Clamped()
	}
	switch code {
	case 'B':
		return b, true
	case 'L':
		return b.BlendLab(spriteWhite, 0.40).Clamped(), true
	case 'D':
		return b.BlendLab(spriteBlack, 0.40).Clamped(), true
	case 'E':
		return spriteBlack, true
	case 'm':
		return b.BlendLab(spriteBlack, 0.55).Clamped(), true
	case 'W':
		return spriteWhite, true
	case 'H':
		return accMain, true
	case 'h':
		return accShade, true
	default:
		return colorful.Color{}, false
	}
}

// AvatarPreview renders a front-facing avatar (style + accessory + color) as
// half-block lines for the character panel: full 12-px width, two pixels per
// cell tall. Drawn through the theme's renderer so it downsamples like the live
// avatar. Transparent pixels show the panel background.
func AvatarPreview(th *ui.Theme, style, accessory int, color lipgloss.Color) []string {
	if th == nil {
		th = ui.Default
	}
	body := playerColor(color)
	accMain, accShade := accessoryColors(accessory)
	bmp := AvatarBitmap(style, accessory, world.DirS, 0)
	bg := lipgloss.Color(ui.HexPanelBg)
	toLip := func(c colorful.Color) lipgloss.Color { return lipgloss.Color(c.Clamped().Hex()) }

	lines := make([]string, 0, len(bmp)/2)
	for row := 0; row+1 < len(bmp); row += 2 {
		top := []rune(bmp[row])
		bot := []rune(bmp[row+1])
		var sb strings.Builder
		for col := 0; col < len(top) && col < len(bot); col++ {
			tc, topOp := spritePixel(top[col], body, accMain, accShade, false)
			bc, botOp := spritePixel(bot[col], body, accMain, accShade, false)
			switch {
			case topOp && botOp:
				sb.WriteString(th.FgBg(toLip(tc), toLip(bc)).Render("▀"))
			case topOp:
				sb.WriteString(th.FgBg(toLip(tc), bg).Render("▀"))
			case botOp:
				sb.WriteString(th.FgBg(toLip(bc), bg).Render("▄"))
			default:
				sb.WriteString(th.FgBg(bg, bg).Render(" "))
			}
		}
		lines = append(lines, sb.String())
	}
	return lines
}

func nameInitial(name string) rune {
	for _, c := range name {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			return unicode.ToUpper(c)
		}
	}
	return '☺'
}

func putCell(grid [][]rcell, r, c int, cell rcell) {
	if r < 0 || r >= len(grid) || c < 0 || len(grid) == 0 || c >= len(grid[0]) {
		return
	}
	grid[r][c] = cell
}
