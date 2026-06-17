package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/pixel"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// HD ("real pixel") mode is the default sixel/kitty renderer, served on a plain
// interactive connection:  ssh -p 2222 you@host  (opt back into the classic
// glyph client with `ssh -t -p 2222 you@host glyph`).
//
// It deliberately bypasses bubbletea — image escapes are out-of-band and would
// fight bubbletea's frame diffing — and writes graphics frames straight to the
// SSH session. It drives the same game.Area objects as the glyph client (so it
// reuses their spawn, movement and portal logic) and works in every area, not
// just the Wilds: walk through a portal and the destination renders in pixels
// too. It shares the live world, so HD and bubbletea players see each other.
const (
	hdScale   = 36 // pixels per tile — larger on-screen tiles
	hdFPS     = 12
	hdRefresh = 48 // frames between full repaints
	hdMaxTile = 140
	// Cap the rendered buffer so per-frame cost (render + dirty-diff + encode +
	// bandwidth) doesn't scale with window size — the work is ~width·height
	// pixels, and at hdScale=24 a full-screen buffer is huge. This bounds it to
	// ≈hdMaxPxW×hdMaxPxH regardless of terminal size; a bigger window just shows
	// the same world slice at the same cost, not more.
	hdMaxPxW = 1024
	hdMaxPxH = 640
)

// HD UI panels reachable with single keys (HD has no command line).
const (
	hdPanelNone = iota
	hdPanelChar
	hdPanelInv
)

// setupAvatar restores a player's persisted color/style/accessory, or — on a
// first visit — rolls a random look and remembers it, so everyone spawns with a
// distinct avatar that then stays theirs across reconnects.
func setupAvatar(w *world.World, st store.Store, name string) {
	if color, style, accessory, ok := st.LoadAvatar(name); ok {
		if color != "" {
			w.SetColor(name, lipgloss.Color(color))
		}
		w.SetAvatar(name, style, accessory)
		// Grandfather a hat the player is already wearing into their owned set,
		// so gating stays consistent for anyone from before hats were earned.
		if accessory != 0 {
			st.UnlockHat(name, accessory)
		}
		return
	}
	// New players spawn with a random body/color but no hat — hats are earned by
	// exploring the Wilds.
	color := ui.AvatarColorByIndex(rand.Intn(ui.NumAvatarColors()))
	style := rand.Intn(game.NumAvatarStyles())
	w.SetColor(name, color)
	w.SetAvatar(name, style, 0)
	st.SaveAvatar(name, string(color), style, 0)
}

// wantsClassic reports whether a session asked for the classic glyph client
// (`ssh -t host glyph`) instead of the default HD renderer.
func wantsClassic(s ssh.Session) bool { return cmdWantsClassic(s.Command()) }

// cmdWantsClassic is wantsClassic over a raw command slice, split out so the
// arg matching is testable without an ssh.Session.
func cmdWantsClassic(cmd []string) bool {
	for _, a := range cmd {
		if a == "glyph" {
			return true
		}
	}
	return false
}

// hdMiddleware serves the HD (sixel/kitty) renderer by default and falls
// through to the bubbletea glyph client only when a session explicitly opts
// out (e.g. `ssh -t host glyph`). Serving HD on a plain `ssh host` is what lets
// it work without a forced `-t`: an interactive session gets a PTY for free,
// whereas a command session (`ssh host hd`) would need `-t` to allocate one.
// Placed inside activeterm so a PTY is guaranteed. style is the server-wide art
// style for HD frames.
func hdMiddleware(w *world.World, st store.Store, style *game.Style) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			if wantsClassic(s) {
				next(s)
				return
			}
			runHD(s, w, st, style)
		}
	}
}

// preferKitty decides whether to drive the session with the kitty graphics
// protocol instead of sixel, inferred from TERM: kitty and ghostty speak the
// kitty protocol (and ghostty has no sixel), so they get kitty; everything else
// gets sixel.
func preferKitty(term string) bool {
	return strings.Contains(term, "kitty") || strings.Contains(term, "ghostty")
}

