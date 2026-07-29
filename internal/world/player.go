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

// StrikeOutcome is the referee's ruling on one blow (docs/SWORDPLAY_PLAN.md).
// The game layer turns it into toasts, staggers and knockback; the world only
// decides what happened, atomically.
type StrikeOutcome int

const (
	// StrikeMissed: no such target, already down, or in the respawn grace.
	StrikeMissed StrikeOutcome = iota
	// StrikeDodged: the target was mid-dodge — the blow passed through air.
	StrikeDodged
	// StrikeHit: a clean hit; damage applied in full.
	StrikeHit
	// StrikeDowned: the hit emptied the target's HP; they're knocked out.
	StrikeDowned
	// StrikeGuarded: the target's raised guard softened the blow (half damage).
	StrikeGuarded
	// StrikeGuardBroken: a strong blow smashed a raised guard open — full
	// damage, and the caller should stagger the defender.
	StrikeGuardBroken
	// StrikeParried: the guard came up inside ParryWindow — no damage, the
	// target earns a riposte, and the caller should stagger the attacker.
	StrikeParried
)

// Connected reports whether the blow actually cost the target HP.
func (o StrikeOutcome) Connected() bool {
	switch o {
	case StrikeHit, StrikeDowned, StrikeGuarded, StrikeGuardBroken:
		return true
	}
	return false
}

// Strike applies dmg to target on attacker's behalf and tells the area about it.
// It is the single atomic path for hurting a player: HP floors at 0, and the
// blow that empties it knocks the target out for downedFor (immune until they
// respawn). The caller (the game layer) has already decided dmg/downedFor and
// checked the zone rules; weapon is the attacker's weapon name for the toast
// ("" = bare hands). strong marks a heavy blow, which is what breaks guards.
//
// The referee consults the target's defensive state under the same lock the
// state is set under: a dodge's immunity window, and a raised guard — fresh
// enough to parry (no damage, riposte earned), or standing (damage halved,
// unless the blow was strong, which smashes the guard open and lands in full).
//
// Returns the target's remaining HP and the outcome; a second hit on a downed
// player rules StrikeMissed, not a double knock-out.
func (w *World) Strike(attacker, target, weapon string, dmg int, strong bool, downedFor time.Duration) (hp int, out StrikeOutcome) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, exists := w.players[target]
	if !exists {
		return 0, StrikeMissed
	}
	now := time.Now()
	if !p.DownedUntil.IsZero() && p.DownedUntil.After(now) {
		return p.HP, StrikeMissed // already knocked out — can't be hit again
	}
	if !p.InvulnUntil.IsZero() && p.InvulnUntil.After(now) {
		return p.HP, StrikeMissed // just respawned — briefly protected
	}
	if !p.DodgeUntil.IsZero() && p.DodgeUntil.After(now) {
		return p.HP, StrikeDodged // rolled clear — the blow finds only air
	}
	if dmg < 0 {
		dmg = 0
	}
	out = StrikeHit
	if p.Guarding {
		switch {
		case now.Sub(p.GuardStart) <= ParryWindow:
			// A fresh guard turns the blade aside entirely. The parrier earns a
			// riposte; the attacker's stagger is the caller's to apply (it owns
			// walkability). Everyone nearby hears the steel.
			p.RiposteUntil = now.Add(RiposteWindow)
			w.broadcastToArea(p.Area, Event{Type: EventPlayerActed, Player: target, Target: attacker, Area: p.Area, X: p.X, Y: p.Y, Detail: ActParry})
			return p.HP, StrikeParried
		case strong:
			// A heavy blow smashes a standing guard open: full damage, and the
			// guard drops — it must be raised again.
			p.Guarding = false
			out = StrikeGuardBroken
		default:
			dmg /= 2
			if dmg < 1 {
				dmg = 1 // a guarded blow still chips
			}
			out = StrikeGuarded
		}
	}
	p.HP -= dmg
	if p.HP < 0 {
		p.HP = 0
	}
	p.LastHurt = now
	p.LastHurtBy = attacker

	x, y := p.X, p.Y
	if p.HP == 0 {
		p.DownedUntil = now.Add(downedFor)
		w.broadcastToArea(p.Area, Event{Type: EventPlayerDowned, Player: attacker, Target: target, Area: p.Area, X: x, Y: y, Detail: weapon})
		return 0, StrikeDowned
	}
	w.broadcastToArea(p.Area, Event{Type: EventPlayerDamaged, Player: attacker, Target: target, Area: p.Area, X: x, Y: y, Detail: weapon})
	return p.HP, out
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
	now := time.Now()
	p.HP = p.MaxHP
	p.DownedUntil = time.Time{}
	p.LastHurtBy = ""
	p.InvulnUntil = now.Add(RespawnImmunity)
	p.Area = area
	p.X, p.Y = x, y
	p.LastMoved = now
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

