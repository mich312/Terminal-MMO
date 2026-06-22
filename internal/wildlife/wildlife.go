// Package wildlife is the live animal simulation: the first server-owned,
// autonomous entities in Durst World. A single Sim rides the world's tick,
// spawning biome-appropriate creatures near online players, drifting and
// startling them, and despawning them once nobody is around to watch.
//
// It is deliberately the *only* writer of creatures: every session renders the
// herd by reading world.CreaturesInArea snapshots in its normal redraw, so the
// simulation adds no network fan-out. Spawning is seeded by location so the
// world still reads as authored, but motion is live.
package wildlife

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// Area is the only area wildlife inhabits at MVP.
const Area = "wilds"

// Tuning knobs. Population is bounded by online players (not the infinite map):
// an empty region costs nothing, a crowded one is capped. See the performance
// analysis in docs/WILDLIFE_PLAN.md.
const (
	maxPerPlayer = 6  // soft budget contributed by each online player
	maxTotal     = 48 // hard ceiling on live creatures in the area
	spawnRadius  = 11 // spawn this far from a player — at the edge of sight, so they wander in
	spawnInner   = 8  // …but no closer than this, so they never pop in on top of you
	despawnAt    = 16 // a creature with no player within this many tiles is reclaimed
	tickInterval = 500 * time.Millisecond
)

// Sim owns the wildlife population for one world. Only its own goroutine
// (or a test calling Step) touches its fields; all shared state lives behind
// the world mutex.
type Sim struct {
	w     *world.World
	gen   *worldgen.Generator
	store store.Store // companion persistence; may be nil (tests, no-DB)
	rng   *rand.Rand
	frame int
	seq   int // monotonic, for unique creature ids

	// compCache memoizes each present player's saved companion species (""
	// meaning none), so the reattach pass doesn't hit the store every tick. An
	// entry is dropped when its player leaves, so a reconnect re-reads it.
	compCache map[string]string
}

// New builds a simulation for a world over the given overworld generator (use
// the Wilds seed so creatures spawn on the same terrain every session). st
// persists tamed companions; pass nil to run without persistence (tests).
func New(w *world.World, gen *worldgen.Generator, st store.Store) *Sim {
	return &Sim{w: w, gen: gen, store: st,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		compCache: map[string]string{}}
}

// Run steps the simulation at the world's 2 Hz cadence until stop is closed.
// Start it once from main, like the persistence wiring.
func (s *Sim) Run(stop <-chan struct{}) {
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.Step()
		}
	}
}

// Step advances the herd by one tick: census, spawn, move, despawn. Exported so
// tests can drive it deterministically without a clock.
func (s *Sim) Step() {
	s.frame++
	players := s.w.PlayersInArea(Area)
	creatures := s.w.CreaturesInArea(Area)

	if len(players) == 0 {
		// Nobody to watch: reclaim everything (companions persist in the store and
		// reattach on return) so an empty world holds no animals.
		for _, c := range creatures {
			s.w.DespawnCreature(c.ID)
		}
		s.compCache = map[string]string{}
		return
	}

	s.reconcileCompanions(players, creatures)
	creatures = s.w.CreaturesInArea(Area) // include any companions just reattached
	s.despawn(players, creatures)
	s.move(players, creatures)
	s.spawn(players, s.w.CountCreatures(Area))
}

// reconcileCompanions keeps each player's tamed pet in sync with who's present:
// a companion whose owner has left the area is despawned (it persists and
// reattaches later), and a present player with a saved companion but no live one
// gets it spawned beside them.
func (s *Sim) reconcileCompanions(players []world.Player, creatures []world.Creature) {
	present := make(map[string]bool, len(players))
	for _, p := range players {
		present[p.Name] = true
	}
	ownedPresent := map[string]bool{}
	for _, c := range creatures {
		if c.Owner == "" {
			continue
		}
		if !present[c.Owner] {
			s.w.DespawnCreature(c.ID) // owner walked off / disconnected
			delete(s.compCache, c.Owner)
			continue
		}
		ownedPresent[c.Owner] = true
	}
	for _, p := range players {
		if ownedPresent[p.Name] {
			continue
		}
		if kind := s.companionKind(p.Name); kind != "" {
			s.spawnCompanion(p, kind)
		}
	}
}

