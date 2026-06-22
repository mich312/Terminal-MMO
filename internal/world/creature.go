package world

import "time"

// Creature is one live, server-owned wild animal. Unlike players (a session
// each) and unlike the deterministic terrain/items, creatures are simulated:
// the wildlife stepper spawns, moves and despawns them under the world mutex,
// and both clients render them by reading snapshots in their normal redraw —
// no per-creature event is broadcast, so the live herd is free on the wire.
//
// The world stays schema-agnostic about a creature: it stores the fields and
// guards them, but never interprets Kind/State/HP/Owner — the wildlife
// simulation (and later the interaction code) owns that meaning.
type Creature struct {
	ID        string // stable instance id, unique while alive
	Kind      string // species id; resolved to a look + behavior elsewhere
	Area      string // area id (Phase 1: only "wilds")
	X, Y      int
	Facing    Dir
	State     string    // opaque: "wander" | "flee" | "graze" | … (Phase 2+: "tamed")
	HP        int       // 0 until hunting (Phase 2); world never reads it
	Owner     string    // "" until taming (Phase 3); the companion's player
	LastMoved time.Time // for the renderer's walk/idle animation, like a player
}

// CreaturesInArea returns snapshots of every live creature in an area — the
// read the renderers poll each frame, mirroring PlayersInArea.
func (w *World) CreaturesInArea(area string) []Creature {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []Creature
	for _, c := range w.creatures {
		if c.Area == area {
			out = append(out, *c)
		}
	}
	return out
}

// CountCreatures reports how many live creatures are in an area (the stepper's
// budget check).
func (w *World) CountCreatures(area string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.creatures {
		if c.Area == area {
			n++
		}
	}
	return n
}

// SpawnCreature registers a new live creature. Rendering polls CreaturesInArea,
// so there is deliberately no broadcast here — the next tick/poll redraw shows
// it.
func (w *World) SpawnCreature(c Creature) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cc := c
	w.creatures[c.ID] = &cc
}

// DespawnCreature removes a creature by id, if present.
func (w *World) DespawnCreature(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.creatures, id)
}

// MutateCreature runs fn against the creature with the given id under the world
// mutex, for an atomic read-modify-write — the race-safe way to step a creature
// (and, in Phase 2, to apply a strike so two players can't double-kill one).
// fn mutates the creature in place and returns whether it changed anything; the
// return is only informational at MVP since nothing is broadcast. Returns false
// if no such creature exists or fn reported no change.
func (w *World) MutateCreature(id string, fn func(*Creature) bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.creatures[id]
	if !ok {
		return false
	}
	return fn(c)
}
