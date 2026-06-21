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
	pixel.Frame(img, 0, by, W, bh)

	y := by + 2*s
	pixel.DrawText(img, 4*s, y, s, areaName, hudAccent)
	if hint != "" {
		pixel.DrawText(img, 4*s+pixel.TextWidth(areaName+"  ", s), y, s, asciiOnly(hint), hudWhite)
	}
	pixel.DrawText(img, 4*s, y+lh, s,
		"move  e use  enter chat  c char  i bag  q quit", hudDim)
}

// panelBox centers a pw×ph panel in the play area above the HUD bar, clamping it
// on-screen. Reserving the bar's height keeps a tall panel's footer from
// colliding with the status bar on small frames.
func panelBox(W, H, pw, ph int) (ox, oy, cw int) {
	if pw > W-4 {
		pw = W - 4
	}
	barH := 36 * hudScale(W) // matches DrawHUD's bottom bar (2*lh + 4*s)
	ox, oy = (W-pw)/2, (H-barH-ph)/2
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
	pixel.Frame(img, x-3*s, y-2*s, tw+6*s, 16*s+2*s)
	pixel.DrawText(img, x, y, s, text, hudToast)
}

// DrawPortalLabels floats each visible portal's destination name above it, so a
// player can read where a gate leads (and where a broken/sealed gate would lead)
// before stepping in. tm is the window tilemap — tile (vx,vy) maps to screen
// pixel (vx*scale, vy*scale) — and scale is the pixels-per-tile the scene was
// rasterized at. A sealed gate's label is dimmed to match its dormant look.
func DrawPortalLabels(img *image.RGBA, tm *TileMap, scale int) {
	W := img.Bounds().Dx()
	s := hudScale(W)
	apx := scale / 6 // tileArtN: portal art is 12 art-pixels (~2 tiles) tall
	if apx < 1 {
		apx = 1
	}
	lh := 13*s + 2*s
	for vy := 0; vy < tm.H && vy < len(tm.Tiles); vy++ {
		for vx := 0; vx < tm.W && vx < len(tm.Tiles[vy]); vx++ {
			t := tm.Tiles[vy][vx]
			sealed := t.Prop == PropSealed
			if t.Prop != PropPortal && !sealed && t.Kind != TilePortal {
				continue
			}
			name := t.Label
			if name == "" && t.Portal != "" {
				name = DisplayName(t.Portal)
			}
			name = asciiOnly(name)
			if name == "" {
				continue
			}
			tw := pixel.TextWidth(name, s)
			lx := vx*scale + scale/2 - tw/2
			// The portal art is bottom-aligned on the tile and overhangs ~2 tiles
			// upward; sit the label just above that so it never covers the gate.
			portalTop := (vy+1)*scale - 12*apx
			ty := portalTop - lh - s
			if ty < 2 { // too near the top edge — drop the label just below the gate
				ty = (vy+1)*scale + s
			}
			pixel.Shade(img, lx-2*s, ty-s, tw+4*s, lh+s, 0.6)
			col := hudToast
			if sealed {
				col = hudDim
			}
			pixel.DrawText(img, lx, ty, s, name, col)
		}
	}
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

// DrawInventoryPanel draws the player's pack: their hero (a large avatar that
// grows with the screen) with name and worn hat on the left, and a gem-iconed
// item list with owned hats on the right.
func DrawInventoryPanel(img *image.RGBA, ctx *Ctx) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 8 * s
	gap := pad // gutter between the two columns

	cur, hasSelf := ctx.World.Self(ctx.Name)

	// Right column: collected items (with their gem color) and owned hats.
	type itemRow struct {
		name string
		n    int
		base color.RGBA
		hi   color.RGBA
	}
	var items []itemRow
	total := 0
	for _, it := range Items {
		if n := ctx.Inventory[it.ID]; n > 0 {
			c := mustHex(it.Hex)
			items = append(items, itemRow{it.Name, n, colorfulToRGBA(c),
				colorfulToRGBA(c.BlendLab(spriteWhite, 0.55).Clamped())})
			total += n
		}
	}
	var hatIdxs []int
	for _, idx := range OwnedHats(ctx) {
		if idx != 0 {
			hatIdxs = append(hatIdxs, idx)
		}
	}
	const maxHatRows = 6
	hatsShown := len(hatIdxs)
	if hatsShown > maxHatRows {
		hatsShown = maxHatRows
	}
	equipped := -1
	if hasSelf {
		equipped = cur.Accessory
	}

	gem := lh - 4*s // gem-icon box, fitted to a text line
	emptyMsg := "Empty - walk over loot to gather it."

	// Left column: the hero, scaled up with the frame ("bigger you on a bigger
	// screen"), then clamped so it never eats more than a third of the width.
	aScale := H / 90
	if aScale > 16 {
		aScale = 16
	}
	if aScale < s*3 {
		aScale = s * 3
	}
	var avW, avH, bw, bh int
	var bmp []string
	if hasSelf {
		bmp = AvatarBitmap(cur.Style, cur.Accessory, world.DirS, 0)
		bw, bh = len([]rune(bmp[0])), len(bmp)
		for aScale > s*2 && bw*aScale > W/3 {
			aScale--
		}
		avW, avH = bw*aScale, bh*aScale
	}
	name := asciiOnly(ctx.Name)
	hatLine := ""
	if hasSelf {
		hatLine = "Hat: " + AccessoryName(cur.Accessory)
	}
	leftW := avW
	for _, t := range []string{name, hatLine} {
		if w := pixel.TextWidth(t, s); w > leftW {
			leftW = w
		}
	}
	leftH := avH + lh/2 + lh + lh/4 + lh // avatar, name, hat

	// Right-column width: title, item rows (gem + name + count), hat rows.
	title := fmt.Sprintf("INVENTORY  -  %d item", total)
	if total != 1 {
		title += "s"
	}
	rightW := pixel.TextWidth("ACCESSORIES", s)
	for _, r := range items {
		w := gem + s*3 + pixel.TextWidth(r.name, s) + s*4 + pixel.TextWidth(fmt.Sprintf("x%d", r.n), s)
		if w > rightW {
			rightW = w
		}
	}
	if len(items) == 0 {
		if w := pixel.TextWidth(emptyMsg, s); w > rightW {
			rightW = w
		}
	}
	for i := 0; i < hatsShown; i++ {
		label := AccessoryName(hatIdxs[i])
		if hatIdxs[i] == equipped {
			label += "  worn"
		}
		if w := gem + s*3 + pixel.TextWidth(label, s); w > rightW {
			rightW = w
		}
	}

	itemLines := len(items)
	if itemLines == 0 {
		itemLines = 1
	}
	rightH := itemLines * lh
	if len(hatIdxs) > 0 {
		rightH += lh/2 + lh + hatsShown*lh
		if len(hatIdxs) > hatsShown {
			rightH += lh
		}
	}

	bodyH := leftH
	if rightH > bodyH {
		bodyH = rightH
	}
	contentW := leftW + gap + rightW
	if !hasSelf {
		contentW = rightW
	}
	ph := pad*2 + lh + lh/2 + bodyH + lh/2 + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	// Header.
	top := oy + pad
	pixel.DrawText(img, ox+pad, top, s, title, hudAccent)
	bodyY := top + lh + lh/2

	rightX := ox + pad
	if hasSelf {
		// Left column: a shaded pedestal, the avatar, name and worn hat.
		leftX := ox + pad
		ax := leftX + (leftW-avW)/2
		pixel.Shade(img, ax-s, bodyY+avH-2*s, avW+2*s, 3*s, 0.5)
		drawAvatarInto(img, ax, bodyY, aScale, cur.Style, cur.Accessory, cur.Color)
		ny := bodyY + avH + lh/2
		pixel.DrawText(img, leftX+(leftW-pixel.TextWidth(name, s))/2, ny, s, name, hudBright)
		pixel.DrawText(img, leftX+(leftW-pixel.TextWidth(hatLine, s))/2, ny+lh+lh/4, s, hatLine, hudDim)
		rightX = leftX + leftW + gap
	}

	// Right column: items, then owned hats.
	y := bodyY
	if len(items) == 0 {
		pixel.DrawText(img, rightX, y, s, emptyMsg, hudDim)
		y += lh
	}
	for _, r := range items {
		drawGem(img, rightX, y+(lh-gem)/2, gem, r.base, r.hi)
		pixel.DrawText(img, rightX+gem+s*3, y, s, r.name, hudWhite)
		cnt := fmt.Sprintf("x%d", r.n)
		pixel.DrawText(img, rightX+rightW-pixel.TextWidth(cnt, s), y, s, cnt, hudAccent)
		y += lh
	}
	if len(hatIdxs) > 0 {
		y += lh / 2
		pixel.DrawText(img, rightX, y, s, "ACCESSORIES", hudDim)
		y += lh
		for i := 0; i < hatsShown; i++ {
			idx := hatIdxs[i]
			worn := idx == equipped
			if worn {
				// Highlight strip behind the equipped accessory.
				pixel.Shade(img, rightX-s, y-s, rightW+2*s, lh, 0.35)
			}
			drawAccessoryIcon(img, rightX, y+(lh-gem)/2, gem, idx)
			nm := AccessoryName(idx)
			nameCol := hudWhite
			if worn {
				nameCol = hudBright
			}
			pixel.DrawText(img, rightX+gem+s*3, y, s, nm, nameCol)
			if worn {
				pixel.DrawText(img, rightX+gem+s*3+pixel.TextWidth(nm+"  ", s), y, s, "worn", hudAccent)
			}
			y += lh
		}
		if extra := len(hatIdxs) - hatsShown; extra > 0 {
			pixel.DrawText(img, rightX+gem+s*3, y, s, fmt.Sprintf("+%d more", extra), hudDim)
			y += lh
		}
	}

	footer := "i or q: close"
	pixel.DrawText(img, ox+pad, oy+ph-pad-lh+lh/4, s, footer, hudDim)
}