// companionKind returns a player's saved companion species (cached), "" if none.
func (s *Sim) companionKind(name string) string {
	if k, ok := s.compCache[name]; ok {
		return k
	}
	k := ""
	if s.store != nil {
		if ck, ok := s.store.LoadCompanion(name); ok {
			k = ck
		}
	}
	s.compCache[name] = k
	return k
}

// spawnCompanion places a player's tamed pet on an open tile beside them.
func (s *Sim) spawnCompanion(p world.Player, kind string) {
	sp, ok := game.SpeciesByKind(kind)
	if !ok {
		delete(s.compCache, p.Name) // unknown species id — forget it
		return
	}
	for _, o := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
		nx, ny := p.X+o[0], p.Y+o[1]
		if s.habitable(sp, nx, ny) {
			s.seq++
			s.w.SpawnCreature(world.Creature{
				ID: fmt.Sprintf("%s-pet-%s-%d", kind, p.Name, s.seq), Kind: kind, Area: Area,
				X: nx, Y: ny, Facing: world.DirS, State: "tamed", Owner: p.Name, HP: sp.MaxHP,
			})
			return
		}
	}
}

// despawn reclaims wild creatures with no player within despawnAt tiles.
// Companions (Owner set) are exempt — they follow their owner and are managed by
// reconcileCompanions instead.
func (s *Sim) despawn(players []world.Player, creatures []world.Creature) {
	for _, c := range creatures {
		if c.Owner != "" {
			continue
		}
		if nearestPlayerDist(players, c.X, c.Y) > despawnAt {
			s.w.DespawnCreature(c.ID)
		}
	}
}

// move steps each creature: a companion trails its owner; a wild animal flees a
// close player, else wanders on its cadence.
func (s *Sim) move(players []world.Player, creatures []world.Creature) {
	for _, c := range creatures {
		sp, ok := game.SpeciesByKind(c.Kind)
		if !ok {
			continue
		}
		if c.Owner != "" {
			if foe, ok := s.defendTarget(c, players); ok {
				s.defend(c, sp, foe)
			} else {
				s.follow(c, sp, players)
			}
			continue
		}
		px, py, dist := nearestPlayer(players, c.X, c.Y)
		var dx, dy int
		state := "wander"
		switch {
		case sp.FleeRadius > 0 && dist <= sp.FleeRadius:
			// Bolt directly away from the nearest player.
			dx, dy = sign(c.X-px), sign(c.Y-py)
			state = "flee"
		case s.frame%moveEvery(sp) != 0:
			continue // not this creature's turn to amble
		default:
			dx, dy = s.rng.Intn(3)-1, s.rng.Intn(3)-1
		}
		if dx == 0 && dy == 0 {
			continue
		}
		nx, ny := c.X+dx, c.Y+dy
		if !s.habitable(sp, nx, ny) {
			continue // stay put rather than walk into water/walls
		}
		id := c.ID
		facing := world.Facing8(dx, dy)
		s.w.MutateCreature(id, func(cc *world.Creature) bool {
			cc.X, cc.Y, cc.Facing, cc.State, cc.LastMoved = nx, ny, facing, state, time.Now()
			return true
		})
	}
}

