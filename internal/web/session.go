package web

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/coder/websocket"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// Session cadences. These mirror the HD client's constants and for the same
// reasons — the numbers were tuned against how the world feels, not against any
// property of a terminal, so they carry over unchanged.
const (
	sceneHz = 15 // scene frames per second (each one a delta; skipped when idle)
	// The browser paces its own walking at 10 steps/sec, using the key-up events
	// a terminal never gets. This floor is only a guard against a client that
	// doesn't — so it sits comfortably above the honest cadence: pinning it to
	// exactly 10Hz would make ordinary jitter drop every other real step.
	moveHz = 16 // movement steps per second the server will accept
	tickHz = 12 // world animation counter, for portal pulse and tile anims
	// viewport bounds. A browser window can be enormous; the tile window is
	// capped so one client can't ask the server to build a 200×200 scene.
	minTiles = 8
	maxTileW = 64
	maxTileH = 48
	// A frame is dropped rather than queued when the socket is behind, so a slow
	// connection degrades to a lower frame rate instead of accumulating lag.
	outBuffer = 8
)

// session is one browser player: a websocket, the game area they're standing
// in, and the delta state for their scene.
//
// It is deliberately the same shape as cmd/durstworld/hd.go's runHD — join the
// world, build an area, funnel keys into it, watch for transitions, pump world
// events — because that loop is the contract every client honors. Reimplementing
// it differently is how two clients start disagreeing about the world.
type session struct {
	conn *websocket.Conn
	ctx  context.Context

	w     *world.World
	st    store.Store
	gctx  *game.Ctx
	name  string
	msgs  chan ClientMsg
	out   chan []byte
	evs   <-chan world.Event
	scene *sceneState

	areaID string
	area   game.Area
	view   game.HDViewer

	vw, vh    int
	frame     int
	enteredAt time.Time
	lastStep  time.Time
	lastTick  time.Time
	started   time.Time

	panel    string // the panel the client has open ("" = none)
	panelSel int
	stallXY  [2]int
	machXY   [2]int
	tradeReq string
	lastSent []byte

	fx       []FX      // combat motions since the last frame, riding the next scene
	lastFace time.Time // throttle for face: turns (the action camera sends them freely)
}

// runSession owns one browser player from connect to disconnect.
func runSession(ctx context.Context, conn *websocket.Conn, w *world.World, st store.Store, desired string) {
	name, events := w.Join(desired)
	// Like HD, the browser repaints from world state each frame rather than
	// reacting to every move event, so opt out of the move/tick stream.
	w.MarkPoller(name)
	st.RecordVisit(name)
	game.SetupAvatar(w, st, name)
	log.Printf("%s connected (web)", name)
	defer func() {
		w.Leave(name)
		st.RecordDisconnect(name)
		log.Printf("%s disconnected (web)", name)
	}()

	s := &session{
		conn:  conn,
		ctx:   ctx,
		w:     w,
		st:    st,
		name:  name,
		evs:   events,
		msgs:  make(chan ClientMsg, 64),
		out:   make(chan []byte, outBuffer),
		scene: newSceneState(),
		vw:    32,
		vh:    24,
		gctx: &game.Ctx{
			World: w, Store: st, Name: name,
			Inventory:  st.LoadInventory(name),
			Hats:       st.LoadHats(name),
			Compendium: st.LoadCompendium(name),
			FixedGates: st.LoadPersonalGates(name),
		},
		started:   time.Now(),
		enteredAt: time.Now(),
		lastTick:  time.Now(),
	}

	writerDone := make(chan struct{})
	go s.writeLoop(writerDone)
	go s.readLoop()

	s.enter("", "wilds")
	ids, shapes := shapeNames()
	s.send(Hello{
		T: MsgHello, Version: ProtocolVersion, Name: name,
		MaxW: maxTileW, MaxH: maxTileH,
		Shapes: shapes, Props: ids, Texes: texIDs(),
		Weapons: game.HeldWeaponShapes(),
	})
	s.render(true)

	ticker := time.NewTicker(time.Second / sceneHz)
	defer ticker.Stop()
	defer func() {
		close(s.out)
		<-writerDone
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case m, ok := <-s.msgs:
			if !ok {
				return
			}
			s.handleClient(m)
			// Drain anything else already queued before rendering, so a burst of
			// input collapses into one frame the way the HD loop coalesces keys.
		drain:
			for {
				select {
				case m2, ok2 := <-s.msgs:
					if !ok2 {
						return
					}
					s.handleClient(m2)
				default:
					break drain
				}
			}

		case ev, ok := <-s.evs:
			if !ok {
				s.evs = nil
				continue
			}
			s.handleWorldEvent(ev)

		case <-ticker.C:
			s.tick()
			s.render(false)
		}
	}
}

