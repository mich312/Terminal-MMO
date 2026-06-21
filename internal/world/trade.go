package world

import "errors"

// Player-to-player trading. A trade is a face-to-face swap: one player requests,
// the other accepts, both lay items on their side of the table, and once both
// mark ready the swap is committed. The world owns the negotiation (this file);
// it never touches inventory itself — on commit it records what each side gave
// and got, and each session applies its own half (CompletedTrade) to its
// inventory. That keeps the world inventory-agnostic and works even with
// persistence disabled.

// TradeRadius is how close two players must stand to trade (Chebyshev tiles).
const TradeRadius = 3

// tradeState is one live negotiation, stored under both traders' names.
type tradeState struct {
	who   [2]string
	offer [2]map[string]int
	ready [2]bool
}

func (t *tradeState) idx(name string) int {
	if t.who[1] == name {
		return 1
	}
	return 0
}

// TradeSnapshot is one trader's view of the live table ("you" vs "them").
type TradeSnapshot struct {
	With       string
	YourOffer  map[string]int
	TheirOffer map[string]int
	YouReady   bool
	ThemReady  bool
}

// CompletedTrade is the agreed swap, from one party's side: what they handed
// over and what they receive. Each session applies this to its own inventory.
type CompletedTrade struct {
	With string
	Gave map[string]int
	Got  map[string]int
}

var (
	errBusy     = errors.New("one of you is already trading")
	errFar      = errors.New("you need to stand together to trade")
	errSelf     = errors.New("you can't trade with yourself")
	errNoParty  = errors.New("they're not here")
	errNoTrade  = errors.New("you're not in a trade")
	errNoOffer  = errors.New("no pending request from them")
)

// RequestTrade asks `to` for a trade. Both must be online, in the same area,
// within TradeRadius, and not already trading. The recipient gets a request
// they can accept or decline.
func (w *World) RequestTrade(from, to string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if from == to {
		return errSelf
	}
	if _, _, err := w.tradeEligibleLocked(from, to); err != nil {
		return err
	}
	w.pending[to] = from
	w.deliverTo(to, Event{Type: EventTrade, Player: from, Target: to, Detail: TradeRequest})
	return nil
}

// AcceptTrade opens the table after `to` accepts a request from `from`.
func (w *World) AcceptTrade(to, from string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending[to] != from {
		return errNoOffer
	}
	if _, _, err := w.tradeEligibleLocked(from, to); err != nil {
		delete(w.pending, to)
		return err
	}
	delete(w.pending, to)
	t := &tradeState{who: [2]string{from, to}, offer: [2]map[string]int{{}, {}}}
	w.trades[from] = t
	w.trades[to] = t
	w.notifyTradeLocked(t, TradeOpen)
	return nil
}

// DeclineTrade rejects a pending request and tells the requester.
func (w *World) DeclineTrade(to, from string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending[to] != from {
		return
	}
	delete(w.pending, to)
	w.deliverTo(from, Event{Type: EventTrade, Player: to, Target: from, Detail: TradeDeclined})
}

// SetOffer replaces a trader's whole offer (item id → count). Any change resets
// both ready flags, so nobody confirms a table that then shifts under them.
func (w *World) SetOffer(name string, offer map[string]int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.trades[name]
	if !ok {
		return errNoTrade
	}
	cp := make(map[string]int, len(offer))
	for k, v := range offer {
		if v > 0 {
			cp[k] = v
		}
	}
	i := t.idx(name)
	t.offer[i] = cp
	t.ready = [2]bool{false, false}
	w.notifyTradeLocked(t, TradeUpdate)
	return nil
}

// SetReady marks a trader ready (or not). When both are ready the swap commits:
// each side's CompletedTrade is recorded and the table closes.
func (w *World) SetReady(name string, ready bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.trades[name]
	if !ok {
		return errNoTrade
	}
	t.ready[t.idx(name)] = ready
	if t.ready[0] && t.ready[1] {
		w.completeTradeLocked(t)
		return nil
	}
	w.notifyTradeLocked(t, TradeUpdate)
	return nil
}