// drawGem paints a small diamond loot icon (matching the in-world gem), with a
// lit upper-left facet so it reads as faceted rather than flat.
func drawGem(img *image.RGBA, x, y, box int, base, hi color.RGBA) {
	r := box / 2
	cx, cy := x+r, y+r
	bnd := img.Bounds()
	for dy := -r; dy <= r; dy++ {
		span := r - abs(dy)
		for dx := -span; dx <= span; dx++ {
			c := base
			if dx+dy <= 0 {
				c = hi // upper-left facet catches the light
			}
			px, py := cx+dx, cy+dy
			if (image.Point{X: px, Y: py}).In(bnd) {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// drawAccessoryIcon renders an accessory's own silhouette (its overlay shape) as
// a small icon in its color, cropped to its pixels and centered in a box-sized
// cell — so a crown reads as a crown and a flower as a flower, not a generic dot.
func drawAccessoryIcon(img *image.RGBA, x, y, box, accessory int) {
	ov := accessories[wrapIdx(accessory, len(accessories))].overlay
	minX, minY, maxX, maxY := 1<<30, 1<<30, -1, -1
	for r, row := range ov {
		for c, ch := range row {
			if ch != ' ' {
				minX, maxX = min(minX, c), max(maxX, c)
				minY, maxY = min(minY, r), max(maxY, r)
			}
		}
	}
	if maxX < 0 {
		return
	}
	bw, bh := maxX-minX+1, maxY-minY+1
	sc := min(box/bw, box/bh)
	if sc < 1 {
		sc = 1
	}
	main, shade := accessoryColors(accessory)
	mc, sh := colorfulToRGBA(main), colorfulToRGBA(shade)
	offX := x + (box-bw*sc)/2
	offY := y + (box-bh*sc)/2
	for r := minY; r <= maxY; r++ {
		row := []rune(ov[r])
		for c := minX; c <= maxX && c < len(row); c++ {
			var col color.RGBA
			switch row[c] {
			case 'H':
				col = mc
			case 'h':
				col = sh
			default:
				continue
			}
			fillRect(img, offX+(c-minX)*sc, offY+(r-minY)*sc, sc, sc, col)
		}
	}
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
	accMain, accShade := accessoryColors(accessory)
	bmp := AvatarBitmap(style, accessory, world.DirS, 0)
	for r := 0; r < len(bmp); r++ {
		row := []rune(bmp[r])
		for c := 0; c < len(row); c++ {
			cc, op := spritePixel(row[c], body, accMain, accShade, false)
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