// readLoop decodes client messages onto the session channel. A malformed
// message closes the session rather than being guessed at.
func (s *session) readLoop() {
	defer close(s.msgs)
	for {
		_, data, err := s.conn.Read(s.ctx)
		if err != nil {
			return
		}
		var m ClientMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return
		}
		select {
		case s.msgs <- m:
		case <-s.ctx.Done():
			return
		}
	}
}

// writeLoop owns the socket, so a congested link never blocks the game loop.
func (s *session) writeLoop(done chan struct{}) {
	defer close(done)
	for data := range s.out {
		wctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		err := s.conn.Write(wctx, websocket.MessageText, data)
		cancel()
		if err != nil {
			return
		}
	}
}

// send queues a message. When the writer is behind, the message is dropped
// rather than queued: every scene is a full-enough picture that skipping one is
// invisible, whereas queueing them turns latency into permanent lag.
func (s *session) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case s.out <- data:
	default: // writer is behind — drop this frame
	}
}

// sendNow queues a message that must not be dropped (chat, panels, goodbye).
// These are rare and carry state the client can't reconstruct from a later
// frame, so they wait for room instead of vanishing.
func (s *session) sendNow(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case s.out <- data:
	case <-s.ctx.Done():
	case <-time.After(5 * time.Second):
	}
}

// enter constructs and spawns into an area, exactly as enterHD does — including
// the fallback that keeps a player off a scene the client can't draw.
func (s *session) enter(from, id string) {
	s.gctx.From = from
	a := game.NewArea(id, s.gctx)
	hv, ok := a.(game.HDViewer)
	if !ok {
		// Panel-only areas (the Arcade stub) have no tile window to render.
		// Falling back to the lobby means a portal can never strand a browser
		// player on an empty scene.
		id = "lobby"
		a = game.NewArea(id, s.gctx)
		hv, _ = a.(game.HDViewer)
	}
	self, _ := s.w.Self(s.name)
	a.Init(&self)
	s.areaID, s.area, s.view = id, a, hv
	s.enteredAt = time.Now()
	s.lastTick = time.Now()
	s.scene.reset(id)
}

// sendArea hands a message to the active area and follows a transition, the
// single funnel that stops a player being left on the Transition sentinel.
func (s *session) sendArea(msg tea.Msg) {
	next, _ := s.area.Update(msg)
	if t, isTransition := next.(game.Transition); isTransition {
		s.enter(s.areaID, t.To)
		return
	}
	s.area = next
}

// tick advances the world animation counter and drives real-time areas off the
// wall clock, the way both terminal clients do — a Ticker area (Snake) has no
// clock of its own.
func (s *session) tick() {
	if nf := int(time.Since(s.started) / (time.Second / tickHz)); nf != s.frame {
		s.frame = nf
	}
	if tk, ok := s.area.(game.Ticker); ok && time.Since(s.lastTick) >= tk.TickInterval() {
		s.lastTick = time.Now()
		next := tk.GameTick()
		if t, isT := next.(game.Transition); isT {
			s.enter(s.areaID, t.To)
		} else {
			s.area = next
		}
	}
}