// Shove tells a player they've been knocked back to (x,y) by attacker. Because
// each session owns its own position, this only broadcasts the intent — the
// victim's client moves itself (and re-validates the cell); everyone else sees
// the resulting move. No-op for an unknown target.
func (w *World) Shove(attacker, target string, x, y int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[target]
	if !ok {
		return
	}
	w.broadcastToArea(p.Area, Event{Type: EventPlayerShoved, Player: attacker, Target: target, Area: p.Area, X: x, Y: y})
}

// Immune reports whether the named player is in their post-respawn grace window.
func (w *World) Immune(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return false
	}
	return !p.InvulnUntil.IsZero() && p.InvulnUntil.After(time.Now())
}

// SetFacing turns a player in place without a step (docs/SWORDPLAY_PLAN.md).
// The action camera aims strikes by look direction rather than by the last
// move, which is the only way facing has ever changed before. Broadcasts a
// move event at the current cell so event-driven clients redraw the turn;
// polling clients pick it up on their next frame anyway.
func (w *World) SetFacing(name string, d Dir) {
	if d < DirS || d > DirSW {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok || p.Facing == d {
		return
	}
	p.Facing = d
	w.broadcastToArea(p.Area, Event{Type: EventMoved, Player: name, Area: p.Area, X: p.X, Y: p.Y})
}

// SetGuard raises or lowers a player's blade. Raising stamps GuardStart with
// the server's clock — the timestamp Strike judges the parry window against —
// and re-raising while already up does not refresh it, so a guard cannot be
// held permanently fresh by repeating the input.
func (w *World) SetGuard(name string, up bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok || p.Guarding == up {
		return
	}
	p.Guarding = up
	if up {
		p.GuardStart = time.Now()
	}
}

// BeginDodge opens the named player's dodge immunity window for d. The game
// layer owns the duration (and the movement that goes with it); the world just
// stamps the state Strike consults.
func (w *World) BeginDodge(name string, d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return
	}
	p.DodgeUntil = time.Now().Add(d)
}

// TakeRiposte consumes the named player's riposte window if it is open,
// reporting whether it was. At most one strike gets the bonus per parry —
// read-and-clear under the one lock, so two quick blows can't both claim it.
func (w *World) TakeRiposte(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return false
	}
	if p.RiposteUntil.IsZero() || !p.RiposteUntil.After(time.Now()) {
		return false
	}
	p.RiposteUntil = time.Time{}
	return true
}

// Acted broadcasts a combat motion — a swing, a dodge — to the actor's area so
// every client can animate it. Whiffs broadcast too: a fight is only readable
// (baitable, dodgeable) if the swing itself is visible, not just the damage.
func (w *World) Acted(name, act string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.players[name]
	if !ok {
		return
	}
	w.broadcastToArea(p.Area, Event{Type: EventPlayerActed, Player: name, Area: p.Area, X: p.X, Y: p.Y, Detail: act})
}