// follow steps a companion one tile toward its owner (greedy, no pathfinding):
// it closes the gap when it's lagging and just turns to face them when it's
// already at heel. If the direct diagonal is blocked it tries the two orthogonal
// steps, so a pet rounds a tree instead of getting stuck on it.
func (s *Sim) follow(c world.Creature, sp game.Species, players []world.Player) {
	var owner world.Player
	found := false
	for _, p := range players {
		if p.Name == c.Owner {
			owner, found = p, true
			break
		}
	}
	if !found {
		return // owner absent — reconcileCompanions will reclaim the pet
	}
	dx, dy := sign(owner.X-c.X), sign(owner.Y-c.Y)
	if chebyshev(owner.X, owner.Y, c.X, c.Y) <= 1 {
		// At heel: hold position, just turn to face the owner.
		if dx == 0 && dy == 0 {
			return
		}
		facing := world.Facing8(dx, dy)
		s.w.MutateCreature(c.ID, func(cc *world.Creature) bool {
			if cc.Facing == facing {
				return false
			}
			cc.Facing = facing
			return true
		})
		return
	}
	nx, ny := c.X+dx, c.Y+dy
	if !s.habitable(sp, nx, ny) {
		switch {
		case dx != 0 && s.habitable(sp, c.X+dx, c.Y):
			ny = c.Y
		case dy != 0 && s.habitable(sp, c.X, c.Y+dy):
			nx = c.X
		default:
			return // boxed in this tick; try again next
		}
	}
	facing := world.Facing8(nx-c.X, ny-c.Y)
	s.w.MutateCreature(c.ID, func(cc *world.Creature) bool {
		cc.X, cc.Y, cc.Facing, cc.State, cc.LastMoved = nx, ny, facing, "tamed", time.Now()
		return true
	})
}

// Companion defense (docs/WEAPON_PLAN.md): a tamed pet stands up for its owner.
// When someone strikes you in the open Wilds, your companion drops the heel and
// goes for the attacker — closing the gap and biting on a cadence — until the
// threat passes or it loses them.
const (
	defendWindow = 6 * time.Second // how long after a blow a pet stays roused
	aggroRadius  = 9               // it only takes up a fight this near its owner
	biteEvery    = 2               // bites once every N ticks (~1/sec at 2 Hz)
	petKnockout  = 5 * time.Second // a pet's blow downs a player for this long, like any strike
)

// defendTarget returns the player a companion should fight for its owner, if
// any: the owner's recent attacker, still present, near enough, not already
// down, and standing where fighting is allowed (never the hub or a claim).
func (s *Sim) defendTarget(c world.Creature, players []world.Player) (world.Player, bool) {
	var owner world.Player
	found := false
	for _, p := range players {
		if p.Name == c.Owner {
			owner, found = p, true
			break
		}
	}
	if !found || owner.LastHurtBy == "" || time.Since(owner.LastHurt) > defendWindow {
		return world.Player{}, false
	}
	for _, p := range players {
		if p.Name != owner.LastHurtBy {
			continue
		}
		if s.w.Downed(p.Name) || !s.pvpHere(p.X, p.Y) {
			return world.Player{}, false
		}
		if chebyshev(owner.X, owner.Y, p.X, p.Y) > aggroRadius {
			return world.Player{}, false
		}
		return p, true
	}
	return world.Player{}, false
}

// defend moves a companion toward the foe and bites when it's in reach. The bite
// is credited to the owner ("ada's fox catches you") and respects the same
// knock-out rules as any strike.
func (s *Sim) defend(c world.Creature, sp game.Species, foe world.Player) {
	if chebyshev(c.X, c.Y, foe.X, foe.Y) <= 1 {
		facing := world.Facing8(sign(foe.X-c.X), sign(foe.Y-c.Y))
		s.w.MutateCreature(c.ID, func(cc *world.Creature) bool {
			cc.Facing, cc.State, cc.LastMoved = facing, "tamed", time.Now()
			return true
		})
		if s.frame%biteEvery == 0 {
			s.w.Strike(c.Owner, foe.Name, sp.Name, petBite(sp), petKnockout)
		}
		return
	}
	// Close the gap, stepping around obstacles like follow does.
	dx, dy := sign(foe.X-c.X), sign(foe.Y-c.Y)
	nx, ny := c.X+dx, c.Y+dy
	if !s.habitable(sp, nx, ny) {
		switch {
		case dx != 0 && s.habitable(sp, c.X+dx, c.Y):
			ny = c.Y
		case dy != 0 && s.habitable(sp, c.X, c.Y+dy):
			nx = c.X
		default:
			return
		}
	}
	facing := world.Facing8(nx-c.X, ny-c.Y)
	s.w.MutateCreature(c.ID, func(cc *world.Creature) bool {
		cc.X, cc.Y, cc.Facing, cc.State, cc.LastMoved = nx, ny, facing, "tamed", time.Now()
		return true
	})
}

