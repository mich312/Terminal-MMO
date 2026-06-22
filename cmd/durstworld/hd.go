package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
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
	hdScale    = 26 // pixels per tile — zoomed out so the 1-tile hero reads as small in a big world
	hdFPS      = 12 // tile-animation / world-reflection rate (the `frame` counter)
	hdRenderHz = 30 // max on-screen redraw rate; input is coalesced to this cadence
	hdMoveHz   = 10 // max movement steps/sec — a held key walks at this even cadence instead of the terminal's jittery key-repeat rate
	hdRefresh  = 48 // frames between full repaints
	hdMaxTile  = 140
	// Cap the rendered buffer so per-frame cost (render + dirty-diff + encode +
	// bandwidth) doesn't scale with window size — the work is ~width·height
	// pixels, and at hdScale=24 a full-screen buffer is huge. This bounds it to
	// ≈hdMaxPxW×hdMaxPxH regardless of terminal size; a bigger window just shows
	// the same world slice at the same cost, not more.
	hdMaxPxW = 1920
	hdMaxPxH = 1200
)

// HD UI panels reachable with single keys (HD has no command line).
const (
	hdPanelNone = iota
	hdPanelChar
	hdPanelCompendium
	hdPanelHelp
	hdPanelWho
	hdPanelMenu
	hdPanelCraft
	hdPanelMachine
	hdPanelStall
	hdPanelTrade
)

// compScrollPage is how many lines a page-up/down jumps in the compendium.
const compScrollPage = 6

