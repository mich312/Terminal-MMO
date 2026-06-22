package world

// Land claims — a player's deed over a settlement plot (docs/CLAIMS_PLAN.md).
// Like the co-op gate pool, claims are shared world state persisted through a
// callback rather than a visual placement: a claim is a *right over a region*,
// not a tile object, so keeping it out of the placements map avoids polluting the
// terrain overlay and collision. The world stores each claim's parcel box and a
// last-touch clock; the *lapse policy* (how long absence forfeits a claim) lives
// in the game layer, which passes an expiry cutoff in when it matters.

// Claim is one player's deed over a settlement plot.
type Claim struct {
	PlotID                 string // worldgen plot id this deed covers
	Owner                  string // username of the holder
	MinX, MinY, MaxX, MaxY int    // parcel bounding box (build-rights region)
	LastTouch              int64  // unix seconds of the owner's last presence
}

// Covers reports whether (x,y) lies in the claim's parcel box.
func (c Claim) Covers(x, y int) bool {
	return x >= c.MinX && x <= c.MaxX && y >= c.MinY && y <= c.MaxY
}

// LoadClaims seeds the claim set from persistence (called once at startup).
func (w *World) LoadClaims(cs []Claim) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range cs {
		w.claims[c.PlotID] = c
	}
}

// SetClaimPersist registers callbacks to persist a claim and a release.
func (w *World) SetClaimPersist(add func(Claim), del func(plotID string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.claimPersist, w.claimDel = add, del
}

// ClaimAt returns the claim whose parcel covers (x,y), if any. The raw claim is
// returned (including a possibly-lapsed one); the caller applies the lapse rule.
func (w *World) ClaimAt(x, y int) (Claim, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range w.claims {
		if c.Covers(x, y) {
			return c, true
		}
	}
	return Claim{}, false
}

// ClaimForPlot returns the claim on a specific plot id, if any.
func (w *World) ClaimForPlot(plotID string) (Claim, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.claims[plotID]
	return c, ok
}

// ClaimPlot deeds c.PlotID to c.Owner, atomically, if the plot is unclaimed or
// the standing claim has lapsed — i.e. its LastTouch is at or before expireBefore
// (the game passes now-leasePeriod). A live claim held by someone else blocks it;
// the owner re-deeding their own live claim just refreshes it. Returns ok.
func (w *World) ClaimPlot(c Claim, expireBefore int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if cur, ok := w.claims[c.PlotID]; ok {
		lapsed := cur.LastTouch <= expireBefore
		if !lapsed && cur.Owner != c.Owner {
			return false // a live claim by another player
		}
	}
	w.claims[c.PlotID] = c
	if w.claimPersist != nil {
		w.claimPersist(c)
	}
	w.broadcastToArea("wilds", Event{Type: EventPlaced, Player: c.Owner, Area: "wilds",
		X: c.MinX, Y: c.MinY})
	return true
}

// TouchClaim refreshes the lease on the owner's claim (their presence in the
// parcel), no-op if the plot isn't theirs. Persists the new clock.
func (w *World) TouchClaim(plotID, owner string, now int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.claims[plotID]
	if !ok || c.Owner != owner || c.LastTouch >= now {
		return
	}
	c.LastTouch = now
	w.claims[plotID] = c
	if w.claimPersist != nil {
		w.claimPersist(c)
	}
}

// ReleaseClaim gives up the owner's claim on a plot, persists the removal and
// broadcasts. Returns false if the plot isn't claimed by owner.
func (w *World) ReleaseClaim(plotID, owner string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.claims[plotID]
	if !ok || c.Owner != owner {
		return false
	}
	delete(w.claims, plotID)
	if w.claimDel != nil {
		w.claimDel(plotID)
	}
	w.broadcastToArea("wilds", Event{Type: EventPlaced, Player: owner, Area: "wilds",
		X: c.MinX, Y: c.MinY})
	return true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
