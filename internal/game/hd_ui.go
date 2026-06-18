package game

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/pixel"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// HD UI: the on-frame interface for the default (pixel) client. HD has no glyph
// layer, so the status bar, toasts and the character/inventory panels are drawn
// straight onto the RGBA frame with basicfont (ASCII only). These mirror the
// glyph client's overlays so both renderers offer the same UI.

var (
	hudWhite  = color.RGBA{0xF2, 0xF5, 0xFA, 0xFF}
	hudDim    = color.RGBA{0x9A, 0xA3, 0xAD, 0xFF}
	hudAccent = color.RGBA{0x2E, 0x8B, 0xFF, 0xFF}
	hudBright = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	hudToast  = color.RGBA{0x7D, 0xF0, 0xFF, 0xFF}
)

// hudScale picks a legible text scale for the frame width.
func hudScale(w int) int {
	s := w / 540
	if s < 2 {
		s = 2
	}
	if s > 3 {
		s = 3
	}
	return s
}

// DrawHUD draws the bottom status bar onto an HD frame: the area name, a
// context hint (e.g. "e - wear the crown") and the control legend.
func DrawHUD(img *image.RGBA, areaName, hint string) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	bh := 2*lh + 4*s
	by := H - bh
	pixel.Shade(img, 0, by, W, bh, 0.8)

	y := by + 2*s
	pixel.DrawText(img, 4*s, y, s, areaName, hudAccent)
	if hint != "" {
		pixel.DrawText(img, 4*s+pixel.TextWidth(areaName+"  ", s), y, s, asciiOnly(hint), hudWhite)
	}
	pixel.DrawText(img, 4*s, y+lh, s,
		"move  e pick  enter chat  c char  i bag  q quit", hudDim)
}

// panelBox centers a pw×ph panel, clamping it on-screen.
func panelBox(W, H, pw, ph int) (ox, oy, cw int) {
	if pw > W-4 {
		pw = W - 4
	}
	ox, oy = (W-pw)/2, (H-ph)/2
	if ox < 2 {
		ox = 2
	}
	if oy < 2 {
		oy = 2
	}
	return ox, oy, pw
}

// DrawToast draws a transient message near the top of an HD frame.
func DrawToast(img *image.RGBA, text string) {
	W := img.Bounds().Dx()
	s := hudScale(W)
	text = asciiOnly(text)
	tw := pixel.TextWidth(text, s)
	x, y := (W-tw)/2, 6*s
	pixel.Shade(img, x-3*s, y-2*s, tw+6*s, 16*s+2*s, 0.72)
	pixel.DrawText(img, x, y, s, text, hudToast)
}

// DrawCharPanel draws the interactive character editor onto an HD frame: a live
// avatar preview over the cycleable Style / Color / Hat fields, the selected
// one marked. field is the highlighted row (0..CharFields-1).
func DrawCharPanel(img *image.RGBA, ctx *Ctx, field int) {
	cur, ok := ctx.World.Self(ctx.Name)
	if !ok {
		return
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	aScale := s * 3
	bmp := AvatarBitmap(cur.Style, cur.Accessory, world.DirS, 0)
	avW, avH := len([]rune(bmp[0]))*aScale, len(bmp)*aScale

	hatVal := AccessoryName(cur.Accessory)
	if len(OwnedHats(ctx)) == 1 {
		hatVal += "  (find hats!)"
	}
	fields := []struct{ label, val string }{
		{"Style", AvatarStyleName(cur.Style)},
		{"Color", fmt.Sprintf("#%d", ui.AvatarColorIndex(cur.Color))},
		{"Hat", hatVal},
	}
	rows := make([]string, len(fields))
	for i, f := range fields {
		if i == field {
			rows[i] = "> " + f.label + ":  < " + f.val + " >"
		} else {
			rows[i] = "  " + f.label + ":  " + f.val
		}
	}
	footer := "arrows: edit   q: close"

	contentW := avW
	for _, r := range append(append([]string{}, rows...), "CHARACTER", footer) {
		if w := pixel.TextWidth(r, s); w > contentW {
			contentW = w
		}
	}
	pad := 7 * s
	ph := pad*2 + lh + lh/2 + avH + lh + len(rows)*lh + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	y := oy + pad
	pixel.DrawText(img, ox+pad, y, s, "CHARACTER", hudAccent)
	y += lh + lh/2
	drawAvatarInto(img, ox+(pw-avW)/2, y, aScale, cur.Style, cur.Accessory, cur.Color)
	y += avH + lh
	for i, r := range rows {
		col := hudWhite
		if i == field {
			col = hudBright
		}
		pixel.DrawText(img, ox+pad, y, s, r, col)
		y += lh
	}
	y += lh / 2
	pixel.DrawText(img, ox+pad, y, s, footer, hudDim)
}

// DrawInventoryPanel draws the player's collected items and unlocked hats.
func DrawInventoryPanel(img *image.RGBA, ctx *Ctx) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s

	var rows []string
	total := 0
	for _, it := range Items {
		if n := ctx.Inventory[it.ID]; n > 0 {
			rows = append(rows, fmt.Sprintf("%-14s x%d", it.Name, n))
			total += n
		}
	}
	var hats []string
	for _, idx := range OwnedHats(ctx) {
		if idx != 0 {
			hats = append(hats, AccessoryName(idx))
		}
	}
	if len(hats) > 0 {
		rows = append(rows, "", "Hats: "+strings.Join(hats, ", "))
	}
	if len(rows) == 0 {
		rows = []string{"empty - explore and press e to forage"}
	}
	title := fmt.Sprintf("INVENTORY  (%d items)", total)
	footer := "i or q: close"

	contentW := 0
	for _, r := range append(append([]string{}, rows...), title, footer) {
		if w := pixel.TextWidth(r, s); w > contentW {
			contentW = w
		}
	}
	pad := 7 * s
	ph := pad*2 + lh + lh/2 + len(rows)*lh + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	y := oy + pad
	pixel.DrawText(img, ox+pad, y, s, title, hudAccent)
	y += lh + lh/2
	for _, r := range rows {
		pixel.DrawText(img, ox+pad, y, s, r, hudWhite)
		y += lh
	}
	y += lh / 2
	pixel.DrawText(img, ox+pad, y, s, footer, hudDim)
}

