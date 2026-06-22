package world

import "time"

// Combat primitives on the World (docs/WEAPON_PLAN.md). These mirror the
// creature side (MutateCreature): the world owns atomicity under the one mutex
// and the event fan-out, while the *game* layer owns the meaning — how much a
// weapon hurts, how long a knock-out lasts, and where a player respawns. The
// world never decides those; it just applies and broadcasts what it's told,
// race-safely, so two attackers on one victim can't both claim the knock-out.

// MutatePlayer runs fn against the named player under the world mutex, for an
// atomic read-modify-write — the player twin of MutateCreature. fn mutates in
// place and returns whether it changed anything. Returns false if no such
// player exists or fn reported no change. It broadcasts nothing; use Strike /
// Respawn when onlookers need to see the result.
func (w *World) MutatePlayer(name string, fn func(*Player) bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return false
	}
	return fn(p)
}

// Strike applies dmg to target on attacker's behalf and tells the area about it.
// It is the single atomic path for hurting a player: HP floors at 0, and the
// blow that empties it knocks the target out for downedFor (immune until they
// respawn). The caller (the game layer) has already decided dmg/downedFor and
// checked the zone rules; weapon is the attacker's weapon name for the toast
// ("" = bare hands).
//
// Returns the target's remaining HP, whether this strike downed them, and ok =
// false if the target is unknown or already down (so a second hit on a downed
// player is a no-op, not a double knock-out).
func (w *World) Strike(attacker, target, weapon string, dmg int, downedFor time.Duration) (hp int, downed, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, exists := w.players[target]
	if !exists {
		return 0, false, false
	}
	now := time.Now()
	if !p.DownedUntil.IsZero() && p.DownedUntil.After(now) {
		return p.HP, false, false // already knocked out — can't be hit again
	}
	if dmg < 0 {
		dmg = 0
	}
	p.HP -= dmg
	if p.HP < 0 {
		p.HP = 0
	}
	p.LastHurt = now

	x, y := p.X, p.Y
	if p.HP == 0 {
		p.DownedUntil = now.Add(downedFor)
		w.broadcastToArea(p.Area, Event{Type: EventPlayerDowned, Player: attacker, Target: target, Area: p.Area, X: x, Y: y, Detail: weapon})
		return 0, true, true
	}
	w.broadcastToArea(p.Area, Event{Type: EventPlayerDamaged, Player: attacker, Target: target, Area: p.Area, X: x, Y: y, Detail: weapon})
	return p.HP, false, true
}

// Respawn puts a downed player back on their feet: full HP, knock-out cleared,
// repositioned to (area, x, y). The game layer supplies the spawn — the world
// doesn't know where any area's hub is. Broadcasts EventPlayerRespawn to the
// destination area so everyone redraws them upright. No-op for an unknown name.
func (w *World) Respawn(name, area string, x, y int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return
	}
	p.HP = p.MaxHP
	p.DownedUntil = time.Time{}
	p.Area = area
	p.X, p.Y = x, y
	p.LastMoved = time.Now()
	w.broadcastToArea(area, Event{Type: EventPlayerRespawn, Player: name, Target: name, Area: area, X: x, Y: y})
}

// Downed reports whether the named player is currently knocked out.
func (w *World) Downed(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return false
	}
	return !p.DownedUntil.IsZero() && p.DownedUntil.After(time.Now())
}