// petBite is how hard a companion bites — gentle, scaled a little by the
// animal's size so a deer hits harder than a rabbit.
func petBite(sp game.Species) int {
	if d := sp.MaxHP / 3; d > 1 {
		return d
	}
	return 1
}

// pvpHere mirrors game.PvPAllowedAt for the sim (which has no game.Ctx): the open
// Wilds allow fighting, the hub ward and claimed land do not.
func (s *Sim) pvpHere(x, y int) bool {
	if worldgen.HubSafe(x, y) {
		return false
	}
	if _, claimed := s.w.ClaimAt(x, y); claimed {
		return false
	}
	return true
}

// spawn tops the population up toward the player-scaled budget, placing new
// creatures at the edge of a random player's sight so they drift into view.
func (s *Sim) spawn(players []world.Player, have int) {
	budget := len(players) * maxPerPlayer
	if budget > maxTotal {
		budget = maxTotal
	}
	// At most a few births per tick keeps the herd easing in, not popping in.
	for tries := 0; have < budget && tries < 8; tries++ {
		p := players[s.rng.Intn(len(players))]
		ang := s.rng.Float64() * 2 * math.Pi
		r := spawnInner + s.rng.Intn(spawnRadius-spawnInner+1)
		nx := p.X + int(float64(r)*math.Cos(ang))
		ny := p.Y + int(float64(r)*math.Sin(ang))
		cell := s.gen.At(nx, ny)
		sp, ok := s.pickSpecies(cell.Biome)
		if !ok || !s.habitable(sp, nx, ny) || occupied(players, nx, ny) {
			continue
		}
		s.seq++
		s.w.SpawnCreature(world.Creature{
			ID:   fmt.Sprintf("%s-%d", sp.Kind, s.seq),
			Kind: sp.Kind,
			Area: Area,
			X:    nx, Y: ny,
			Facing: world.DirS,
			State:  "wander",
			HP:     sp.MaxHP,
		})
		have++
	}
}

// pickSpecies chooses a random species whose habitat includes the biome.
func (s *Sim) pickSpecies(b worldgen.Biome) (game.Species, bool) {
	var candidates []game.Species
	for _, sp := range game.SpeciesList() {
		for _, hb := range sp.Biomes {
			if hb == b {
				candidates = append(candidates, sp)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return game.Species{}, false
	}
	return candidates[s.rng.Intn(len(candidates))], true
}

// habitable reports whether a species may stand on (x,y): aquatic animals need
// open water, land animals need walkable, non-portal ground.
func (s *Sim) habitable(sp game.Species, x, y int) bool {
	cell := s.gen.At(x, y)
	if cell.Portal != "" {
		return false
	}
	if sp.Aquatic {
		return cell.Biome == worldgen.Water || cell.Biome == worldgen.Deep
	}
	if !cell.Walkable {
		return false
	}
	// A blocking placement (fence, machine) stops an animal too.
	if pl, ok := s.w.PlacementAt(x, y); ok {
		if pb, ok := game.PlaceableByID(pl.Kind); ok && !pb.Walkable {
			return false
		}
	}
	return true
}

// moveEvery is a species' wander cadence in ticks, floored at 1.
func moveEvery(sp game.Species) int {
	if sp.MoveEvery < 1 {
		return 1
	}
	return sp.MoveEvery
}

func occupied(players []world.Player, x, y int) bool {
	for _, p := range players {
		if p.X == x && p.Y == y {
			return true
		}
	}
	return false
}

func nearestPlayer(players []world.Player, x, y int) (px, py, dist int) {
	dist = 1 << 30
	for _, p := range players {
		if d := chebyshev(p.X, p.Y, x, y); d < dist {
			dist, px, py = d, p.X, p.Y
		}
	}
	return px, py, dist
}

func nearestPlayerDist(players []world.Player, x, y int) int {
	_, _, d := nearestPlayer(players, x, y)
	return d
}

func chebyshev(ax, ay, bx, by int) int {
	dx, dy := abs(ax-bx), abs(ay-by)
	if dx > dy {
		return dx
	}
	return dy
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}
