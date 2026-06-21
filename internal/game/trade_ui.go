package game

import (
	"fmt"
	"image"
	"image/color"

	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/pixel"
)

// Trade UI (HD). A trade is a face-to-face swap between two nearby players: each
// lays items into their side of the table, both must mark Ready, and the swap
// happens atomically. This file is the on-frame panel; the negotiation/settlement
// logic lives in the world/store. Both sides render the same view so the table
// reads identically to each trader (with "you" always on the left).

// TradeRow is one item stack in an offer or in the pack picker.
type TradeRow struct {
	Item Item
	N    int
}

// TradeParty is one side of the table.
type TradeParty struct {
	Name      string
	Style     int
	Accessory int
	Color     lipgloss.Color
	Offer     []TradeRow
	Ready     bool
}

// TradeView is everything the trade panel draws: both parties, your pack to
// offer from, and which pack slot is selected.
type TradeView struct {
	You, Them TradeParty
	Pack      []TradeRow
	Sel       int
}

var (
	hudOK   = color.RGBA{0x6F, 0xD8, 0x8F, 0xFF} // ready / confirmed
	hudWait = color.RGBA{0xD7, 0xB0, 0x6A, 0xFF} // still deciding
	hudCard = color.RGBA{0x22, 0x28, 0x33, 0xFF} // inset slot background
)

// DrawTradePanel renders the trade table onto an HD frame.
func DrawTradePanel(img *image.RGBA, v TradeView) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 10 * s
	icon := lh + lh/2 // offer & pack icon box
	rowH := icon + 4*s
	colGap := 18 * s
	avS := s * 3

	pw := W - 8*s
	if pw > 540*s {
		pw = 540 * s
	}
	ph := H - 8*s
	if ph > 410*s {
		ph = 410 * s
	}
	ox, oy, pw := panelBox(W, H, pw, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	// Title.
	top := oy + pad
	pixel.DrawText(img, ox+pad, top, s, "TRADE", hudAccent)

	midX := ox + pw/2
	leftX := ox + pad
	rightX := midX + colGap/2
	colW := pw/2 - pad - colGap/2

	headerY := top + lh + lh/2

	// Vertical divider between the two sides, with a swap mark in the middle.
	colsBottom := headerY + avS*14 + lh + maxTradeRows*rowH + lh
	fillRect(img, midX-s/2, headerY, max(s/2, 1), colsBottom-headerY, hudDim)
	swap := "<>"
	sw := pixel.TextWidth(swap, s)
	smy := (headerY+colsBottom)/2 - lh/2
	fillRect(img, midX-sw/2-4*s, smy-3*s, sw+8*s, lh+4*s, hudCard)
	pixel.Frame(img, midX-sw/2-4*s, smy-3*s, sw+8*s, lh+4*s)
	pixel.DrawText(img, midX-sw/2, smy, s, swap, hudAccent)

	drawTradeSide(img, leftX, colW, headerY, s, v.You, true)
	drawTradeSide(img, rightX, colW, headerY, s, v.Them, false)

	// Pack picker along the bottom: your inventory, one slot selected, with the
	// hint that +/- move a stack on and off the table.
	footY := oy + ph - pad - lh
	packLabelY := colsBottom + lh/2
	pixel.DrawText(img, leftX, packLabelY, s, "YOUR PACK", hudDim)
	pixel.DrawText(img, leftX+pixel.TextWidth("YOUR PACK  ", s), packLabelY, s,
		"+ offer   - withdraw", rgbaLerp(hudDim, hudBright, 0.25))

	slot := icon + 6*s
	gap := 6 * s
	px, py := leftX, packLabelY+lh+lh/4
	for i, r := range v.Pack {
		x := px + i*(slot+gap)
		if x+slot > ox+pw-pad {
			break
		}
		fillRect(img, x, py, slot, slot, hudCard)
		if i == v.Sel {
			pixel.Shade(img, x, py, slot, slot, 0.0) // keep bg
			pixel.Frame(img, x-s, py-s, slot+2*s, slot+2*s)
		}
		drawItemIcon(img, x+(slot-icon)/2, py+(slot-icon)/2, icon, r.Item)
		cnt := fmt.Sprintf("x%d", r.N)
		col := hudWhite
		if i == v.Sel {
			col = hudBright
		}
		pixel.DrawText(img, x+(slot-pixel.TextWidth(cnt, s))/2, py+slot+s, s, cnt, col)
	}

	// Footer controls.
	footer := "left/right select   +/- offer   Enter ready   Esc cancel"
	pixel.DrawText(img, ox+pad, footY, s, footer, hudDim)
}

const maxTradeRows = 4

// drawTradeSide renders one trader's column: avatar + name, a Ready/▢ chip, an
// OFFER label and their staged item rows (icon, name, count).
func drawTradeSide(img *image.RGBA, x, colW, y, s int, p TradeParty, isYou bool) {
	lh := 16 * s
	icon := lh + lh/2
	rowH := icon + 4*s
	avS := s * 3

	drawAvatarInto(img, x, y, avS, p.Style, p.Accessory, p.Color)
	avW := 12 * avS // avatar bitmap is ~12 wide
	name := asciiOnly(p.Name)
	if isYou {
		name += " (you)"
	}
	pixel.DrawText(img, x+avW+3*s, y+avS*2, s, name, hudBright)

	// Ready chip, right-aligned in the column.
	chip, chipCol := "deciding", hudWait
	if p.Ready {
		chip, chipCol = "READY", hudOK
	}
	cw := pixel.TextWidth(chip, s) + 8*s
	cx := x + colW - cw
	cy := y + avS*2 - 2*s
	fillRect(img, cx, cy, cw, lh+s, hudCard)
	pixel.DrawText(img, cx+4*s, y+avS*2, s, chip, chipCol)

	// Offer rows.
	oy := y + avS*14
	pixel.DrawText(img, x, oy, s, "OFFERS", hudDim)
	ry := oy + lh + lh/4
	if len(p.Offer) == 0 {
		pixel.DrawText(img, x+4*s, ry+icon/2-lh/2, s, "(empty)", rgbaLerp(hudCard, hudDim, 1))
	}
	for i, r := range p.Offer {
		if i >= maxTradeRows {
			break
		}
		fillRect(img, x, ry-2*s, colW, rowH-2*s, hudCard)
		drawItemIcon(img, x+3*s, ry+(rowH-2*s-icon)/2-2*s, icon, r.Item)
		tx := x + icon + 8*s
		pixel.DrawText(img, tx, ry+lh/4, s, asciiOnly(r.Item.Name), hudWhite)
		cnt := fmt.Sprintf("x%d", r.N)
		pixel.DrawText(img, x+colW-pixel.TextWidth(cnt, s)-6*s, ry+lh/4, s, cnt, hudAccent)
		ry += rowH
	}
}