// areaFlare is how long an area's name stays emphasized after you enter it,
// before settling to the quiet top-left label.
const areaFlare = 2500 * time.Millisecond

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
	w.MarkPoller(name) // HD repaints from world state each frame; skip the move/tick stream
	st.RecordVisit(name)
	setupAvatar(w, st, name)
	log.Printf("%s connected (HD/%s, TERM=%s)", name, proto, ptyReq.Term)
	defer func() {
		w.Leave(name)
		st.RecordDisconnect(name)
		log.Printf("%s disconnected (HD)", name)
	}()
	ctx := &game.Ctx{World: w, Store: st, Name: name,
		Inventory: st.LoadInventory(name), Hats: st.LoadHats(name),
		FixedGates: st.LoadPersonalGates(name)}
	areaID, area, hv := enterHD(ctx, "", "wilds")
	lastGameTick := time.Now() // wall-clock pacing for real-time areas (game.Ticker)

	cellW, cellH := hdCellSize(ptyReq.Window)
	win := ptyReq.Window

	// A dedicated writer goroutine owns the SSH socket so a slow/congested link
	// never blocks the input+render loop. Output is handed over as whole jobs;
	// control sequences (clears, the goodbye line) are always sent, while frames
	// are gated by frameInFlight below — at most one frame is ever outstanding, so
	// a link that can't keep up simply drops to a lower frame rate instead of
	// stalling movement. Frame jobs signal frameDone when flushed.
	type wjob struct {
		data  []byte
		frame bool
	}
	jobs := make(chan wjob, 64)
	frameDone := make(chan struct{}, 1)
	writerDone := make(chan struct{})
	go func() {
		bw := bufio.NewWriterSize(s, 1<<20)
		for j := range jobs {
			bw.Write(j.data)
			bw.Flush() // may block on a slow socket — only this goroutine waits
			if j.frame {
				select {
				case frameDone <- struct{}{}:
				default:
				}
			}
		}
		close(writerDone)
	}()
	ctrl := func(seq string) { jobs <- wjob{data: []byte(seq)} }
	ctrl("\x1b[2J\x1b[?25l")
	defer func() {
		ctrl("\x1b[?25h\x1b[0m\r\n")
		close(jobs)
		<-writerDone
	}()

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
	var inc game.IncrementalRenderer // re-rasterizes only changed tiles each frame
	var frameImg *image.RGBA         // reused scratch: terrain copy + UI overlays
	frameCopy := func(src *image.RGBA) *image.RGBA {
		if frameImg == nil || frameImg.Bounds() != src.Bounds() {
			frameImg = image.NewRGBA(src.Bounds())
		}
		copy(frameImg.Pix, src.Pix)
		return frameImg
	}
	var (
		frame      int
		sent       int
		framesSent int
		start      = time.Now()
		uiPanel    = hdPanelNone // which on-frame UI panel is open
		uiField    int           // selected field in the character panel
		menuSel    int           // selected row in the Tab menu
		craftSel   int           // selected recipe in the crafting panel
		machineXY  [2]int        // the machine whose panel is open
		awayOut    int           // output a machine made while away (panel banner)
		awayIn     int           // input it consumed while away
		stallXY    [2]int        // the trade stall whose panel is open
		stallSel   int           // selected offer in the stall panel
		stallNew   bool          // composing a new offer at the stall (owner)
		stallDraft game.OfferDraft
		compScroll int           // first visible line in the compendium panel
		tradeSel   int           // selected pack slot in the trade panel
		tradeReq   string        // latest player who asked us to trade (for /accept)
		enteredAt  = time.Now()  // when the current area was entered (for the title flare)
		chatLog    []game.HDLine // recent chat lines
		chatInput  string        // text being typed
		chatActive bool          // chat input has focus
		lastChat   time.Time     // when the log last changed (for idle fade)
	)
	hudDim := color.RGBA{0x9A, 0xA3, 0xAD, 0xFF}
	hudAccent := color.RGBA{0x2E, 0x8B, 0xFF, 0xFF}
	appendChat := func(ln game.HDLine) {
		chatLog = append(chatLog, ln)
		if len(chatLog) > 50 {
			chatLog = chatLog[len(chatLog)-50:]
		}
		lastChat = time.Now()
	}

	frameInFlight := false // a frame is queued/being written; don't compute another
	minimapOpen := false   // the overview map is up (set in render) — a covering overlay
	render := func() {
		if frameInFlight {
			return // backpressure: the writer is still draining the last frame
		}
		cols, rows := win.Width, win.Height
		if cols < 8 || rows < 6 {
			return
		}
		vw := clamp(cols*cellW/hdScale, 8, min(hdMaxTile, hdMaxPxW/hdScale))
		vh := clamp((rows-1)*cellH/hdScale, 8, min(hdMaxTile, hdMaxPxH/hdScale))
		tm, ox, oy := hv.HDView(vw, vh)
		light := game.Light{}
		if l, ok := area.(game.HDLighter); ok {
			light = l.HDLight() // the Wilds' discovery circle
		}
		full := frame%hdRefresh == 0
		// Incremental terrain: only tiles that actually changed are re-rasterized.
		// The result is byte-identical to a full RenderRGBA (see IncrementalRenderer),
		// so the on-wire delta — and what the player sees — is unchanged.
		players := w.PlayersInArea(areaID)
		if h, ok := area.(game.AvatarHider); ok && h.HideAvatars() {
			players = nil // board games (Pong, Tetris…) draw no avatars over the board
		}
		base := inc.Render(tm, players, name, frame, light, ox, oy, hdScale, style, full, w.CreaturesInArea(areaID)...)
		// Composite UI onto a copy so overlays never bleed into the cached terrain.
		img := frameCopy(base)
		// A raycaster area (Doom) repaints the whole frame in first person.
		if fr, ok := area.(game.HDFramer); ok {
			fr.HDFrame(img)
		}
		game.OverlayWalkable(img, tm, hdScale)  // debug: tint blocked tiles (toggle with F2)
		game.DrawPortalLabels(img, tm, hdScale) // float each gate's destination name above it

		// Draw an area's on-screen text (a presentation slide) into the frame —
		// HD has no glyph layer, so slides are rasterized straight onto the image.
		if ov, ok := area.(game.HDOverlayer); ok {
			if src, footer, show := ov.HDSlide(); show {
				pixel.DrawSlidePanel(img, src, footer)
			}
		}

		// HD UI overlays. The world fills the screen; the chrome is minimal and
		// mostly contextual: a quiet area title (flaring on entry) top-left, the
		// persistent mini-legend top-right, a transient pickup toast, and — only
		// when something's actually usable — a button prompt near the bottom.
		emphasis := 0.0
		if d := time.Since(enteredAt); d < areaFlare {
			emphasis = 1 - float64(d)/float64(areaFlare)
		}
		game.DrawAreaTitle(img, area.Name(), emphasis)
		if cl, ok := area.(game.ClaimLabeler); ok {
			if label, show := cl.ClaimLabel(); show {
				game.DrawClaimBanner(img, label)
			}
		}
		game.DrawTopLegend(img)
		// While building, the palette (with its own footer) is the guide; skip the
		// centered action prompt so the two don't fight for the bottom.
		building := false
		if bv, ok := area.(game.BuildViewer); ok {
			if _, _, _, show := bv.BuildPanel(); show {
				building = true
			}
		}
		if prompt, show := actionPrompt(area); show && !building {
			game.DrawActionPrompt(img, prompt)
		}
		if tz, ok := area.(game.Toaster); ok {
			if msg, show := tz.Toast(); show {
				game.DrawToast(img, msg)
			}
		}
		// The build palette (a left-anchored, non-modal HUD while building).
		if bv, ok := area.(game.BuildViewer); ok {
			if sel, footer, warn, show := bv.BuildPanel(); show {
				game.DrawBuildPanel(img, ctx, sel, footer, warn)
			}
		}
		// The coarse overview map (toggled with 'm'), drawn as colored blocks.
		minimapOpen = false
		if mm, ok := area.(game.HDMinimapper); ok {
			if title, rows, show := mm.HDMinimap(); show {
				game.DrawMinimapPanel(img, title, rows)
				minimapOpen = true
			}
		}
		// Chat log + input, fading out when idle so the scene stays clear.
		var shownChat []game.HDLine
		if chatActive || time.Since(lastChat) < 12*time.Second {
			shownChat = chatLog
		}
		game.DrawChat(img, shownChat, chatActive, chatInput)

		switch uiPanel {
		case hdPanelChar:
			game.DrawCharPanel(img, ctx, uiField)
		case hdPanelCompendium:
			game.DrawCompendiumPanel(img, ctx, &compScroll)
		case hdPanelHelp:
			game.DrawHelpPanel(img, ctx)
		case hdPanelWho:
			game.DrawWhoPanel(img, ctx)
		case hdPanelMenu:
			game.DrawMenuPanel(img, menuSel)
		case hdPanelCraft:
			game.DrawCraftPanel(img, ctx, craftSel)
		case hdPanelMachine:
			game.DrawMachinePanel(img, ctx, machineXY[0], machineXY[1], awayOut, awayIn)
		case hdPanelStall:
			game.DrawStallPanel(img, ctx, stallXY[0], stallXY[1], stallSel, stallNew, stallDraft)
		case hdPanelTrade:
			if v, ok := game.TradeViewFor(ctx, tradeSel); ok {
				game.DrawTradePanel(img, v)
			}
		}

		var buf bytes.Buffer
		n := fw.WriteFrame(&buf, img, full)
		if buf.Len() == 0 {
			return // nothing changed — no job, stay idle
		}
		sent += n
		framesSent++
		frameInFlight = true
		jobs <- wjob{data: buf.Bytes(), frame: true} // room guaranteed: no frame in flight
	}

	// Input handlers mark the frame dirty rather than rendering inline, so a slow
	// encode never blocks key reads and a burst of movement keys collapses into a
	// single render. The ticker below is the only thing that actually draws.
	dirty := false
	draw := func() { dirty = true }

	hud := func() {
		dur := time.Since(start).Seconds()
		if dur <= 0 {
			dur = 1
		}
		ctrl(fmt.Sprintf("\x1b[?25h\x1b[0m\r\nHD session over: %d frames, %.0f KB sent, %.1f KB/s avg over %.0fs\r\n",
			framesSent, float64(sent)/1024, float64(sent)/1024/dur, dur))
	}

	// uiKey handles HD interface keys. Direct keys ('c' character, 'i' inventory,
	// '?' help) toggle their panels from anywhere; Tab opens the menu hub. The
	// character editor and the menu are interactive (arrows navigate); help, who
	// and inventory are passive (any key dismisses them). Returns whether the key
	// was consumed by the UI.
	toggle := func(p int) bool {
		if uiPanel == p {
			uiPanel = hdPanelNone
		} else {
			uiPanel, uiField = p, 0
		}
		return true
	}
	openMenuSel := func() {
		switch menuSel {
		case 0:
			uiPanel, compScroll = hdPanelCompendium, 0
		case 1:
			uiPanel, craftSel = hdPanelCraft, 0
		case 2:
			uiPanel, uiField = hdPanelChar, 0
		case 3:
			uiPanel = hdPanelWho
		case 4:
			uiPanel = hdPanelHelp
		}
	}
	uiKey := func(key string) bool {
		switch key {
		case "c":
			return toggle(hdPanelChar)
		case "i":
			compScroll = 0
			return toggle(hdPanelCompendium)
		case "k":
			if uiPanel == hdPanelCraft {
				uiPanel = hdPanelNone
			} else {
				uiPanel, craftSel = hdPanelCraft, 0
			}
			return true
		case "?":
			return toggle(hdPanelHelp)
		case "tab":
			if uiPanel == hdPanelMenu {
				uiPanel = hdPanelNone
			} else {
				uiPanel, menuSel = hdPanelMenu, 0
			}
			return true
		}
		if uiPanel == hdPanelNone {
			return false
		}
		if uiPanel == hdPanelMenu {
			switch key {
			case "up":
				menuSel = (menuSel + len(game.MenuEntries()) - 1) % len(game.MenuEntries())
			case "down":
				menuSel = (menuSel + 1) % len(game.MenuEntries())
			case "\r", "\n":
				openMenuSel()
			case "q":
				uiPanel = hdPanelNone
			}
			return true
		}
		// Crafting is interactive: arrows pick a recipe, e crafts the selection.
		if uiPanel == hdPanelCraft {
			n := len(game.Recipes)
			switch key {
			case "up":
				craftSel = (craftSel + n - 1) % n
			case "down":
				craftSel = (craftSel + 1) % n
			case "e", "\r", "\n":
				if craftSel >= 0 && craftSel < n {
					game.Craft(ctx, game.Recipes[craftSel])
				}
			case "q":
				uiPanel = hdPanelNone
			}
			return true
		}
		// Machine panel: e collects, f refuels, q closes.
		if uiPanel == hdPanelMachine {
			switch key {
			case "e", "\r", "\n":
				game.CollectMachine(ctx, machineXY[0], machineXY[1])
				awayOut, awayIn = 0, 0
			case "f":
				game.RefuelMachine(ctx, machineXY[0], machineXY[1])
			case "q":
				uiPanel = hdPanelNone
			}
			return true
		}
		// Trade stall: arrows pick an offer, e buys, owner f collects/x removes/n
		// composes a new offer; q closes. The composer (owner only) edits the offer
		// terms in-panel — the UI twin of /sell, since HD has no command line.
		if uiPanel == hdPanelStall {
			st, ok := game.StallSnapshot(ctx, stallXY[0], stallXY[1])
			if !ok {
				uiPanel, stallNew = hdPanelNone, false
				return true
			}
			owner := game.StallOwner(ctx, stallXY[0], stallXY[1])
			if stallNew && owner {
				switch key {
				case "up":
					stallDraft.Field = (stallDraft.Field + game.OfferFields - 1) % game.OfferFields
				case "down":
					stallDraft.Field = (stallDraft.Field + 1) % game.OfferFields
				case "left":
					game.CycleOfferField(ctx, &stallDraft, -1)
				case "right":
					game.CycleOfferField(ctx, &stallDraft, +1)
				case "e", "\r", "\n":
					if game.PostDraft(ctx, stallXY[0], stallXY[1], stallDraft) > 0 {
						stallNew = false
						stallSel = len(st.Offers) // the new last offer
					}
				case "q":
					stallNew = false
				}
				return true
			}
			n := len(st.Offers)
			switch key {
			case "up":
				if n > 0 {
					stallSel = (stallSel + n - 1) % n
				}
			case "down":
				if n > 0 {
					stallSel = (stallSel + 1) % n
				}
			case "e", "\r", "\n":
				game.AcceptOffer(ctx, stallXY[0], stallXY[1], stallSel)
			case "n":
				if owner {
					if d, ok := game.NewOfferDraft(ctx); ok {
						stallDraft, stallNew = d, true
					}
				}
			case "f":
				game.CollectTill(ctx, stallXY[0], stallXY[1])
			case "x":
				if game.RemoveOffer(ctx, stallXY[0], stallXY[1], stallSel) && stallSel > 0 {
					stallSel--
				}
			case "q":
				uiPanel = hdPanelNone
			}
			return true
		}
		// The trade table is driven by keys: pick from your pack, stage an offer,
		// toggle ready, or cancel. It reads live world state, so no local copy.
		if uiPanel == hdPanelTrade {
			n := game.TradePackSize(ctx)
			switch key {
			case "left":
				if n > 0 {
					tradeSel = (tradeSel + n - 1) % n
				}
			case "right":
				if n > 0 {
					tradeSel = (tradeSel + 1) % n
				}
			case "+", "=":
				game.OfferSlot(ctx, tradeSel, +1)
			case "-", "_":
				game.OfferSlot(ctx, tradeSel, -1)
			case "r", "\r", "\n":
				snap, _ := w.TradeSnapshot(name)
				w.SetReady(name, !snap.YouReady)
			}
			return true
		}
		// The compendium scrolls (arrows / page keys); any other key closes it.
		if uiPanel == hdPanelCompendium {
			switch key {
			case "up":
				compScroll--
			case "down":
				compScroll++
			case "pgup", "b":
				compScroll -= compScrollPage
			case "pgdown", " ", "f":
				compScroll += compScrollPage
			default:
				uiPanel = hdPanelNone
			}
			if compScroll < 0 {
				compScroll = 0
			}
			// The upper bound depends on the frame size, so DrawCompendiumPanel
			// clamps compScroll against the live layout each render.
			return true
		}
		// Passive panels: any key closes them (and is otherwise swallowed).
		if uiPanel == hdPanelHelp || uiPanel == hdPanelWho {
			uiPanel = hdPanelNone
			return true
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

	// sendChat handles a submitted chat line: plain text is proximity chat, a
	// leading "/" runs the handful of world commands HD supports (the panels
	// cover the rest). The sender sees their own chat/emote echoed via the world
	// event stream; whispers are echoed locally here since the world doesn't.
	sendChat := func(text string) {
		if !strings.HasPrefix(text, "/") {
			w.Chat(name, text)
			return
		}
		f := strings.Fields(text[1:])
		if len(f) == 0 {
			return
		}
		switch strings.ToLower(f[0]) {
		case "help", "h":
			uiPanel = hdPanelHelp
		case "who":
			uiPanel = hdPanelWho
		case "where":
			if self, ok := w.Self(name); ok {
				appendChat(game.HDLine{
					Text: fmt.Sprintf("%s - (%d, %d)", game.DisplayName(areaID), self.X, self.Y),
					Col:  hudDim})
			}
		case "me":
			if len(f) > 1 {
				w.Emote(name, strings.Join(f[1:], " "))
			}
		case "w", "tell", "msg", "whisper":
			if len(f) > 2 {
				to, msg := f[1], strings.Join(f[2:], " ")
				if w.Whisper(name, to, msg) {
					appendChat(game.HDLine{Text: "-> " + to + ": " + msg, Col: hudAccent})
				} else {
					appendChat(game.HDLine{Text: to + " is not online", Col: hudDim})
				}
			}
		case "goto", "go":
			if len(f) <= 1 { // no argument → show the list of places
				appendChat(game.HDLine{Text: "go to — /goto <name>:", Col: hudAccent})
				for _, ln := range game.GotoListLines() {
					appendChat(game.HDLine{Text: ln, Col: hudDim})
				}
			} else if dest := strings.ToLower(f[1]); dest != areaID && game.AreaRegistered(dest) {
				fw.Reset()
				inc.Reset() // new area → discard the cached terrain
				ctrl("\x1b[2J")
				areaID, area, hv = enterHD(ctx, areaID, dest)
				enteredAt, lastGameTick = time.Now(), time.Now()
			} else {
				appendChat(game.HDLine{Text: "no such area: " + f[1] + " — /goto for the list", Col: hudDim})
			}
		case "trade", "tr":
			if len(f) < 2 {
				appendChat(game.HDLine{Text: "usage: /trade <player> - stand next to them", Col: hudDim})
				break
			}
			target, ok := "", false
			la := strings.ToLower(f[1])
			for _, p := range w.PlayersInArea(areaID) {
				if p.Name != name && strings.ToLower(p.Name) == la {
					target, ok = p.Name, true
				}
			}
			if !ok {
				appendChat(game.HDLine{Text: "no other player here named " + f[1], Col: hudDim})
			} else if err := w.RequestTrade(name, target); err != nil {
				appendChat(game.HDLine{Text: err.Error(), Col: hudDim})
			} else {
				appendChat(game.HDLine{Text: "asked " + target + " to trade", Col: hudAccent})
			}
		case "accept":
			if tradeReq == "" {
				appendChat(game.HDLine{Text: "no trade request to accept", Col: hudDim})
				break
			}
			from := tradeReq
			tradeReq = ""
			if err := w.AcceptTrade(name, from); err != nil {
				appendChat(game.HDLine{Text: err.Error(), Col: hudDim})
			}
		case "decline":
			if tradeReq != "" {
				w.DeclineTrade(name, tradeReq)
				tradeReq = ""
				appendChat(game.HDLine{Text: "declined the trade", Col: hudDim})
			}
		default:
			appendChat(game.HDLine{Text: "press ? for help - commands: /me /w /who /goto /trade", Col: hudDim})
		}
	}

	// Movement is throttled to hdMoveHz. A held key repeats far faster than that
	// at the terminal's key-repeat rate (and SSH has no key-up event), which made
	// walking lurch and overshoot: the first repeat steps, the OS pauses, then a
	// burst of repeats sprints the avatar past where you released. We take the
	// first step immediately (so a tap stays responsive) and coalesce further
	// repeats into one even step per tick — newest direction wins — until the key
	// stops repeating. This also caps the camera-pan full-frame rate over SSH.
	moveInterval := time.Second / hdMoveHz
	// A held key keeps walking while repeats keep arriving; if none has for
	// heldTimeout the key is treated as released. It must comfortably exceed the
	// terminal's repeat gap (and any frame-encode stall in the loop) so a genuine
	// hold doesn't stutter, yet be short enough that release feels immediate.
	const heldTimeout = 250 * time.Millisecond
	var (
		pendingMove tea.KeyMsg
		havePending bool
		lastStep    time.Time
	)
	// sendArea hands a message to the active area and, if it asks to leave
	// (returns a Transition), swaps in the destination — the single place key and
	// move handlers funnel area updates so none of them can strand the player on
	// the Transition sentinel (which is not itself renderable).
	sendArea := func(msg tea.Msg) {
		next, _ := area.Update(msg)
		if t, isTransition := next.(game.Transition); isTransition {
			fw.Reset()  // new scene → full repaint
			inc.Reset() // new area → discard the cached terrain
			ctrl("\x1b[2J")
			areaID, area, hv = enterHD(ctx, areaID, t.To)
			enteredAt, lastGameTick = time.Now(), time.Now()
		} else {
			area = next
		}
	}
	applyMove := func(km tea.KeyMsg) {
		sendArea(km)
		lastStep = time.Now()
		draw()
	}

	render() // first paint; thereafter the ticker renders when something is dirty
	ticker := time.NewTicker(time.Second / hdRenderHz)
	defer ticker.Stop()
	// Minimal escape-sequence parser: ESC [ (or O) <params> <final>. Arrow keys
	// are A/B/C/D; a ";2" parameter means Shift is held → run.
	esc := 0
	var csi []byte
	var lastKey time.Time // when a movement key last arrived, for release detection

	// handleKey processes one input byte, returning true if the session should
	// quit. Movement keys only record the held direction (stepped, throttled, by
	// the move ticker) so a burst of buffered repeats collapses to one intent
	// rather than marching the avatar on after the key is released.
	handleKey := func(b byte) (quit bool) {
		if chatActive { // typing a chat line: capture the byte, skip game keys
			switch {
			case b == 3: // Ctrl-C still quits
				hud()
				return true
			case b == '\r' || b == '\n':
				text := strings.TrimSpace(chatInput)
				chatActive, chatInput = false, ""
				if text != "" {
					sendChat(text)
				}
				draw()
			case b == 0x7f || b == 0x08: // backspace
				if n := len(chatInput); n > 0 {
					chatInput = chatInput[:n-1]
				}
				draw()
			case b == 0x1b: // esc cancels
				chatActive, chatInput = false, ""
				draw()
			case b >= 0x20 && b < 0x7f:
				chatInput += string(b)
				draw()
			}
			return false
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
			return true
		case b == 'q' || b == 'Q': // q closes an open panel, else quits
			if uiPanel == hdPanelTrade {
				w.CancelTrade(name) // the cancel event closes the table for both
				draw()
			} else if uiPanel != hdPanelNone {
				uiPanel = hdPanelNone
				draw()
			} else {
				hud()
				return true
			}
		case b == '\t': // Tab toggles the who's-online panel
			key = "tab"
		default:
			key = string(b)
		}
		if uiKey(key) {
			draw()
		} else if km, ok := moveKeyMsg(key); ok {
			// Step now if a tick has elapsed, else hold the latest direction for the
			// move ticker. lastKey marks the input fresh so the ticker keeps walking
			// while the key repeats and stops within a tick once it is released.
			pendingMove, lastKey = km, time.Now()
			if time.Since(lastStep) >= moveInterval {
				applyMove(km)
				havePending = false
			} else {
				havePending = true
			}
		} else if key == "e" { // pick up an item, or open a station beside you
			sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
			if ctx.UseStation != nil { // the area asked to open a panel
				xy := *ctx.UseStation
				ctx.UseStation = nil
				pl, _ := w.PlacementAt(xy[0], xy[1])
				switch {
				case game.IsStall(pl.Kind):
					stallXY, stallSel, stallNew, uiPanel = xy, 0, false, hdPanelStall
				case game.IsWorkbench(pl.Kind):
					craftSel, uiPanel = 0, hdPanelCraft
				default:
					machineXY = xy
					_, awayOut, awayIn, _ = game.OpenMachine(ctx, xy[0], xy[1])
					uiPanel = hdPanelMachine
				}
			}
			draw()
		} else if key == "b" || key == "r" || key == "[" || key == "]" || key == "x" {
			// build mode: 'b' toggles, 'r'/brackets cycle the placeable, 'x' demolishes
			// under the ghost. In a minigame these double as game keys (restart/leave),
			// which may transition — so funnel through sendArea.
			sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			draw()
		} else if key == "f" { // hunt an adjacent animal
			sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
			draw()
		} else if key == "t" { // tame an adjacent animal with bait
			sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
			draw()
		} else if key == "m" { // toggle the area overview map
			sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
			draw()
		} else if key == "f2" { // toggle the walkability debug overlay
			game.DebugWalkable = !game.DebugWalkable
			draw()
		} else if key == "\r" || key == "\n" { // open chat
			chatActive, chatInput = true, ""
			draw()
		} else if key == "/" { // open chat pre-filled for a command
			chatActive, chatInput = true, "/"
			draw()
		}
		return false
	}

	for {
		select {
		case b, ok := <-keys:
			if !ok {
				return
			}
			if handleKey(b) {
				return
			}
			// Drain the rest of the buffered burst right now, coalescing it. While
			// the loop is busy encoding a frame, held-key repeats pile up in the
			// channel (and the kernel SSH buffer); processing them one-per-select
			// would let that backlog keep driving movement long after release. Empty
			// it in one go so a release stops the avatar within a tick.
		drain:
			for {
				select {
				case b2, ok2 := <-keys:
					if !ok2 {
						return
					}
					if handleKey(b2) {
						return
					}
				default:
					break drain
				}
			}
		case ev, ok := <-events:
			if !ok {
				events = nil // world closed this stream; stop selecting on it
				continue
			}
			if ln, show := game.HDChatLine(ev, name); show {
				appendChat(ln)
				draw()
			}
			// Presentation needs slide/deck events to rebuild; the Wilds needs
			// chat to catch a gate riddle answered aloud. Other areas poll the
			// world each frame, so they don't need the rest.
			if ev.Type == world.EventSlide || ev.Type == world.EventDeck || ev.Type == world.EventChat {
				area, _ = area.Update(game.WorldEventMsg(ev))
				draw()
			}
			if ev.Type == world.EventTrade {
				switch ev.Detail {
				case world.TradeRequest:
					tradeReq = ev.Player
					appendChat(game.HDLine{Text: ev.Player + " wants to trade - /accept or /decline", Col: hudAccent})
				case world.TradeOpen:
					uiPanel, tradeSel, tradeReq = hdPanelTrade, 0, ""
				case world.TradeDone:
					if s, ok := game.ApplyCompletedTrade(ctx); ok {
						appendChat(game.HDLine{Text: s, Col: hudAccent})
					}
					uiPanel = hdPanelNone
				case world.TradeCancel:
					if uiPanel == hdPanelTrade {
						uiPanel = hdPanelNone
					}
					appendChat(game.HDLine{Text: "trade cancelled", Col: hudDim})
				case world.TradeDeclined:
					appendChat(game.HDLine{Text: ev.Player + " declined to trade", Col: hudDim})
				}
				draw()
			}
		case win = <-winCh:
			cellW, cellH = hdCellSize(win) // the client may report new pixel dims
			fw.CellW, fw.CellH = cellW, cellH
			fw.Reset() // size changed → force a full repaint
			ctrl("\x1b[2J")
			draw()
		case <-frameDone:
			// The last frame finished writing; paint any pending change now, so a
			// slow link self-paces — the next frame goes out as soon as it can.
			frameInFlight = false
			if dirty {
				render()
				dirty = false
			}
		case <-ticker.C:
			// Continue a held key one step per move tick, but only while it is still
			// being pressed: once released the repeat stream stops, lastKey goes
			// stale, and movement halts within a tick — so a key-up never strands the
			// avatar mid-stride even though the terminal sends no key-up event.
			if havePending && time.Since(lastKey) < heldTimeout && time.Since(lastStep) >= moveInterval {
				applyMove(pendingMove)
				havePending = false
			}
			// Drive a real-time area (Snake, …) off the wall clock — the HD client
			// never forwards a clock to areas otherwise. A tick may transition.
			if tk, ok := area.(game.Ticker); ok && time.Since(lastGameTick) >= tk.TickInterval() {
				lastGameTick = time.Now()
				next := tk.GameTick()
				if t, isT := next.(game.Transition); isT {
					fw.Reset()
					inc.Reset()
					ctrl("\x1b[2J")
					areaID, area, hv = enterHD(ctx, areaID, t.To)
					enteredAt = time.Now()
				} else {
					area = next
				}
				dirty = true
			}
			// Advance the animation/world-reflection counter on wall-clock time so
			// it stays at hdFPS regardless of the (higher) render cadence. While a
			// full-screen panel or the overview map is up, the scene is all but
			// hidden, so don't repaint just because the world animated a frame: a
			// translucent panel over animated terrain otherwise re-diffs (and often
			// fully repaints) every frame for a shimmer no one can see. The counter
			// still advances, so the world catches up the moment the menu is closed
			// or the player acts (those paths set dirty explicitly).
			if nf := int(time.Since(start) / (time.Second / hdFPS)); nf != frame {
				frame = nf
				if uiPanel == hdPanelNone && !minimapOpen {
					dirty = true
				}
			}
			// Render only when the writer is free; if it's still draining, leave the
			// frame dirty and frameDone will pick it up (dropping intermediate ones).
			if dirty && !frameInFlight {
				render()
				dirty = false
			}
		}
	}
}

// actionPrompt returns the contextual action available where the player stands,
// preferring an area's Prompter; areas that only implement Hinter treat a
// non-empty hint as the prompt. The bool is false when nothing is actionable,
// so the HD client leaves the bottom of the screen clear.
func actionPrompt(a game.Area) (string, bool) {
	if p, ok := a.(game.Prompter); ok {
		return p.Prompt()
	}
	if h, ok := a.(game.Hinter); ok {
		if t := h.Hint(); t != "" {
			return t, true
		}
	}
	return "", false
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
