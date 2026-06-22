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
	rng   *rand.Rand
	frame int
	seq   int // monotonic, for unique creature ids
}

// New builds a simulation for a world over the given overworld generator (use
// the Wilds seed so creatures spawn on the same terrain every session).
func New(w *world.World, gen *worldgen.Generator) *Sim {
	return &Sim{w: w, gen: gen, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
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
		// Nobody to watch: reclaim everything so an empty world holds no animals.
		for _, c := range creatures {
			s.w.DespawnCreature(c.ID)
		}
		return
	}

	s.despawn(players, creatures)
	s.move(players, creatures)
	s.spawn(players, len(creatures))
}

// despawn reclaims creatures with no player within despawnAt tiles.
func (s *Sim) despawn(players []world.Player, creatures []world.Creature) {
	for _, c := range creatures {
		if nearestPlayerDist(players, c.X, c.Y) > despawnAt {
			s.w.DespawnCreature(c.ID)
		}
	}
}

// move steps each creature: flee a close player, else wander on its cadence.
func (s *Sim) move(players []world.Player, creatures []world.Creature) {
	for _, c := range creatures {
		sp, ok := game.SpeciesByKind(c.Kind)
		if !ok {
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
			cc.X, cc.Y, cc.Facing, cc.State = nx, ny, facing, state
			return true
		})
	}
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