func runHD(s ssh.Session, w *world.World, st store.Store, style *game.Style) {
	ptyReq, winCh, ok := s.Pty()
	if !ok {
		fmt.Fprint(s, "HD mode needs a PTY — use: ssh -t -p 2222 you@host hd\r\n")
		return
	}
	// Ghostty/kitty speak the kitty graphics protocol and have no sixel; other
	// terminals (iTerm2, WezTerm, xterm) use sixel. Pick from the forwarded TERM.
	useKitty := preferKitty(ptyReq.Term)
	proto := "sixel"
	if useKitty {
		proto = "kitty"
	}

	name, events := w.Join(s.User())
	st.RecordVisit(name)
	setupAvatar(w, st, name)
	log.Printf("%s connected (HD/%s, TERM=%s)", name, proto, ptyReq.Term)
	defer func() {
		w.Leave(name)
		st.RecordDisconnect(name)
		log.Printf("%s disconnected (HD)", name)
	}()
	go func() { // HD polls the world each frame; drain presence events so senders never block
		for range events {
		}
	}()

	ctx := &game.Ctx{World: w, Store: st, Name: name,
		Inventory: st.LoadInventory(name), Hats: st.LoadHats(name)}
	areaID, area, hv := enterHD(ctx, "", "wilds")

	cellW, cellH := hdCellSize(ptyReq.Window)
	win := ptyReq.Window

	out := bufio.NewWriterSize(s, 1<<20)
	out.WriteString("\x1b[2J\x1b[?25l")
	defer func() { out.WriteString("\x1b[?25h\x1b[0m\r\n"); out.Flush() }()

	keys := make(chan byte, 256)
	go func() {
		b := make([]byte, 64)
		for {
			n, err := s.Read(b)
			for i := 0; i < n; i++ {
				keys <- b[i]
			}
			if err != nil {
				close(keys)
				return
			}
		}
	}()

	fw := &pixel.FrameWriter{Kitty: useKitty, CellW: cellW, CellH: cellH}
	var (
		frame      int
		sent       int
		framesSent int
		start      = time.Now()
		uiPanel    = hdPanelNone // which on-frame UI panel is open
		uiField    int           // selected field in the character panel
	)

	draw := func() {
		cols, rows := win.Width, win.Height
		if cols < 8 || rows < 6 {
			return
		}
		vw := clamp(cols*cellW/hdScale, 8, min(hdMaxTile, hdMaxPxW/hdScale))
		vh := clamp((rows-1)*cellH/hdScale, 8, min(hdMaxTile, hdMaxPxH/hdScale))
		tm, ox, oy := hv.HDView(vw, vh)
		cam := game.Camera{W: vw, H: vh}
		light := game.Light{}
		if l, ok := area.(game.HDLighter); ok {
			light = l.HDLight() // the Wilds' discovery circle
		}
		img := game.RenderRGBA(nil, tm, w.PlayersInArea(areaID), name, frame, cam, light, ox, oy, hdScale, false, style)

		// Draw an area's on-screen text (a presentation slide) into the frame —
		// HD has no glyph layer, so slides are rasterized straight onto the image.
		if ov, ok := area.(game.HDOverlayer); ok {
			if src, footer, show := ov.HDSlide(); show {
				pixel.DrawSlidePanel(img, src, footer)
			}
		}

		// HD UI overlays: the status/hint bar, a transient pickup toast, and any
		// open panel — so the default client carries the full interface.
		hint := ""
		if h, ok := area.(game.Hinter); ok {
			hint = h.Hint()
		}
		game.DrawHUD(img, area.Name(), hint)
		if tz, ok := area.(game.Toaster); ok {
			if msg, show := tz.Toast(); show {
				game.DrawToast(img, msg)
			}
		}
		switch uiPanel {
		case hdPanelChar:
			game.DrawCharPanel(img, ctx, uiField)
		case hdPanelInv:
			game.DrawInventoryPanel(img, ctx)
		}

		sent += fw.WriteFrame(out, img, frame%hdRefresh == 0)
		out.Flush()
		framesSent++
	}

	hud := func() {
		dur := time.Since(start).Seconds()
		if dur <= 0 {
			dur = 1
		}
		out.WriteString("\x1b[?25h\x1b[0m\r\n")
		fmt.Fprintf(out, "HD session over: %d frames, %.0f KB sent, %.1f KB/s avg over %.0fs\r\n",
			framesSent, float64(sent)/1024, float64(sent)/1024/dur, dur)
		out.Flush()
	}

	// uiKey handles HD interface keys: 'c'/'i' toggle the character/inventory
	// panels, and while a panel is open the arrows navigate/edit it and every
	// other key is swallowed. Returns whether the key was consumed by the UI.
	uiKey := func(key string) bool {
		switch key {
		case "c":
			if uiPanel == hdPanelChar {
				uiPanel = hdPanelNone
			} else {
				uiPanel, uiField = hdPanelChar, 0
			}
			return true
		case "i":
			if uiPanel == hdPanelInv {
				uiPanel = hdPanelNone
			} else {
				uiPanel = hdPanelInv
			}
			return true
		}
		if uiPanel == hdPanelNone {
			return false
		}
		switch key {
		case "up":
			uiField = (uiField + game.CharFields - 1) % game.CharFields
		case "down":
			uiField = (uiField + 1) % game.CharFields
		case "left":
			if uiPanel == hdPanelChar {
				game.CycleAvatarField(ctx, uiField, -1)
			}
		case "right":
			if uiPanel == hdPanelChar {
				game.CycleAvatarField(ctx, uiField, 1)
			}
		}
		return true // a panel swallows everything else
	}

	draw()
	ticker := time.NewTicker(time.Second / hdFPS)
	defer ticker.Stop()
	// Minimal escape-sequence parser: ESC [ (or O) <params> <final>. Arrow keys
	// are A/B/C/D; a ";2" parameter means Shift is held → run.
	esc := 0
	var csi []byte
	for {
		select {
		case b, ok := <-keys:
			if !ok {
				return
			}
			var key string
			switch {
			case esc == 0 && b == 0x1b:
				esc = 1
			case esc == 1:
				if b == '[' || b == 'O' {
					esc, csi = 2, csi[:0]
				} else {
					esc = 0
				}
			case esc == 2:
				if (b >= 'A' && b <= 'Z') || b == '~' {
					esc = 0
					key = csiKey(string(csi), b)
				} else {
					csi = append(csi, b)
				}
			case b == 3: // Ctrl-C always quits
				hud()
				return
			case b == 'q' || b == 'Q': // q closes an open panel, else quits
				if uiPanel != hdPanelNone {
					uiPanel = hdPanelNone
					draw()
				} else {
					hud()
					return
				}
			default:
				key = string(b)
			}
			if uiKey(key) {
				draw()
			} else if km, ok := moveKeyMsg(key); ok {
				next, _ := area.Update(km)
				if t, isTransition := next.(game.Transition); isTransition {
					fw.Reset() // new scene → full repaint
					out.WriteString("\x1b[2J")
					areaID, area, hv = enterHD(ctx, areaID, t.To)
				} else {
					area = next
				}
				draw()
			} else if key == "e" { // pick up an item under the player
				area, _ = area.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
				draw()
			}
		case win = <-winCh:
			cellW, cellH = hdCellSize(win) // the client may report new pixel dims
			fw.CellW, fw.CellH = cellW, cellH
			fw.Reset() // size changed → force a full repaint
			out.WriteString("\x1b[2J")
			draw()
		case <-ticker.C:
			frame++ // advance tile animation and reflect other players' movement
			draw()
		}
	}
}

