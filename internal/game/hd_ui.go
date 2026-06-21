package game

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"unicode"

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

// keyHint is one "KEY label" pair shown in the on-frame legends.
type keyHint struct{ key, label string }

// hudLegend is the persistent mini-legend pinned to the top-right corner: the
// everyday keys in a tidy 2×2 grid. The full reference lives behind "?" and the
// Tab menu, so this stays short — the two doors (menu, help) plus the two things
// you do most (move, chat).
var hudLegend = [2][2]keyHint{
	{{"WASD", "move"}, {"Enter", "chat"}},
	{{"Tab", "menu"}, {"?", "help"}},
}

// drawKeyHint draws one "KEY label" pair at (x,y) — key bright, label dim — and
// returns its rendered width.
func drawKeyHint(img *image.RGBA, x, y, s int, h keyHint) int {
	pixel.DrawText(img, x, y, s, h.key, hudBright)
	kw := pixel.TextWidth(h.key+" ", s)
	pixel.DrawText(img, x+kw, y, s, h.label, hudDim)
	return kw + pixel.TextWidth(h.label, s)
}

// keyHintWidth is drawKeyHint's width without drawing.
func keyHintWidth(h keyHint, s int) int { return pixel.TextWidth(h.key+" "+h.label, s) }

// truncToWidth trims s so it renders within maxW pixels at the given scale,
// ending in ".." when shortened (the bitmap font is fixed-width ASCII).
func truncToWidth(s string, scale, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if pixel.TextWidth(s, scale) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		if cand := string(r) + ".."; pixel.TextWidth(cand, scale) <= maxW {
			return cand
		}
	}
	return ""
}

// rgbaLerp blends a→b by t in [0,1].
func rgbaLerp(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 0xFF}
}

// DrawAreaTitle draws the area name in the top-left corner. emphasis (1 just
// after entering, easing to 0) flares the entrance: the name brightens over a
// soft backing, then settles to a quiet dim label that just answers "where am
// I?" without taking a whole bar.
func DrawAreaTitle(img *image.RGBA, name string, emphasis float64) {
	W := img.Bounds().Dx()
	s := hudScale(W)
	lh := 16 * s
	pad := 5 * s
	x, y := pad, pad
	name = asciiOnly(name)
	if emphasis > 0 {
		tw := pixel.TextWidth(name, s)
		pixel.Shade(img, x-2*s, y-2*s, tw+4*s, lh, 0.55*emphasis)
	}
	pixel.DrawText(img, x, y, s, name, rgbaLerp(hudDim, hudBright, emphasis))
}

// DrawTopLegend draws the persistent mini-legend as a 2×2 grid pinned to the
// top-right corner — always visible, never in the way.
func DrawTopLegend(img *image.RGBA) {
	W := img.Bounds().Dx()
	s := hudScale(W)
	lh := 16 * s
	pad := 5 * s
	colW := 0
	for _, row := range hudLegend {
		for _, h := range row {
			if w := keyHintWidth(h, s); w > colW {
				colW = w
			}
		}
	}
	gap := pixel.TextWidth("  ", s)
	ox := W - pad - (2*colW + gap)
	for r, row := range hudLegend {
		for c, h := range row {
			drawKeyHint(img, ox+c*(colW+gap), pad+r*lh, s, h)
		}
	}
}

// splitPromptKey pulls a leading single-key action out of a prompt formatted
// "X - rest" (e.g. "e - wear the Crown"), returning the uppercased key and the
// remaining label. Prompts without that shape (walk-in portals, multi-key
// lectern hints) return key="" and render as plain text.
func splitPromptKey(text string) (key, label string) {
	if len(text) > 3 && text[1] == ' ' && text[2] == '-' && text[3] == ' ' {
		if unicode.IsLetter(rune(text[0])) {
			return strings.ToUpper(text[:1]), strings.TrimSpace(text[4:])
		}
	}
	return "", text
}

