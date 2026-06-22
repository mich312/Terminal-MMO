// Package world holds the live, in-memory multiplayer state: who is where,
// per-room slide indices, and the pub/sub fan-out that keeps every SSH
// session in sync. One mutex guards everything — fine at this scale.
package world

import (
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/ui"
)

// Dir is an 8-way facing, derived from a player's last move. DirS (facing the
// camera) is the zero value, so a fresh player faces down.
type Dir int

const (
	DirS Dir = iota
	DirSE
	DirE
	DirNE
	DirN
	DirNW
	DirW
	DirSW
)

// Facing8 maps a movement delta to an 8-way facing. A zero delta keeps the
// current facing (the caller skips the update).
func Facing8(dx, dy int) Dir {
	switch {
	case dx > 0 && dy < 0:
		return DirNE
	case dx > 0 && dy > 0:
		return DirSE
	case dx > 0:
		return DirE
	case dx < 0 && dy < 0:
		return DirNW
	case dx < 0 && dy > 0:
		return DirSW
	case dx < 0:
		return DirW
	case dy < 0:
		return DirN
	default:
		return DirS
	}
}

// Player is a snapshot of one connected player. World methods hand out
// copies; nobody mutates shared state outside the world's mutex.
type Player struct {
	Name      string
	Area      string // area id; "" while still booting
	X, Y      int
	Color     lipgloss.Color
	Facing    Dir
	Style     int // avatar sprite style index
	Accessory int // avatar accessory index (0 = none)
	LastMoved time.Time
}

const eventBuffer = 64

// World is the single shared instance behind all sessions.
type World struct {
	mu        sync.Mutex
	players   map[string]*Player
	subs      map[string]chan Event
	pollers   map[string]bool  // subs that render by polling (HD): skip move/tick spam
	decks     map[string]*Deck // player-authored presentation decks
	deckOrder []string         // deck ids in creation order
	deckSeq   int
	persist   func(Deck)      // optional: save a deck on create/edit (set by main)
	removeFn  func(id string) // optional: delete a persisted deck (set by main)
	guestSeq  int
	pulse     bool
	stop      chan struct{}
	stopOnce  sync.Once

	// Shared co-op gate state: contribution pools and which gates are open.
	gatePool    map[string]int
	gateFixed   map[string]bool
	gatePersist func(gate string, pool int, fixed bool) // set by main

	// Shared placements: player-built structures keyed by absolute (x,y),
	// overlaid on the deterministic Wilds. Everyone sees the same set.
	placements   map[[2]int]Placement
	placementAdd func(Placement) // persist a placement (set by main)
	placementDel func(x, y int)  // persist a removal (set by main)

	// Player-to-player trading: live tables keyed by both traders' names,
	// pending incoming requests (recipient → requester), and finished trades
	// waiting for each party to apply its half (TakeCompletedTrade).
	trades    map[string]*tradeState
	pending   map[string]string
	completed map[string]CompletedTrade
}

// Placement is one player-built structure in the shared world.
type Placement struct {
	X, Y  int
	Kind  string // a game.Placeable id
	Owner string
	State string // opaque JSON: machine buffers + wall-clock; "" for static props
}

func New() *World {
	w := &World{
		players:    make(map[string]*Player),
		subs:       make(map[string]chan Event),
		pollers:    make(map[string]bool),
		stop:       make(chan struct{}),
		gatePool:   make(map[string]int),
		gateFixed:  make(map[string]bool),
		placements: make(map[[2]int]Placement),
		trades:     make(map[string]*tradeState),
		pending:    make(map[string]string),
		completed:  make(map[string]CompletedTrade),
	}
	go w.tickLoop()
	return w
}

// LoadPlacements seeds the shared placement set from persistence (called once at
// startup). SetPlacementPersist wires saving back.
func (w *World) LoadPlacements(ps []Placement) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range ps {
		w.placements[[2]int{p.X, p.Y}] = p
	}
}

