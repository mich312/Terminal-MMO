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

// DrawClaimBanner draws the land-claim label as a quiet line just under the area
// title (top-left) — the HD counterpart to the glyph status-line label, shown
// only while the player stands in a claimed Workspace.
func DrawClaimBanner(img *image.RGBA, label string) {
	W := img.Bounds().Dx()
	s := hudScale(W)
	lh := 16 * s
	pad := 5 * s
	label = asciiOnly(label)
	if label == "" {
		return
	}
	x, y := pad, pad+lh // one line below the area title
	tw := pixel.TextWidth(label, s)
	pixel.Shade(img, x-2*s, y-2*s, tw+4*s, lh, 0.45)
	pixel.DrawText(img, x, y, s, label, hudAccent)
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

// DrawMinimapPanel draws an area's coarse overview as a grid of colored blocks —
// the HD twin of the glyph client's 'm' map. Explored cells show their terrain
// color, the player's block is bright, and unexplored ground is left dark so the
// chart fills in as you roam.
func DrawMinimapPanel(img *image.RGBA, title string, rows [][]MiniCell) {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 8 * s
	block := s + 1 // pixels per mini-cell — small, so a wide chart still fits
	cols := len(rows[0])
	gridW, gridH := cols*block, len(rows)*block

	footer := "M or move to close"
	contentW := gridW
	for _, t := range []string{title, footer} {
		if w := pixel.TextWidth(t, s); w > contentW {
			contentW = w
		}
	}
	ph := pad*2 + lh + lh/2 + gridH + lh/2 + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	pixel.DrawText(img, ox+pad, oy+pad, s, asciiOnly(title), hudAccent)
	gx, gy := ox+(pw-gridW)/2, oy+pad+lh+lh/2
	for r, row := range rows {
		for c, cell := range row {
			switch {
			case cell.Self:
				fillRect(img, gx+c*block, gy+r*block, block, block, hudBright)
			case cell.Hex != "":
				fillRect(img, gx+c*block, gy+r*block, block, block, colorfulToRGBA(mustHex(cell.Hex)))
			}
		}
	}
	pixel.DrawText(img, ox+pad, oy+ph-pad-lh+lh/4, s, footer, hudDim)
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
			// Keep the whole label (plus its shaded backing) on-screen, so a portal
			// near a left/right edge doesn't get its name clipped.
			if hi := W - tw - 2*s; lx > hi {
				lx = hi
			}
			if lx < 2*s {
				lx = 2 * s
			}
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

var (
	hudGood = color.RGBA{0x7B, 0xD8, 0x8F, 0xFF}
	hudWarn = color.RGBA{0xFF, 0xB4, 0x54, 0xFF}
)

// fillRectRGBA fills a rectangle with a flat color (clipped to the image).
func fillRectRGBA(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	for j := r.Min.Y; j < r.Max.Y; j++ {
		for i := r.Min.X; i < r.Max.X; i++ {
			img.SetRGBA(i, j, c)
		}
	}
}

// mins formats a seconds count as "6m 05s" or "42s".
func mins(sec int) string {
	if sec <= 0 {
		return "now"
	}
	if sec < 60 {
		return itoa(sec) + "s"
	}
	s := sec % 60
	pad := ""
	if s < 10 {
		pad = "0"
	}
	return itoa(sec/60) + "m " + pad + itoa(s) + "s"
}

// DrawMachinePanel draws a machine's offline-production readout: input/output
// meters, the "while you were away" delta, time-to-next, and Collect/Refuel
// actions. awayOut/awayIn are the gains recorded when the panel was opened.
func DrawMachinePanel(img *image.RGBA, ctx *Ctx, x, y, awayOut, awayIn int) {
	v, ok := MachineSnapshot(ctx, x, y)
	if !ok {
		return
	}
	k := v.Kind
	inName, outName := k.In, k.Out
	if it, ok := ItemByID(k.In); ok {
		inName = it.Name
	}
	if it, ok := ItemByID(k.Out); ok {
		outName = it.Name
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 10 * s

	footer := "e collect   f refuel   q close"
	rows := []string{
		k.Name, "status",
		"input  " + inName + " x" + itoa(v.State.In),
		"output  " + outName + " x" + itoa(v.State.Out) + "  (cap " + itoa(k.Cap) + ")",
		"+ " + itoa(awayOut) + " " + outName + "    - " + itoa(awayIn) + " " + inName,
		"next " + outName + " " + mins(v.NextSec) + "    rate 1 / " + itoa(int(k.Period.Seconds())) + "s",
		footer,
	}
	contentW := 360 * s / 2
	for _, t := range rows {
		if w := pixel.TextWidth(t, s); w > contentW {
			contentW = w
		}
	}
	ph := pad*2 + lh + lh/2 + lh + lh/4 + lh + lh/4 + lh + lh/2 + lh + lh + lh/4 + lh + lh + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)
	xx := ox + pad
	right := ox + pw - pad

	pixel.DrawText(img, xx, oy+pad, s, asciiOnly(k.Name), hudWarn)
	pixel.DrawText(img, right-pixel.TextWidth("owner: "+ctx.Name, s), oy+pad, s, "owner: "+ctx.Name, hudDim)
	yy := oy + pad + lh + lh/2

	pixel.DrawText(img, xx, yy, s, "status:", hudDim)
	status, sc := "IDLE", hudDim
	if v.Running {
		status, sc = "RUNNING", hudGood
	} else if v.State.Out >= k.Cap {
		status, sc = "FULL", hudWarn
	} else if v.State.In < k.InPer {
		status, sc = "NEEDS FUEL", hudWarn
	}
	pixel.DrawText(img, xx+pixel.TextWidth("status:  ", s), yy, s, status, sc)
	yy += lh + lh/4

	// meters
	barX := xx + pixel.TextWidth("output  ", s)
	inLabel := inName + " x" + itoa(v.State.In)
	outLabel := outName + " x" + itoa(v.State.Out) + "  (cap " + itoa(k.Cap) + ")"
	rw := pixel.TextWidth(outLabel, s)
	if w := pixel.TextWidth(inLabel, s); w > rw {
		rw = w
	}
	barW := right - rw - 8*s - barX
	inFill := v.State.In
	if inFill > k.Cap {
		inFill = k.Cap
	}
	drawMeter(img, barX, yy, barW, inFill, k.Cap, hudAccent)
	pixel.DrawText(img, xx, yy+s, s, "input", hudDim)
	pixel.DrawText(img, right-pixel.TextWidth(inLabel, s), yy+s, s, inLabel, hudWhite)
	yy += lh + lh/4
	drawMeter(img, barX, yy, barW, v.State.Out, k.Cap, hudWarn)
	pixel.DrawText(img, xx, yy+s, s, "output", hudDim)
	pixel.DrawText(img, right-pixel.TextWidth(outLabel, s), yy+s, s, outLabel, hudWhite)
	yy += lh + lh/2

	if awayOut > 0 || awayIn > 0 {
		pixel.DrawText(img, xx, yy, s, "while you were away:", hudAccent)
		yy += lh
		pixel.DrawText(img, xx+8*s, yy, s, "+ "+itoa(awayOut)+" "+outName, hudGood)
		pixel.DrawText(img, xx+8*s+pixel.TextWidth("+ "+itoa(awayOut)+" "+outName+"     ", s), yy, s,
			"- "+itoa(awayIn)+" "+inName, hudDim)
		yy += lh + lh/4
	}
	nextTxt := "stalled"
	if v.Running {
		nextTxt = "next " + outName + " " + mins(v.NextSec)
	}
	pixel.DrawText(img, xx, yy, s, nextTxt, hudDim)
	pixel.DrawText(img, right-pixel.TextWidth("rate 1 / "+itoa(int(k.Period.Seconds()))+"s", s), yy, s,
		"rate 1 / "+itoa(int(k.Period.Seconds()))+"s", hudDim)

	// actions
	fy := oy + ph - pad - lh + lh/4
	collect := "[ e  COLLECT " + itoa(v.State.Out) + " ]"
	cw := pixel.TextWidth(collect, s) + 8*s
	pixel.Shade(img, xx, fy-3*s, cw, lh, 0.4)
	pixel.Frame(img, xx, fy-3*s, cw, lh)
	cc := hudGood
	if v.State.Out == 0 {
		cc = hudDim
	}
	pixel.DrawText(img, xx+4*s, fy, s, collect, cc)
	refuel := "[ f  REFUEL ]"
	rwid := pixel.TextWidth(refuel, s) + 8*s
	rx := xx + cw + 10*s
	pixel.Shade(img, rx, fy-3*s, rwid, lh, 0.4)
	pixel.Frame(img, rx, fy-3*s, rwid, lh)
	pixel.DrawText(img, rx+4*s, fy, s, refuel, hudWarn)
}

// DrawStallPanel draws a Durst Group Concession: the offer list (give ⇄ ask)
// with stock and a green/amber affordability mark, plus the till for the owner.
// sel is the highlighted offer. When compose is set (owners only), the panel
// shows the in-panel offer composer driven by d instead of the list.
func DrawStallPanel(img *image.RGBA, ctx *Ctx, x, y, sel int, compose bool, d OfferDraft) {
	st, ok := StallSnapshot(ctx, x, y)
	if !ok {
		return
	}
	owner := StallOwner(ctx, x, y)
	if compose && owner {
		drawStallComposer(img, ctx, d)
		return
	}
	name := func(id string) string {
		if it, ok := ItemByID(id); ok {
			return it.Name
		}
		return id
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 10 * s
	box := 9 * s

	title := "DURST GROUP CONCESSION"
	footer := "up/down offer   e buy   q close"
	if owner {
		footer = "up/down offer   n new   f collect   x remove   q close"
	}

	type row struct {
		give, ask, stock string
		can, dim         bool
	}
	var rows []row
	for _, o := range st.Offers {
		rows = append(rows, row{
			give:  itoa(o.GiveN) + " " + name(o.GiveItem),
			ask:   itoa(o.AskN) + " " + name(o.AskItem),
			stock: "x" + itoa(o.Stock/maxi(o.GiveN, 1)),
			can:   CanAcceptOffer(o, invOf(ctx)),
			dim:   o.Stock < o.GiveN,
		})
	}

	// width
	giveCol, askCol := 0, 0
	for _, r := range rows {
		if w := pixel.TextWidth(r.give, s); w > giveCol {
			giveCol = w
		}
		if w := pixel.TextWidth(r.ask, s); w > askCol {
			askCol = w
		}
	}
	rowW := box + 4*s + giveCol + pixel.TextWidth("  <->  ", s) + box + 4*s + askCol + pixel.TextWidth("  x00", s)
	contentW := rowW
	for _, t := range []string{title, footer} {
		if w := pixel.TextWidth(t, s); w > contentW {
			contentW = w
		}
	}
	nrows := len(rows)
	if nrows == 0 {
		nrows = 1
	}
	ph := pad*2 + lh + lh/2 + nrows*lh + lh + lh/2 + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)
	xx := ox + pad
	right := ox + pw - pad

	pixel.DrawText(img, xx, oy+pad, s, title, hudAccent)
	if !owner {
		if pl, ok := ctx.World.PlacementAt(x, y); ok {
			pixel.DrawText(img, right-pixel.TextWidth(pl.Owner+"'s", s), oy+pad, s, pl.Owner+"'s", hudDim)
		}
	}
	yy := oy + pad + lh + lh/2

	if len(rows) == 0 {
		hint := "no offers yet"
		if owner {
			hint = "no offers yet — press n to post one"
		}
		pixel.DrawText(img, xx, yy, s, hint, hudDim)
	}
	askX := xx + box + 4*s + giveCol + pixel.TextWidth("  <->  ", s)
	for i, r := range rows {
		if i == sel {
			pixel.Shade(img, xx-2*s, yy-2*s, pw-2*pad+4*s, lh, 0.3)
			pixel.DrawText(img, xx-s, yy, s, ">", hudAccent)
		}
		o := st.Offers[i]
		itemSwatch(img, xx+4*s, yy, box, mustItemHex(o.GiveItem))
		gc := hudWhite
		if r.dim {
			gc = hudDim
		}
		pixel.DrawText(img, xx+box+8*s, yy, s, r.give, gc)
		pixel.DrawText(img, xx+box+8*s+giveCol+4*s, yy, s, "<->", hudDim)
		itemSwatch(img, askX, yy, box, mustItemHex(o.AskItem))
		ac := hudWhite
		if i == sel {
			if r.can {
				ac = hudGood
			} else {
				ac = hudWarn
			}
		}
		pixel.DrawText(img, askX+box+4*s, yy, s, r.ask, ac)
		pixel.DrawText(img, right-pixel.TextWidth(r.stock, s), yy, s, r.stock, hudDim)
		yy += lh
	}

	if owner {
		till := 0
		for _, n := range st.Till {
			till += n
		}
		yy += lh / 4
		pixel.DrawText(img, xx, yy, s, "till: "+itoa(till)+" items waiting", hudGood)
	}
	pixel.DrawText(img, xx, oy+ph-pad-lh+lh/4, s, footer, hudDim)
}

// drawStallComposer renders the in-panel offer authoring form (the HD twin of
// /sell): four editable rows — give item, give count, ask item, ask count — with
// the selected field marked, plus a live "stocks N (M sales)" line and a
// validity-tinted post hint.
func drawStallComposer(img *image.RGBA, ctx *Ctx, d OfferDraft) {
	name := func(id string) string {
		if it, ok := ItemByID(id); ok {
			return it.Name
		}
		return id
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 10 * s
	box := 9 * s

	title := "POST AN OFFER"
	footer := "up/down field   left/right change   e post   q back"

	type frow struct {
		label, val string
		swatch     string // item hex for a leading gem, "" for none
	}
	rows := []frow{
		{label: "Give", val: name(d.GiveItem), swatch: mustItemHex(d.GiveItem)},
		{label: "Per sale", val: itoa(d.GiveN)},
		{label: "For", val: name(d.AskItem), swatch: mustItemHex(d.AskItem)},
		{label: "Per sale", val: itoa(d.AskN)},
	}

	units, sales := DraftStock(ctx, d)
	stockLine := "stocks " + itoa(units) + " " + name(d.GiveItem) + " (" + itoa(sales) + " sales)"

	labelCol := 0
	for _, r := range rows {
		if w := pixel.TextWidth(r.label, s); w > labelCol {
			labelCol = w
		}
	}
	rowW := 2*s + labelCol + 8*s + box + 6*s + pixel.TextWidth("Glittering Geode", s)
	contentW := rowW
	for _, t := range []string{title, footer, stockLine} {
		if w := pixel.TextWidth(t, s); w > contentW {
			contentW = w
		}
	}

	ph := pad*2 + lh + lh/2 + OfferFields*lh + lh/2 + lh + lh/2 + lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)
	xx := ox + pad

	pixel.DrawText(img, xx, oy+pad, s, title, hudAccent)
	yy := oy + pad + lh + lh/2
	valX := xx + 2*s + labelCol + 8*s
	for i, r := range rows {
		if i == d.Field {
			pixel.Shade(img, xx-2*s, yy-2*s, pw-2*pad+4*s, lh, 0.3)
			pixel.DrawText(img, xx-s, yy, s, ">", hudAccent)
		}
		lc := hudDim
		if i == d.Field {
			lc = hudWhite
		}
		pixel.DrawText(img, xx+2*s, yy, s, r.label, lc)
		vx := valX
		if r.swatch != "" {
			itemSwatch(img, vx, yy, box, r.swatch)
			vx += box + 6*s
		}
		pixel.DrawText(img, vx, yy, s, r.val, hudWhite)
		yy += lh
	}

	yy += lh / 2
	sc := hudGood
	if !DraftValid(ctx, d) {
		sc = hudWarn
	}
	pixel.DrawText(img, xx, yy, s, stockLine, sc)
	pixel.DrawText(img, xx, oy+ph-pad-lh+lh/4, s, footer, hudDim)
}

// DrawBuildPanel draws the build palette: the buildable catalog grouped
// (Structures · Machines · Trade · Tools), each row with a 1-9 hotbar badge, its
// cost, and a right-aligned afford count — dimmed when the pack can't afford it.
// The selected row is highlighted and its blurb shown, plus a block reason when
// the ghost is on a bad cell. Left-anchored and non-modal, since build mode keeps
// the ghost live. sel is the highlighted Placeables index; footer is a context
// line (claim hint or block reason), shown amber when warn.
func DrawBuildPanel(img *image.RGBA, ctx *Ctx, sel int, footer string, warn bool) {
	groups := BuildPalette(ctx)
	if len(groups) == 0 {
		return
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 14 * s
	pad := 8 * s

	title := "BUILD"
	keys := "1-9/r pick  e place  x remove  b done"

	// Selected entry, for the blurb line.
	var cur Placeable
	for _, g := range groups {
		for _, e := range g.Entries {
			if e.Index == sel {
				cur = e.P
			}
		}
	}
	blurb := ""
	if cur.ID != "" {
		blurb = "\"" + cur.Blurb + "\""
	}

	// Width: the widest row "[n] Name   cost   x000", plus title/footer/blurb.
	rowText := func(e PaletteEntry) (string, string, string) {
		badge := "   "
		if e.Hotkey > 0 {
			badge = "[" + itoa(e.Hotkey) + "]"
		}
		cnt := "x" + itoa(e.Max)
		if e.P.Cat == CatTool {
			cnt = "ready" // a tool is wielded, not built
		}
		return badge + " " + e.P.Name, PlaceableCost(e.P), cnt
	}
	nameCol, costCol := 0, 0
	rows := 0
	for _, g := range groups {
		rows += 1 + len(g.Entries)
		for _, e := range g.Entries {
			n, c, _ := rowText(e)
			if w := pixel.TextWidth(n, s); w > nameCol {
				nameCol = w
			}
			if w := pixel.TextWidth(c, s); w > costCol {
				costCol = w
			}
		}
	}
	gap := 5 * s
	rowW := nameCol + gap + costCol + gap + pixel.TextWidth("x000", s)
	contentW := rowW
	for _, t := range []string{title, keys, blurb, footer} {
		if w := pixel.TextWidth(asciiOnly(t), s); w > contentW {
			contentW = w
		}
	}
	ph := pad*2 + lh + lh/2 + rows*lh + lh/2 + lh + lh
	// Centered like the other panels (panelBox), on a translucent shade + frame
	// rather than an opaque card, so the player and the ghost stay visible through
	// the palette while build mode keeps the world live.
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.Shade(img, ox, oy, pw, ph, 0.7)
	pixel.Frame(img, ox, oy, pw, ph)
	x := ox + pad
	right := ox + pw - pad

	pixel.DrawText(img, x, oy+pad, s, title, hudAccent)
	y := oy + pad + lh + lh/2
	costX := x + nameCol + gap
	for _, g := range groups {
		pixel.DrawText(img, x, y, s, g.Name, hudDim)
		y += lh
		for _, e := range g.Entries {
			name, cost, cnt := rowText(e)
			nc, cc, xc := hudWhite, hudGood, hudGood
			if !e.Afford {
				nc, cc, xc = hudDim, hudDim, hudDim
			}
			if e.Index == sel {
				pixel.Shade(img, x-2*s, y-2*s, pw-2*pad+4*s, lh, 0.32)
				pixel.DrawText(img, x-s, y, s, ">", hudAccent)
				if e.Afford {
					nc = hudBright
				}
			}
			pixel.DrawText(img, x, y, s, name, nc)
			pixel.DrawText(img, costX, y, s, cost, cc)
			pixel.DrawText(img, right-pixel.TextWidth(cnt, s), y, s, cnt, xc)
			y += lh
		}
	}
	y += lh / 2
	if blurb != "" {
		pixel.DrawText(img, x, y, s, blurb, hudDim)
	}
	y += lh
	switch {
	case footer != "" && warn:
		pixel.DrawText(img, x, y, s, hudLine(footer), hudWarn)
	case footer != "":
		pixel.DrawText(img, x, y, s, hudLine(footer), hudAccent)
	default:
		pixel.DrawText(img, x, y, s, keys, hudDim)
	}
}

func mustItemHex(id string) string {
	if it, ok := ItemByID(id); ok {
		return it.Hex
	}
	return "#9AA3AD"
}

func drawMeter(img *image.RGBA, x, y, w, filled, total int, c color.RGBA) {
	s := hudScale(img.Bounds().Dx())
	h := 8 * s
	fillRectRGBA(img, x, y, w, h, color.RGBA{0x2A, 0x30, 0x38, 0xFF})
	if total > 0 {
		f := w * filled / total
		if f > w {
			f = w
		}
		fillRectRGBA(img, x, y, f, h, c)
	}
	pixel.Frame(img, x, y, w, h)
}

// itemSwatch draws an item's gem icon (by hex color) at (x,y), box wide.
func itemSwatch(img *image.RGBA, x, y, box int, hex string) {
	c := mustHex(hex)
	drawGem(img, x, y, box,
		colorfulToRGBA(c),
		colorfulToRGBA(c.BlendLab(spriteWhite, 0.55).Clamped()))
}

// invOf returns the ctx's live inventory, nil-safe.
func invOf(ctx *Ctx) map[string]int {
	if ctx.Inventory == nil {
		return map[string]int{}
	}
	return ctx.Inventory
}

// DrawCraftPanel draws the Crafting (Self-Service) station onto an HD frame: the
// recipe list with a live craftable count per row, and a detail block for the
// selected recipe — its blurb, its inputs (green when the pack can afford each,
// amber when short) and its yield. sel is the highlighted row.
func DrawCraftPanel(img *image.RGBA, ctx *Ctx, sel int) {
	rs := Recipes
	if len(rs) == 0 {
		return
	}
	if sel < 0 {
		sel = 0
	}
	if sel >= len(rs) {
		sel = len(rs) - 1
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 9 * s
	box := 9 * s
	inv := invOf(ctx)
	cur := rs[sel]

	footer := "up/down choose   e craft   q close"

	// Column geometry: a name column wide enough for the longest recipe, then a
	// needs column, then a right-aligned [xN] craftable count.
	nameCol, needsCol := 0, 0
	for _, r := range rs {
		if w := pixel.TextWidth(r.Name, s); w > nameCol {
			nameCol = w
		}
		if w := pixel.TextWidth(RecipeNeeds(r), s); w > needsCol {
			needsCol = w
		}
	}
	rowW := 10*s + nameCol + 8*s + needsCol + 8*s + pixel.TextWidth("[x000]", s)

	blurb := "\"" + cur.Blurb + "\""
	contentW := rowW
	for _, t := range []string{"CRAFTING  (Self-Service)", footer, blurb} {
		if w := pixel.TextWidth(t, s); w > contentW {
			contentW = w
		}
	}

	// list + divider + blurb + needs + yields + count/hint + footer
	ph := pad*2 + (lh + lh/2) + len(rs)*lh + (lh/2 + lh) + lh + lh + lh + (lh/2 + lh)
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)
	x := ox + pad
	right := ox + pw - pad

	pixel.DrawText(img, x, oy+pad, s, "CRAFTING", hudAccent)
	pixel.DrawText(img, x+pixel.TextWidth("CRAFTING  ", s), oy+pad, s, "(Self-Service)", hudDim)

	y := oy + pad + lh + lh/2
	needsX := x + 10*s + nameCol + 8*s
	for i, r := range rs {
		can := Craftable(r, inv)
		col, cc := hudWhite, hudGood
		if can == 0 {
			cc = hudDim
		}
		if i == sel {
			pixel.Shade(img, x-2*s, y-2*s, pw-2*pad+4*s, lh, 0.3)
			pixel.DrawText(img, x-s, y, s, ">", hudAccent)
			col = hudBright
		}
		pixel.DrawText(img, x+10*s, y, s, r.Name, col)
		pixel.DrawText(img, needsX, y, s, RecipeNeeds(r), hudDim)
		count := "[x" + itoa(can) + "]"
		pixel.DrawText(img, right-pixel.TextWidth(count, s), y, s, count, cc)
		y += lh
	}

	// divider
	y += lh / 4
	pixel.Shade(img, x, y, pw-2*pad, s, 0.5)
	y += lh/2 + lh/4

	// selected detail: blurb, the inputs with affordability, the yield.
	pixel.DrawText(img, x, y, s, blurb, hudDim)
	y += lh + lh/4
	pixel.DrawText(img, x, y, s, "needs:", hudDim)
	ix := x + pixel.TextWidth("needs:  ", s)
	for _, in := range cur.In {
		it, _ := ItemByID(in.Item)
		itemSwatch(img, ix, y, box, it.Hex)
		ix += box + 3*s
		label := itoa(in.N) + " " + it.Name
		lc := hudGood
		if inv[in.Item] < in.N {
			lc = hudWarn
		}
		pixel.DrawText(img, ix, y, s, label, lc)
		ix += pixel.TextWidth(label+"   ", s)
	}
	y += lh
	out, _ := ItemByID(cur.Out)
	pixel.DrawText(img, x, y, s, "yields:", hudDim)
	yx := x + pixel.TextWidth("yields:  ", s)
	itemSwatch(img, yx, y, box, out.Hex)
	pixel.DrawText(img, yx+box+3*s, y, s, out.Name+" x"+itoa(cur.OutN), hudWhite)

	// footer + craft button
	fy := oy + ph - pad - lh + lh/4
	pixel.DrawText(img, x, fy, s, footer, hudDim)
	if n := Craftable(cur, inv); n > 0 {
		btn := "[ e CRAFT ]"
		bw := pixel.TextWidth(btn, s) + 8*s
		bx := right - bw
		pixel.Shade(img, bx, fy-3*s, bw, lh, 0.4)
		pixel.Frame(img, bx, fy-3*s, bw, lh)
		pixel.DrawText(img, bx+4*s, fy, s, btn, hudGood)
	}
}

// compLine is one rendered row of the HD compendium: text in a color, optionally
// preceded by an item portrait or an accessory icon, at a left indent measured
// in icon-gutter widths.
type compLine struct {
	text   string
	col    color.RGBA
	indent int    // 0 flush, 1 indented under an entry
	item   *Item  // draw this item's portrait before the text
	dim    bool   // render the portrait dimmed (not yet found / sighted)
	acc    int    // >0: draw this accessory's icon before the text
	crit   string // non-empty: draw this species' portrait before the text
}

// hdInline makes a glyph-client string safe for the HD bitmap font (ASCII only),
// keeping the meaning of the punctuation the catalog uses.
func hdInline(s string) string {
	return asciiOnly(strings.NewReplacer(
		"—", " - ", "·", "-", "×", "x", "’", "'", "“", "", "”", "").Replace(s))
}

// compendiumStats counts how many item kinds the player has found, of the total.
func compendiumStats(inv map[string]int) (found, kinds int) {
	for _, it := range Items {
		kinds++
		if inv[it.ID] > 0 {
			found++
		}
	}
	return found, kinds
}

// compendiumLinesHD flattens the catalog and wearables into rows for the HD
// panel, mirroring the glyph /compendium: items grouped by source (owned ones
// lit with a count, unfound ones dimmed) each with a line on what it does, then
// the wearables and their powers.
func compendiumLinesHD(ctx *Ctx) []compLine {
	var ls []compLine
	for _, g := range Compendium(ctx.Inventory) {
		ls = append(ls, compLine{text: strings.ToUpper(g.Title), col: hudAccent})
		for _, e := range g.Entries {
			it := e.Item
			head, col := "", hudWhite
			if e.Owned > 0 {
				head = fmt.Sprintf("%s   x%d   (%s)", it.Name, e.Owned, it.Rarity)
			} else {
				head, col = fmt.Sprintf("%s   -   (%s)", it.Name, it.Rarity), hudDim
			}
			itc := it
			ls = append(ls, compLine{text: hdInline(head), col: col, item: &itc, dim: e.Owned == 0})
			sub := e.Note // what it does; fall back to the flavor for plain materials
			if sub == "" {
				sub = it.About
			}
			ls = append(ls, compLine{text: hdInline(sub), col: hudDim, indent: 1})
			ls = append(ls, compLine{}) // breathing room below the portrait
		}
		ls = append(ls, compLine{}) // spacer between groups
	}
	ls = append(ls, compLine{text: "WEARABLES", col: hudAccent})
	for _, w := range Wearables(ctx) {
		power := w.Power
		if power == "" {
			power = "cosmetic"
		}
		text, col := "", hudWhite
		if w.Owned {
			text = w.Name
			if w.Worn {
				text += "  (worn)"
			}
			text += "   " + power
		} else {
			text, col = w.Name+"   "+power+" - "+w.Source, hudDim
		}
		ls = append(ls, compLine{text: hdInline(text), col: col, acc: w.Index})
	}

	ls = append(ls, compLine{})
	sighted, total := BestiaryStats(ctx.Compendium)
	ls = append(ls, compLine{text: fmt.Sprintf("WILDLIFE  %d/%d SIGHTED", sighted, total), col: hudAccent})
	for _, b := range Bestiary(ctx.Compendium) {
		if b.Seen {
			ls = append(ls, compLine{text: hdInline(b.Name + "   " + b.Habitat), col: hudWhite, crit: b.Kind})
			ls = append(ls, compLine{text: hdInline("drops " + b.Drops), col: hudDim, indent: 1})
			if b.Tame != "" {
				ls = append(ls, compLine{text: hdInline("tame with a " + b.Tame), col: hudDim, indent: 1})
			}
		} else {
			ls = append(ls, compLine{text: "? ? ?", col: hudDim, crit: b.Kind, dim: true})
			ls = append(ls, compLine{text: "not yet sighted", col: hudDim, indent: 1})
		}
	}
	return ls
}

// DrawCompendiumPanel draws the items-and-wearables codex onto an HD frame: the
// full catalog (owned finds lit with their count, unfound ones dimmed) with what
// each is and does, then every wearable and its power. The listing is taller
// than the screen, so it scrolls — *scroll is clamped here against the live
// layout (its upper bound depends on the frame height) and the input loop just
// nudges it. Reuses the 'i' key and Tab menu.
func DrawCompendiumPanel(img *image.RGBA, ctx *Ctx, scroll *int) {
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	s := hudScale(W)
	lh := 16 * s
	pad := 8 * s
	accBox := lh - 4*s        // wearable icon box, fitted to a text line
	accGutter := accBox + s*3 // space a wearable icon takes before its text
	itemBox := lh*2 - 2*s     // an item portrait spans the entry's two lines
	itemGutter := itemBox + s*3
	indentPx := itemGutter // an entry's sub-line aligns under its header text

	lines := compendiumLinesHD(ctx)
	found, kinds := compendiumStats(ctx.Inventory)
	title := fmt.Sprintf("COMPENDIUM  -  %d / %d found", found, kinds)

	// Size the body to fill the frame between panelBox's top/bottom margins.
	avail := H - 5*lh
	chrome := pad*2 + lh + lh/2 + lh/2 + lh // padding, title, two half-gaps, footer
	bodyRows := (avail - chrome) / lh
	if bodyRows < 3 {
		bodyRows = 3
	}
	if bodyRows > len(lines) {
		bodyRows = len(lines)
	}
	maxScroll := len(lines) - bodyRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if *scroll > maxScroll {
		*scroll = maxScroll
	}
	if *scroll < 0 {
		*scroll = 0
	}

	contentW := pixel.TextWidth(title, s)
	for _, l := range lines {
		lead := l.indent * indentPx
		switch {
		case l.item != nil, l.crit != "":
			lead = itemGutter
		case l.acc > 0:
			lead = accGutter
		}
		if w := lead + pixel.TextWidth(l.text, s); w > contentW {
			contentW = w
		}
	}

	ph := chrome + bodyRows*lh
	ox, oy, pw := panelBox(W, H, contentW+pad*2, ph)
	pixel.DrawPanel(img, ox, oy, pw, ph)

	top := oy + pad
	pixel.DrawText(img, ox+pad, top, s, title, hudAccent)

	y := top + lh + lh/2
	end := *scroll + bodyRows
	if end > len(lines) {
		end = len(lines)
	}
	bodyTop := top + lh + lh/2
	for _, l := range lines[*scroll:end] {
		x := ox + pad + l.indent*indentPx
		switch {
		case l.item != nil:
			iy := y - (itemBox-lh)/2 // center the portrait across the entry's two lines
			if iy < bodyTop {
				iy = bodyTop
			}
			drawItemIcon(img, ox+pad, iy, itemBox, *l.item)
			if l.dim { // not yet found — show the silhouette, darkened
				pixel.Shade(img, ox+pad, iy, itemBox, itemBox, 0.55)
			}
			x = ox + pad + itemGutter
		case l.crit != "":
			iy := y - (itemBox-lh)/2 // center the portrait across the entry's lines
			if iy < bodyTop {
				iy = bodyTop
			}
			drawCreatureIcon(img, ox+pad, iy, itemBox, l.crit)
			if l.dim { // not yet sighted — a darkened silhouette
				pixel.Shade(img, ox+pad, iy, itemBox, itemBox, 0.6)
			}
			x = ox + pad + itemGutter
		case l.acc > 0:
			drawAccessoryIcon(img, x, y+(lh-accBox)/2, accBox, l.acc)
			x += accGutter
		}
		if l.text != "" {
			pixel.DrawText(img, x, y, s, l.text, l.col)
		}
		y += lh
	}

	footer := "i or q: close"
	if maxScroll > 0 {
		hint := ""
		if *scroll > 0 {
			hint += "up "
		}
		if end < len(lines) {
			hint += "down "
		}
		footer = hint + "scroll  -  i/q close"
	}
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
// hudLine prepares a UI string for the ASCII basicfont: the em-dash used in
// prompts (nice in the glyph terminal) becomes a hyphen rather than vanishing.
func hudLine(s string) string { return asciiOnly(strings.ReplaceAll(s, "—", "-")) }

func asciiOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7F {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