// render builds a frame and sends it unless it would say exactly what the last
// one did. Skipping identical frames is what makes an idle player cost nothing:
// standing still in an empty area sends no traffic at all.
func (s *session) render(force bool) {
	if s.view == nil {
		return
	}
	flare := 0.0
	if d := time.Since(s.enteredAt); d < 2500*time.Millisecond {
		flare = 1 - float64(d)/float64(2500*time.Millisecond)
	}
	prompt, _ := actionPrompt(s.area)
	building := false
	if bv, ok := s.area.(game.BuildViewer); ok {
		if _, _, _, show := bv.BuildPanel(); show {
			building = true
			prompt = ""
		}
	}
	sc := s.scene.Build(sceneInput{
		Area: s.area, View: s.view, World: s.w, Name: s.name, AreaID: s.areaID,
		VW: s.vw, VH: s.vh, Frame: s.frame, Flare: flare, Now: time.Now(),
		Prompt: prompt, Building: building,
		Creatures: s.w.CreaturesInArea(s.areaID),
		FX:        s.fx,
	})
	s.fx = nil // motions ride exactly one frame

	// Compare with the frame counter and flare zeroed: those advance on their
	// own and would make every frame look different, defeating the check.
	frame, flareV := sc.Frame, sc.Flare
	sc.Frame, sc.Flare = 0, 0
	sig, err := json.Marshal(sc)
	if err != nil {
		return
	}
	same := string(sig) == string(s.lastSent)
	s.lastSent = sig
	if same && !force && flareV == 0 {
		return
	}
	sc.Frame, sc.Flare = frame, flareV
	s.send(sc)
}

// handleClient dispatches one message from the browser.
func (s *session) handleClient(m ClientMsg) {
	switch m.T {
	case CmdKey:
		s.handleKey(m.Key)
	case CmdChat:
		if text := strings.TrimSpace(m.Text); text != "" {
			s.handleChat(text)
		}
	case CmdPanel:
		s.handlePanel(m)
	case CmdResize:
		s.resize(m.W, m.H)
	case CmdPing:
		// keepalive; nothing to do
	}
}

// resize adjusts the tile window to the browser's viewport, clamped so one
// client can't ask the server for an unbounded scene.
func (s *session) resize(w, h int) {
	s.vw = clamp(w, minTiles, maxTileW)
	s.vh = clamp(h, minTiles, maxTileH)
	s.render(true)
}