// enterHD constructs and spawns into an area for HD mode, returning its id, the
// area and its HD viewer. Panel-only areas (the Arcade stub) aren't pixel-
// renderable, so a portal into one falls back to the lobby — HD can never strand
// the player on a blank screen. from is the area being left, so the destination
// can spawn the player beside the right portal.
func enterHD(ctx *game.Ctx, from, id string) (string, game.Area, game.HDViewer) {
	ctx.From = from
	a := game.NewArea(id, ctx)
	hv, ok := a.(game.HDViewer)
	if !ok {
		id = "lobby"
		a = game.NewArea(id, ctx)
		hv = a.(game.HDViewer)
	}
	self, _ := ctx.World.Self(ctx.Name)
	a.Init(&self)
	return id, a, hv
}

// moveKeyMsg converts a parsed key name into a bubbletea KeyMsg, but only for
// the movement keys the areas act on (WASD/arrows, YUBN diagonals, Shift to
// run) — so HD forwards just movement and never trips chat or panel keys.
func moveKeyMsg(key string) (tea.KeyMsg, bool) {
	if _, _, _, ok := game.MoveKey(key); !ok {
		return tea.KeyMsg{}, false
	}
	switch key {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}, true
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}, true
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}, true
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}, true
	case "shift+up":
		return tea.KeyMsg{Type: tea.KeyShiftUp}, true
	case "shift+down":
		return tea.KeyMsg{Type: tea.KeyShiftDown}, true
	case "shift+left":
		return tea.KeyMsg{Type: tea.KeyShiftLeft}, true
	case "shift+right":
		return tea.KeyMsg{Type: tea.KeyShiftRight}, true
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}, true
}

// csiKey turns a parsed arrow escape into a MoveKey name, prefixing "shift+"
// when the Shift modifier (parameter ";2") is present so it runs.
func csiKey(params string, final byte) string {
	dir := arrowKey(final)
	if dir == "" {
		return ""
	}
	if strings.Contains(params, ";2") { // Shift modifier
		return "shift+" + dir
	}
	return dir
}

// arrowKey maps a CSI/SS3 final byte to the movement key names MoveKey expects.
func arrowKey(b byte) string {
	switch b {
	case 'A':
		return "up"
	case 'B':
		return "down"
	case 'C':
		return "right"
	case 'D':
		return "left"
	}
	return ""
}

// hdCellSize derives the terminal's pixel cell size from the PTY window (if the
// client reports pixel dimensions), falling back to Windows Terminal's 10×20.
func hdCellSize(win ssh.Window) (int, int) {
	if win.WidthPixels > 0 && win.HeightPixels > 0 && win.Width > 0 && win.Height > 0 {
		if cw, ch := win.WidthPixels/win.Width, win.HeightPixels/win.Height; cw > 0 && ch > 0 {
			return cw, ch
		}
	}
	return 10, 20
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