// HDLine is one rendered chat line for the HD log: text plus its color.
type HDLine struct {
	Text string
	Col  color.RGBA
}

// hdChatLines is how many recent chat lines the HD log shows.
const hdChatLines = 6

// HDChatLine formats a world event as an HD chat line; ok is false for events
// that don't belong in chat (ticks, moves, the player's own join).
func HDChatLine(ev world.Event, self string) (HDLine, bool) {
	switch ev.Type {
	case world.EventChat:
		return HDLine{ev.Player + ": " + ev.Detail, lipToRGBA(ui.AvatarColor(ev.Player))}, true
	case world.EventEmote:
		return HDLine{"* " + ev.Player + " " + ev.Detail, hudAccent}, true
	case world.EventWhisper:
		return HDLine{ev.Player + " whispers: " + ev.Detail, hudToast}, true
	case world.EventJoined:
		if ev.Player != self {
			return HDLine{"- " + ev.Player + " arrived", hudDim}, true
		}
	case world.EventLeft:
		if ev.Player != self {
			if ev.Detail != "" {
				return HDLine{"- " + ev.Player + " -> " + ev.Detail, hudDim}, true
			}
			return HDLine{"- " + ev.Player + " left", hudDim}, true
		}
	}
	return HDLine{}, false
}

// DrawChat overlays the recent chat lines (and, when active, the input line)
// just above the HUD bar.
func DrawChat(img *image.RGBA, lines []HDLine, active bool, input string) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	barH := 2*lh + 4*s // keep in step with DrawHUD
	if len(lines) > hdChatLines {
		lines = lines[len(lines)-hdChatLines:]
	}

	bottom := H - barH
	if active {
		iy := bottom - lh - 2*s
		pixel.Shade(img, 0, iy, W, lh+2*s, 0.85)
		pixel.DrawText(img, 4*s, iy+s, s, "> "+asciiOnly(input)+"_", hudWhite)
		bottom = iy
	}
	if len(lines) == 0 {
		return
	}
	blockH := len(lines)*lh + 2*s
	pixel.Shade(img, 0, bottom-blockH, W*3/4, blockH, 0.5)
	y := bottom - lh
	for i := len(lines) - 1; i >= 0; i-- {
		pixel.DrawText(img, 4*s, y, s, asciiOnly(lines[i].Text), lines[i].Col)
		y -= lh
	}
}

func lipToRGBA(c lipgloss.Color) color.RGBA { return colorfulToRGBA(playerColor(c)) }

// drawAvatarInto rasterizes a front-facing avatar (style + accessory + color)
// into the frame at (x,y), each sprite pixel scaled by scale.
func drawAvatarInto(img *image.RGBA, x, y, scale, style, accessory int, col lipgloss.Color) {
	body := playerColor(col)
	bmp := AvatarBitmap(style, accessory, world.DirS, 0)
	for r := 0; r < len(bmp); r++ {
		row := []rune(bmp[r])
		for c := 0; c < len(row); c++ {
			cc, op := spritePixel(row[c], body, false)
			if !op {
				continue
			}
			rc := colorfulToRGBA(cc)
			for ky := 0; ky < scale; ky++ {
				for kx := 0; kx < scale; kx++ {
					px, py := x+c*scale+kx, y+r*scale+ky
					if (image.Point{px, py}).In(img.Bounds()) {
						img.SetRGBA(px, py, rc)
					}
				}
			}
		}
	}
}

// asciiOnly strips non-ASCII runes so basicfont (ASCII-only) renders cleanly;
// glyph-renderer strings sometimes carry box/arrow runes the HD font lacks.
func asciiOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7F {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