// SetPlacementPersist registers callbacks to persist a placement and a removal.
func (w *World) SetPlacementPersist(add func(Placement), del func(x, y int)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.placementAdd, w.placementDel = add, del
}

// PlacementAt returns the structure placed at (x,y), if any.
func (w *World) PlacementAt(x, y int) (Placement, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.placements[[2]int{x, y}]
	return p, ok
}

// Place adds a structure at p.X,p.Y for everyone, persists it, and broadcasts an
// EventPlaced to the area. It refuses (returns false) if the cell is occupied.
func (w *World) Place(area string, p Placement) bool {
	w.mu.Lock()
	if _, taken := w.placements[[2]int{p.X, p.Y}]; taken {
		w.mu.Unlock()
		return false
	}
	w.placements[[2]int{p.X, p.Y}] = p
	if w.placementAdd != nil {
		w.placementAdd(p)
	}
	w.broadcastToArea(area, Event{Type: EventPlaced, Player: p.Owner, Area: area,
		X: p.X, Y: p.Y, Detail: p.Kind})
	w.mu.Unlock()
	return true
}

// Unplace removes whatever is placed at (x,y), persists the removal and
// broadcasts. Returns the removed placement and whether anything was there.
func (w *World) Unplace(area string, x, y int) (Placement, bool) {
	w.mu.Lock()
	p, ok := w.placements[[2]int{x, y}]
	if !ok {
		w.mu.Unlock()
		return Placement{}, false
	}
	delete(w.placements, [2]int{x, y})
	if w.placementDel != nil {
		w.placementDel(x, y)
	}
	w.broadcastToArea(area, Event{Type: EventPlaced, Player: p.Owner, Area: area, X: x, Y: y})
	w.mu.Unlock()
	return p, true
}

// MutatePlacement runs fn against the placement at (x,y) under the world mutex,
// for an atomic read-modify-write of its opaque State — the safe way to settle a
// race like two buyers hitting one stall at once. fn receives the current State
// and returns the new State plus whether it changed anything; the change is
// persisted and broadcast only when fn reports true. fn must stay pure (decode,
// decide, encode) and must not call back into the world. Returns false if
// nothing is placed there or fn made no change.
func (w *World) MutatePlacement(area string, x, y int, fn func(state string) (string, bool)) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.placements[[2]int{x, y}]
	if !ok {
		return false
	}
	ns, changed := fn(p.State)
	if !changed {
		return false
	}
	p.State = ns
	w.placements[[2]int{x, y}] = p
	if w.placementAdd != nil {
		w.placementAdd(p)
	}
	w.broadcastToArea(area, Event{Type: EventPlaced, Player: p.Owner, Area: area,
		X: x, Y: y, Detail: p.Kind})
	return true
}

// UpdatePlacementState rewrites the opaque State of the placement at (x,y)
// (a machine ticking, refueling or being collected), persists it and nudges a
// redraw. No-op (false) if nothing is placed there.
func (w *World) UpdatePlacementState(area string, x, y int, state string) bool {
	w.mu.Lock()
	p, ok := w.placements[[2]int{x, y}]
	if !ok {
		w.mu.Unlock()
		return false
	}
	p.State = state
	w.placements[[2]int{x, y}] = p
	if w.placementAdd != nil {
		w.placementAdd(p)
	}
	w.broadcastToArea(area, Event{Type: EventPlaced, Player: p.Owner, Area: area,
		X: x, Y: y, Detail: p.Kind})
	w.mu.Unlock()
	return true
}

// LoadGates seeds the shared co-op gate state from persistence (called once at
// startup). SetGatePersist wires saving back.
func (w *World) LoadGates(pools map[string]int, fixed map[string]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for g, n := range pools {
		w.gatePool[g] = n
	}
	for g, f := range fixed {
		w.gateFixed[g] = f
	}
}

// SetGatePersist registers a callback to persist co-op gate state on change.
func (w *World) SetGatePersist(fn func(gate string, pool int, fixed bool)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gatePersist = fn
}