// DrawActionPrompt draws the contextual button near the bottom-center — what
// you can do right where you stand. A single-key action gets a keycap badge
// ("[E] wear the Crown"); other prompts (walk into a portal, multi-key lectern
// controls) render as plain text. It's only drawn when an area reports an
// action, so the bottom of the screen is empty the rest of the time.
func DrawActionPrompt(img *image.RGBA, text string) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	text = asciiOnly(strings.ReplaceAll(text, "—", "-"))
	if text == "" {
		return
	}
	key, label := splitPromptKey(text)

	padX, gap := 5*s, 4*s
	chW, chH := 7*s, 13*s // one glyph cell of the 7×13 font
	capW := 0
	if key != "" {
		capW = chW + 4*s + gap // chip (letter + padding) plus a gap before the label
	}
	label = truncToWidth(label, s, W-12*s-capW)

	boxW := padX*2 + capW + pixel.TextWidth(label, s)
	boxH := lh + 4*s
	x, y := (W-boxW)/2, H-3*lh
	pixel.Shade(img, x, y, boxW, boxH, 0.82)
	pixel.Frame(img, x, y, boxW, boxH)

	cx := x + padX
	ty := y + (boxH-chH)/2
	if key != "" {
		capInner, capH := chW+4*s, chH+2*s
		cy := y + (boxH-capH)/2
		fillRect(img, cx, cy, capInner, capH, hudAccent)
		pixel.DrawText(img, cx+2*s, cy+s, s, key, color.RGBA{0x10, 0x16, 0x20, 0xFF})
		cx += capInner + gap
	}
	pixel.DrawText(img, cx, ty, s, label, hudBright)
}

// DrawMenuPanel draws the Tab menu hub: the panels you can open, each with its
// direct key, the selected row marked. It's the single discoverable entry point
// that also teaches the shortcuts.
func DrawMenuPanel(img *image.RGBA, sel int) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 8 * s
	entries := MenuEntries()

	title, footer := "MENU", "up/down choose   Enter open   Tab close"
	rowW := 0
	for _, e := range entries {
		w := pixel.TextWidth("> "+e.Label+"     "+e.Key, s)
		if w > rowW {
			rowW = w
		}
	}
	contentW := rowW
	for _, t := range []string{title, footer} {
		if w := pixel.TextWidth(t, s); w > contentW {
			contentW = w
		}
	}
	ph := pad*2 + lh + lh/2 + len(entries)*lh + lh/2 + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	pixel.DrawText(img, ox+pad, oy+pad, s, title, hudAccent)
	y := oy + pad + lh + lh/2
	for i, e := range entries {
		col := hudWhite
		if i == sel {
			pixel.Shade(img, ox+pad-2*s, y-s, pw-2*pad+4*s, lh, 0.3)
			pixel.DrawText(img, ox+pad-2*s, y, s, ">", hudAccent)
			col = hudBright
		}
		pixel.DrawText(img, ox+pad+pixel.TextWidth("  ", s), y, s, e.Label, col)
		if e.Key != "" {
			pixel.DrawText(img, ox+pw-pad-pixel.TextWidth(e.Key, s), y, s, e.Key, hudDim)
		}
		y += lh
	}
	pixel.DrawText(img, ox+pad, oy+ph-pad-lh+lh/4, s, footer, hudDim)
}

