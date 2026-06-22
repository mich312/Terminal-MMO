package world

// Cleared terrain — cells a player has felled or quarried clear with a tool
// (docs/BUILD_TOOLS_PLAN.md). Like claims, this is a sparse stored overlay on the
// pure-seed terrain: a cleared cell renders as walkable ground and is buildable.
// Each carries an owner and a last-touch clock; the game layer's lapse policy
// regrows a cell once it's been untouched too long, so the woods reclaim ghost
// towns. The world just stores the cells and a timestamp.

// Cleared is one cell cleared of its blocking terrain feature.
type Cleared struct {
	X, Y      int
	Owner     string
	LastTouch int64 // unix seconds of the last touch (creation or owner presence)
}

// LoadCleared seeds the cleared set from persistence (called once at startup).
func (w *World) LoadCleared(cs []Cleared) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range cs {
		w.cleared[[2]int{c.X, c.Y}] = c
	}
}

// SetClearPersist registers callbacks to persist a clear and a regrowth.
func (w *World) SetClearPersist(add func(Cleared), del func(x, y int)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clearAdd, w.clearDel = add, del
}

// ClearedAt returns the cleared record at (x,y), if any (raw — the caller applies
// the regrowth/lapse rule).
func (w *World) ClearedAt(x, y int) (Cleared, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.cleared[[2]int{x, y}]
	return c, ok
}

// ClearedOverlapping returns copies of every cleared cell inside the box — for
// the renderer to overlay a window in one pass.
func (w *World) ClearedOverlapping(minX, minY, maxX, maxY int) []Cleared {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []Cleared
	for k, c := range w.cleared {
		if k[0] >= minX && k[0] <= maxX && k[1] >= minY && k[1] <= maxY {
			out = append(out, c)
		}
	}
	return out
}

// ClearCell records (x,y) as cleared by owner at time now, persists and
// broadcasts. Overwrites any prior record (a re-clear refreshes the clock).
func (w *World) ClearCell(c Cleared) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cleared[[2]int{c.X, c.Y}] = c
	if w.clearAdd != nil {
		w.clearAdd(c)
	}
	w.broadcastToArea("wilds", Event{Type: EventPlaced, Player: c.Owner, Area: "wilds", X: c.X, Y: c.Y})
}

// TouchCleared refreshes the clock on a cleared cell the owner is using, no-op if
// it isn't theirs. Keeps the clearing from regrowing while it's lived in.
func (w *World) TouchCleared(x, y int, owner string, now int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.cleared[[2]int{x, y}]
	if !ok || c.Owner != owner || c.LastTouch >= now {
		return
	}
	c.LastTouch = now
	w.cleared[[2]int{x, y}] = c
	if w.clearAdd != nil {
		w.clearAdd(c)
	}
}

// Regrow removes a cleared cell (it has grown back), persists and broadcasts.
func (w *World) Regrow(x, y int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.cleared[[2]int{x, y}]
	if !ok {
		return
	}
	delete(w.cleared, [2]int{x, y})
	if w.clearDel != nil {
		w.clearDel(x, y)
	}
	w.broadcastToArea("wilds", Event{Type: EventPlaced, Player: c.Owner, Area: "wilds", X: x, Y: y})
}