// GateFixed reports whether a co-op gate is open for everyone.
func (w *World) GateFixed(gate string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gateFixed[gate]
}

// GatePool returns how many contributions a co-op gate has so far.
func (w *World) GatePool(gate string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gatePool[gate]
}

// OfferToGate adds one contribution toward a co-op gate; when the pool reaches
// need it locks the gate open for everyone. Returns the new pool and fixed flag.
func (w *World) OfferToGate(gate string, need int) (pool int, fixed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.gateFixed[gate] {
		return w.gatePool[gate], true
	}
	w.gatePool[gate]++
	if w.gatePool[gate] >= need {
		w.gateFixed[gate] = true
	}
	pool, fixed = w.gatePool[gate], w.gateFixed[gate]
	if w.gatePersist != nil {
		w.gatePersist(gate, pool, fixed)
	}
	return pool, fixed
}

// tickLoop broadcasts the global 2 Hz pulse that drives portal animation.
func (w *World) tickLoop() {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	var frame uint64
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			frame++
			w.mu.Lock()
			w.pulse = !w.pulse
			ev := Event{Type: EventTick, Pulse: w.pulse, Frame: frame}
			for subName, ch := range w.subs {
				if w.pollers[subName] {
					continue // HD polls its own animation clock; no tick needed
				}
				deliver(ch, ev)
			}
			w.mu.Unlock()
		}
	}
}

func (w *World) Close() {
	w.stopOnce.Do(func() { close(w.stop) })
}

// Join registers a connecting session. The desired name may be "" (an SSH
// username can be empty) or already taken; the resolved unique name is
// returned together with the session's event channel. The player has no
// area yet — call EnterArea once the boot sequence drops them in.
func (w *World) Join(desired string) (string, <-chan Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	name := desired
	if name == "" {
		w.guestSeq++
		name = fmt.Sprintf("guest-%d", w.guestSeq)
	}
	if _, taken := w.players[name]; taken {
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s-%d", name, i)
			if _, taken := w.players[candidate]; !taken {
				name = candidate
				break
			}
		}
	}

	w.players[name] = &Player{
		Name:  name,
		Color: ui.AvatarColor(name),
	}
	ch := make(chan Event, eventBuffer)
	w.subs[name] = ch
	return name, ch
}

// MarkPoller flags a session that renders by polling world state every frame
// (the HD client) rather than reacting to each event. Such a session needs none
// of the high-frequency positional stream — EventMoved and EventTick — so we
// stop delivering those to it. This keeps a movement flood in a busy area from
// evicting chat, whisper, slide and join/leave events out of its 64-deep buffer
// before it can read them.
func (w *World) MarkPoller(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pollers[name] = true
}

// Leave removes a player entirely (disconnect). Idempotent — it is called
// both from the quit path and the session-closed watchdog.
func (w *World) Leave(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return
	}
	area := p.Area
	w.cancelTradeLocked(name, TradeCancel) // a disconnect aborts any open trade
	delete(w.players, name)
	delete(w.pollers, name)
	if ch, ok := w.subs[name]; ok {
		delete(w.subs, name)
		close(ch)
	}
	if area != "" {
		w.broadcastToArea(area, Event{Type: EventLeft, Player: name, Area: area})
	}
}

// EnterArea moves a player into an area at the given spawn position and
// notifies both the old and the new area. destDisplay is the human name of
// the new area, used for "headed to …" toasts in the area left behind.
func (w *World) EnterArea(name, area string, x, y int, destDisplay string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return
	}
	old := p.Area
	if old != area {
		w.cancelTradeLocked(name, TradeCancel) // walking off to another area aborts a trade
	}
	p.Area = area
	p.X, p.Y = x, y
	p.LastMoved = time.Now()
	if old != "" && old != area {
		w.broadcastToArea(old, Event{Type: EventLeft, Player: name, Area: old, Detail: destDisplay})
	}
	w.broadcastToArea(area, Event{Type: EventJoined, Player: name, Area: area, X: x, Y: y})
}