// CancelTrade aborts a trader's open table (or clears a request they sent).
func (w *World) CancelTrade(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cancelTradeLocked(name, TradeCancel)
}

// TradeSnapshot returns a trader's current view of their live table.
func (w *World) TradeSnapshot(name string) (TradeSnapshot, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.trades[name]
	if !ok {
		return TradeSnapshot{}, false
	}
	me := t.idx(name)
	them := 1 - me
	return TradeSnapshot{
		With:       t.who[them],
		YourOffer:  cloneCounts(t.offer[me]),
		TheirOffer: cloneCounts(t.offer[them]),
		YouReady:   t.ready[me],
		ThemReady:  t.ready[them],
	}, true
}

// TakeCompletedTrade hands a session the swap it just agreed to (and clears it),
// so it can apply the delta to its own inventory exactly once.
func (w *World) TakeCompletedTrade(name string) (CompletedTrade, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.completed[name]
	if ok {
		delete(w.completed, name)
	}
	return c, ok
}

// Trading reports whether a player currently has an open trade table.
func (w *World) Trading(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.trades[name]
	return ok
}

// ── internals (callers hold w.mu) ──────────────────────────────────────────

// tradeEligibleLocked validates that from and to can start a trade together.
func (w *World) tradeEligibleLocked(from, to string) (*Player, *Player, error) {
	a, ok := w.players[from]
	if !ok {
		return nil, nil, errNoParty
	}
	b, ok := w.players[to]
	if !ok {
		return nil, nil, errNoParty
	}
	if _, busy := w.trades[from]; busy {
		return nil, nil, errBusy
	}
	if _, busy := w.trades[to]; busy {
		return nil, nil, errBusy
	}
	if a.Area == "" || a.Area != b.Area || chebyshev(a.X, a.Y, b.X, b.Y) > TradeRadius {
		return nil, nil, errFar
	}
	return a, b, nil
}

// completeTradeLocked records each side's agreed delta and closes the table.
func (w *World) completeTradeLocked(t *tradeState) {
	w.completed[t.who[0]] = CompletedTrade{With: t.who[1], Gave: cloneCounts(t.offer[0]), Got: cloneCounts(t.offer[1])}
	w.completed[t.who[1]] = CompletedTrade{With: t.who[0], Gave: cloneCounts(t.offer[1]), Got: cloneCounts(t.offer[0])}
	delete(w.trades, t.who[0])
	delete(w.trades, t.who[1])
	w.notifyTradeLocked(t, TradeDone)
}

// cancelTradeLocked tears down any trade or pending request involving name and
// notifies the other party.
func (w *World) cancelTradeLocked(name, phase string) {
	if t, ok := w.trades[name]; ok {
		delete(w.trades, t.who[0])
		delete(w.trades, t.who[1])
		w.notifyTradeLocked(t, phase)
	}
	// Clear a request this player received or sent.
	delete(w.pending, name)
	for to, from := range w.pending {
		if from == name {
			delete(w.pending, to)
		}
	}
}

// notifyTradeLocked tells both traders the table changed; each is told who the
// other party is so the client can render "you" vs "them".
func (w *World) notifyTradeLocked(t *tradeState, phase string) {
	w.deliverTo(t.who[0], Event{Type: EventTrade, Player: t.who[1], Target: t.who[0], Detail: phase})
	w.deliverTo(t.who[1], Event{Type: EventTrade, Player: t.who[0], Target: t.who[1], Detail: phase})
}

// deliverTo pushes an event to one player's subscription if present.
func (w *World) deliverTo(name string, ev Event) {
	if ch, ok := w.subs[name]; ok {
		deliver(ch, ev)
	}
}

func cloneCounts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
