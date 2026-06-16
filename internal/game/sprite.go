package game

import (
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Sprite bitmaps live in avatar.go (AvatarBitmap), shared by both renderers.
// They are 6×6 pixels → 6 cells wide × 3 cells tall in the half-block renderer,
// sized to sit close to the 2×2 footprint rather than loom over it.

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

// stampSprite draws one player's avatar onto the grid. (fc,fr) is the
// top-left grid cell of the player's PlayerW×PlayerH footprint; the sprite is
// centered over it and bottom-aligned, overhanging upward and sideways.
func stampSprite(grid [][]rcell, th *ui.Theme, p world.Player, isSelf bool, frame, fc, fr int) {
	body := playerColor(p.Color)
	// The half-block grid can't show the full-res sprite; downsample it to keep
	// the glyph avatar ~2 tiles (6 px wide × 6 px tall → 6 cells × 3 cells).
	bmp := downsampleBitmap(AvatarBitmap(p.Style, p.Accessory, p.Facing, AvatarWalkFrame(p.LastMoved, frame)), 6, 6)
	cellsW := len(bmp[0])
	cellsH := len(bmp) / 2

	left := fc + PlayerW/2 - cellsW/2
	bottom := fr + PlayerH - 1
	top := bottom - (cellsH - 1)

	// "you" marker: a small chevron floating above your own head.
	if isSelf {
		putCell(grid, top-1, fc+PlayerW/2, rcell{ch: '▾', fg: spriteWhite, bold: true})
	}

	for cr := 0; cr < cellsH; cr++ {
		topRow := []rune(bmp[2*cr])
		botRow := []rune(bmp[2*cr+1])
		for cc := 0; cc < cellsW; cc++ {
			tc, topOp := spritePixel(topRow[cc], body, isSelf)
			bc, botOp := spritePixel(botRow[cc], body, isSelf)
			if !topOp && !botOp {
				continue
			}
			gr, gc := top+cr, left+cc
			switch {
			case topOp && botOp:
				putCell(grid, gr, gc, rcell{ch: '▀', fg: tc, bg: bc, hasBg: true, bold: true})
			case topOp:
				putCell(grid, gr, gc, rcell{ch: '▀', fg: tc, bold: true})
			default:
				putCell(grid, gr, gc, rcell{ch: '▄', fg: bc, bold: true})
			}
		}
	}

	// name initial below the feet, in the player's color (reversed for self).
	init := nameInitial(p.Name)
	cell := rcell{ch: init, fg: body, bold: true}
	if isSelf {
		cell.fg = spriteWhite
		cell.bg = body
		cell.hasBg = true
	}
	putCell(grid, bottom+1, fc+PlayerW/2, cell)
}

// spritePixel resolves a bitmap code to a color (and whether it's opaque),
// shading relative to the player's body color.
func spritePixel(code rune, body colorful.Color, isSelf bool) (colorful.Color, bool) {
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
		return hatMain, true
	case 'h':
		return hatShade, true
	default:
		return colorful.Color{}, false
	}
}

// downsampleBitmap nearest-samples a sprite down to at most maxW×maxH (keeping
// an even height for half-block pairing). Sprites already within bounds are
// returned unchanged.
func downsampleBitmap(rows []string, maxW, maxH int) []string {
	h := len(rows)
	w := len([]rune(rows[0]))
	if w <= maxW && h <= maxH {
		return rows
	}
	nw, nh := min(w, maxW), min(h, maxH)
	if nh%2 == 1 {
		nh--
	}
	out := make([]string, nh)
	for y := 0; y < nh; y++ {
		src := []rune(rows[y*h/nh])
		o := make([]rune, nw)
		for x := 0; x < nw; x++ {
			o[x] = src[x*w/nw]
		}
		out[y] = string(o)
	}
	return out
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