// Move updates a player's position within their current area.
func (w *World) Move(name string, x, y int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return
	}
	if dx, dy := x-p.X, y-p.Y; dx != 0 || dy != 0 {
		p.Facing = Facing8(dx, dy)
	}
	p.X, p.Y = x, y
	p.LastMoved = time.Now()
	w.broadcastToArea(p.Area, Event{Type: EventMoved, Player: name, Area: p.Area, X: x, Y: y})
}

// Chat delivers a message to every subscriber in the sender's area within
// ChatRadius (Chebyshev) of the sender — including the sender.
func (w *World) Chat(name, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	sender, ok := w.players[name]
	if !ok || sender.Area == "" {
		return
	}
	w.proximity(sender, Event{Type: EventChat, Player: name, Area: sender.Area, X: sender.X, Y: sender.Y, Detail: text})
}

// Emote delivers a "/me" action to the same audience as Chat (proximity in
// the sender's area, including the sender).
func (w *World) Emote(name, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	sender, ok := w.players[name]
	if !ok || sender.Area == "" {
		return
	}
	w.proximity(sender, Event{Type: EventEmote, Player: name, Area: sender.Area, X: sender.X, Y: sender.Y, Detail: text})
}

// proximity fans an event to every subscriber in the sender's area within
// ChatRadius. Callers must hold w.mu.
func (w *World) proximity(sender *Player, ev Event) {
	for subName, ch := range w.subs {
		p, ok := w.players[subName]
		if !ok || p.Area != sender.Area {
			continue
		}
		if chebyshev(p.X, p.Y, sender.X, sender.Y) <= ChatRadius {
			deliver(ch, ev)
		}
	}
}

// Whisper privately delivers text to one player anywhere in the world.
// Returns false if the recipient is not online (so the caller can report it).
// The sender is not echoed — callers show their own copy locally.
func (w *World) Whisper(from, to, text string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.players[from]; !ok {
		return false
	}
	ch, ok := w.subs[to]
	if !ok {
		return false
	}
	deliver(ch, Event{Type: EventWhisper, Player: from, Target: to, Detail: text})
	return true
}

// SetColor changes a player's avatar color. The new color shows on everyone's
// next render. Returns false if the player is gone.
func (w *World) SetColor(name string, c lipgloss.Color) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return false
	}
	p.Color = c
	return true
}

// SetAvatar changes a player's sprite style and accessory. Returns false if the
// player is gone.
func (w *World) SetAvatar(name string, style, accessory int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return false
	}
	p.Style, p.Accessory = style, accessory
	return true
}

// PlayersInArea returns snapshots of everyone in an area.
func (w *World) PlayersInArea(area string) []Player {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []Player
	for _, p := range w.players {
		if p.Area == area {
			out = append(out, *p)
		}
	}
	return out
}

// AllPlayers returns snapshots of everyone online (for the Tab overlay).
func (w *World) AllPlayers() []Player {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Player, 0, len(w.players))
	for _, p := range w.players {
		out = append(out, *p)
	}
	return out
}

// Self returns the snapshot for one player.
func (w *World) Self(name string) (Player, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return Player{}, false
	}
	return *p, true
}

// Pulse returns the current portal-pulse phase.
func (w *World) Pulse() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pulse
}

// broadcastToArea sends ev to every subscriber currently in area.
// Callers must hold w.mu.
func (w *World) broadcastToArea(area string, ev Event) {
	for subName, ch := range w.subs {
		p, ok := w.players[subName]
		if !ok || p.Area != area {
			continue
		}
		if ev.Type == EventMoved && w.pollers[subName] {
			continue // HD reads positions from PlayersInArea each frame, not from moves
		}
		deliver(ch, ev)
	}
}

// deliver pushes an event without ever blocking the broadcaster: if the
// channel is full the oldest event is dropped (presence is eventually
// consistent).
func deliver(ch chan Event, ev Event) {
	select {
	case ch <- ev:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- ev:
		default:
		}
	}
}

func chebyshev(ax, ay, bx, by int) int {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dy := ay - by
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}