// handleKey routes a game key into the active area. It mirrors the HD client's
// dispatch, minus everything HD only needs because it has no DOM: panel keys
// and chat arrive as their own messages instead.
func (s *session) handleKey(key string) {
	if km, ok := moveKeyMsg(key); ok {
		// Note this also carries the roguelike diagonals y/u/b/n, and 'b' is
		// build mode's toggle in the Wilds. That isn't a clash: the key is
		// forwarded to the area either way, and the area decides what it means
		// where you're standing — exactly as it does for the HD client.
		//
		// Server-side movement cadence. The browser has real key-up events, so
		// it paces its own repeat — but the rate is enforced here too, since the
		// client is the thing we don't control.
		if time.Since(s.lastStep) < time.Second/moveHz {
			return
		}
		s.lastStep = time.Now()
		s.sendArea(km)
		s.render(false)
		return
	}
	switch key {
	case "e":
		s.sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		// The area may have asked to open a station panel where the player
		// stands — the same one-shot handshake the HD client uses.
		if s.gctx.UseStation != nil {
			xy := *s.gctx.UseStation
			s.gctx.UseStation = nil
			pl, _ := s.w.PlacementAt(xy[0], xy[1])
			switch {
			case game.IsStall(pl.Kind):
				s.stallXY, s.panelSel = xy, 0
				s.openPanel("stall")
			case game.IsWorkbench(pl.Kind):
				s.panelSel = 0
				s.openPanel("craft")
			default:
				s.machXY = xy
				game.OpenMachine(s.gctx, xy[0], xy[1])
				s.openPanel("machine")
			}
		}
	case "b", "r", "[", "]", "x", "f", "F", "t", "m", "n", "p":
		// Build mode, minigame restart/leave, striking (f fast, F strong),
		// taming, the overview map and slide navigation all go straight to the
		// area, which owns what they mean where you're standing.
		s.sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	case "guard:1", "guard:0":
		// Guard is world state, not an area action: raise/lower goes straight to
		// the referee, which stamps the parry clock server-side.
		s.w.SetGuard(s.name, key == "guard:1")
	default:
		// The parameterized combat commands, validated here so a browser can no
		// more inject an arbitrary key than an SSH client can.
		if n, ok := combatDir(key, "face:"); ok {
			// The action camera turns freely; the world only needs the quantized
			// facing, throttled so mouse-look doesn't become a broadcast storm.
			if time.Since(s.lastFace) >= 40*time.Millisecond {
				s.lastFace = time.Now()
				s.w.SetFacing(s.name, world.Dir(n))
			}
			return
		}
		if _, ok := combatDir(key, "dodge:"); ok {
			s.sendArea(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			s.render(false)
		}
		return
	}
	s.render(false)
}

// combatDir parses a "prefix:N" combat command into its 8-way direction,
// rejecting anything outside world.Dir's range.
func combatDir(key, prefix string) (int, bool) {
	d, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(d)
	if err != nil || n < int(world.DirS) || n > int(world.DirSW) {
		return 0, false
	}
	return n, true
}

// handleWorldEvent mirrors the HD loop's event handling: chat lines for the
// log, the few event types areas need forwarded, and the trade table's state
// machine.
func (s *session) handleWorldEvent(ev world.Event) {
	if ln, show := game.HDChatLine(ev, s.name); show {
		s.sendNow(ChatMsg{T: MsgChat, Text: ln.Text, Hex: rgbaHex(ln.Col), Kind: chatKind(ev)})
	}
	switch ev.Type {
	case world.EventSlide, world.EventDeck, world.EventChat,
		world.EventPlayerDamaged, world.EventPlayerDowned,
		world.EventPlayerRespawn, world.EventPlayerShoved:
		s.area, _ = s.area.Update(game.WorldEventMsg(ev))
		s.render(false)
	case world.EventPlayerActed:
		// A swing, a dodge, a parry: collect it for the next scene frame so the
		// client can animate the motion on the right actor.
		s.fx = append(s.fx, FX{Name: ev.Player, Act: ev.Detail, Target: ev.Target})
		s.render(false)
	case world.EventTrade:
		s.handleTradeEvent(ev)
	}
}

func (s *session) handleTradeEvent(ev world.Event) {
	switch ev.Detail {
	case world.TradeRequest:
		s.tradeReq = ev.Player
		s.system(ev.Player + " wants to trade — /accept or /decline")
	case world.TradeOpen:
		s.panelSel, s.tradeReq = 0, ""
		s.openPanel("trade")
	case world.TradeDone:
		if msg, ok := game.ApplyCompletedTrade(s.gctx); ok {
			s.system(msg)
		}
		s.closePanel()
	case world.TradeCancel:
		if s.panel == "trade" {
			s.closePanel()
		}
		s.system("trade cancelled")
	case world.TradeDeclined:
		s.system(ev.Player + " declined to trade")
	}
}

// system posts a server-side line to the player's own chat log.
func (s *session) system(text string) {
	s.sendNow(ChatMsg{T: MsgChat, Text: text, Kind: "system"})
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

// actionPrompt returns the contextual action where the player stands, matching
// the HD client's preference for a Prompter over a Hinter.
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

// moveKeyMsg converts a key name from the browser into the bubbletea KeyMsg the
// areas act on, for movement keys only — so a browser can no more inject an
// arbitrary key into an area than an SSH client can.
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

// chatKind labels an event for the client's log styling.
func chatKind(ev world.Event) string {
	switch ev.Type {
	case world.EventChat:
		return "say"
	case world.EventWhisper:
		return "whisper"
	case world.EventEmote:
		return "emote"
	}
	return "system"
}
