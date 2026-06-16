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

// Player is a snapshot of one connected player. World methods hand out
// copies; nobody mutates shared state outside the world's mutex.
type Player struct {
	Name      string
	Area      string // area id; "" while still booting
	X, Y      int
	Color     lipgloss.Color
	LastMoved time.Time
}

const eventBuffer = 64

// World is the single shared instance behind all sessions.
type World struct {
	mu       sync.Mutex
	players  map[string]*Player
	subs     map[string]chan Event
	slides   map[string]int // room key → slide index, persists while server runs
	guestSeq int
	pulse    bool
	stop     chan struct{}
	stopOnce sync.Once
}

func New() *World {
	w := &World{
		players: make(map[string]*Player),
		subs:    make(map[string]chan Event),
		slides:  make(map[string]int),
		stop:    make(chan struct{}),
	}
	go w.tickLoop()
	return w
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
			for _, ch := range w.subs {
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
	delete(w.players, name)
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
	ev := Event{Type: EventChat, Player: name, Area: sender.Area, X: sender.X, Y: sender.Y, Detail: text}
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

// Slide returns the current slide index for a presentation room key.
func (w *World) Slide(roomKey string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.slides[roomKey]
}

// ChangeSlide moves a room's slide index by delta, clamped to [0, max-1],
// and broadcasts the change to the area. Returns the new index.
func (w *World) ChangeSlide(area, roomKey string, delta, max int, by string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	idx := w.slides[roomKey] + delta
	if idx < 0 {
		idx = 0
	}
	if idx > max-1 {
		idx = max - 1
	}
	w.slides[roomKey] = idx
	w.broadcastToArea(area, Event{Type: EventSlide, Player: by, Area: area, Detail: roomKey, Slide: idx})
	return idx
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