// panelBox centers a pw×ph panel between the top legend band and the bottom
// chat/prompt band, clamping it on-screen, so a modal panel never lands on the
// always-on chrome.
func panelBox(W, H, pw, ph int) (ox, oy, cw int) {
	if pw > W-4 {
		pw = W - 4
	}
	lh := 16 * hudScale(W)
	top, bot := 2*lh, 3*lh
	ox, oy = (W-pw)/2, top+(H-top-bot-ph)/2
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

// helpRow is a line in a help column: a heading (head=true) or a key→desc pair.
type helpRow struct {
	key, desc string
	head      bool
}

// width returns the row's rendered width at scale s (key column padded so the
// monospace descriptions line up).
func (r helpRow) width(s int) int {
	if r.head {
		return pixel.TextWidth(r.desc, s)
	}
	return pixel.TextWidth(r.key+"   "+asciiOnly(r.desc), s)
}

// draw paints the row at (x,y): headings in accent, pairs as a bright key and a
// dim description.
func (r helpRow) draw(img *image.RGBA, x, y, s int) {
	switch {
	case r.head:
		pixel.DrawText(img, x, y, s, r.desc, hudAccent)
	case r.key != "" || r.desc != "":
		pixel.DrawText(img, x, y, s, r.key, hudBright)
		pixel.DrawText(img, x+pixel.TextWidth(r.key+"   ", s), y, s, asciiOnly(r.desc), hudDim)
	}
}

// DrawHelpPanel draws the "?" reference onto an HD frame in two columns —
// controls on the left, chat commands on the right — both from the shared
// Controls() / CommandReference() source, so the default client is as self-
// documenting as the glyph client. Two columns keep it short enough to fit a
// wide HD frame. Any key closes it (handled in the input loop).
func DrawHelpPanel(img *image.RGBA, ctx *Ctx) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 8 * s
	colGap := 6 * s

	// Left: controls, grouped. Key column padded to the widest control key.
	keyW := 0
	for _, g := range Controls() {
		for _, c := range g.Items {
			if len(c.Keys) > keyW {
				keyW = len(c.Keys)
			}
		}
	}
	var left []helpRow
	for _, g := range Controls() {
		left = append(left, helpRow{desc: g.Title, head: true})
		for _, c := range g.Items {
			left = append(left, helpRow{key: padRight(c.Keys, keyW), desc: c.Desc})
		}
	}

	// Right: chat commands. The left column explains the controls; here we only
	// need to reveal that the commands exist, so the usage strings (self-
	// descriptive, e.g. "/me <action>") stand alone — keeping the column narrow
	// enough for a wide HD frame.
	right := []helpRow{{desc: "Chat commands", head: true}}
	for _, c := range CommandReference() {
		right = append(right, helpRow{key: c[0]})
	}

	leftW, rightW := 0, 0
	for _, r := range left {
		if w := r.width(s); w > leftW {
			leftW = w
		}
	}
	for _, r := range right {
		if w := r.width(s); w > rightW {
			rightW = w
		}
	}
	bodyLines := len(left)
	if len(right) > bodyLines {
		bodyLines = len(right)
	}

	footer := "any key to close"
	ph := pad*2 + lh + lh/2 + bodyLines*lh + lh/2 + lh
	ox, oy, pw := panelBox(W, H, leftW+colGap+rightW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	pixel.DrawText(img, ox+pad, oy+pad, s, "CONTROLS", hudAccent)
	bodyY := oy + pad + lh + lh/2
	leftX, rightX := ox+pad, ox+pad+leftW+colGap
	for i, r := range left {
		r.draw(img, leftX, bodyY+i*lh, s)
	}
	for i, r := range right {
		r.draw(img, rightX, bodyY+i*lh, s)
	}
	pixel.DrawText(img, ox+pad, oy+ph-pad-lh+lh/4, s, footer, hudDim)
}

// DrawWhoPanel lists everyone online and where they are — the HD answer to Tab
// and /who, which the default client otherwise had no way to show.
func DrawWhoPanel(img *image.RGBA, ctx *Ctx) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 8 * s
	gap := "  "

	players := ctx.World.AllPlayers()
	type prow struct {
		name, where string
		self        bool
	}
	rows := make([]prow, 0, len(players))
	for _, p := range players {
		where := DisplayName(p.Area)
		if p.Area == "" {
			where = "connecting"
		}
		rows = append(rows, prow{asciiOnly(p.Name), asciiOnly("- " + where), p.Name == ctx.Name})
	}

	title := fmt.Sprintf("ONLINE  -  %d", len(players))
	footer := "any key to close"
	contentW := pixel.TextWidth(title, s)
	if w := pixel.TextWidth(footer, s); w > contentW {
		contentW = w
	}
	for _, r := range rows {
		w := pixel.TextWidth(r.name+"  (you)"+gap+r.where, s)
		if w > contentW {
			contentW = w
		}
	}
	bodyRows := len(rows)
	if bodyRows == 0 {
		bodyRows = 1
	}
	ph := pad*2 + lh + lh/2 + bodyRows*lh + lh/2 + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	y := oy + pad
	pixel.DrawText(img, ox+pad, y, s, title, hudAccent)
	y += lh + lh/2
	if len(rows) == 0 {
		pixel.DrawText(img, ox+pad, y, s, "nobody here yet", hudDim)
		y += lh
	}
	for _, r := range rows {
		name := r.name
		col := hudWhite
		if r.self {
			name += "  (you)"
			col = hudBright
		}
		pixel.DrawText(img, ox+pad, y, s, name, col)
		pixel.DrawText(img, ox+pad+pixel.TextWidth(name+gap, s), y, s, r.where, hudDim)
		y += lh
	}
	y += lh / 2
	pixel.DrawText(img, ox+pad, y, s, footer, hudDim)
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
// at the bottom-left of the frame.
func DrawChat(img *image.RGBA, lines []HDLine, active bool, input string) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	if len(lines) > hdChatLines {
		lines = lines[len(lines)-hdChatLines:]
	}

	bottom := H - 4*s
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
