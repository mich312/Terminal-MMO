package wildlife

import (
	"testing"

	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// testSeed is the live Wilds seed (kept in sync with internal/areas/wilds), so
// the simulation runs over the same terrain the real game uses without pulling
// in the area package.
const testSeed uint64 = 0xD0117_C0FFEE_5742

// newSim builds a sim over the real overworld generator (no persistence).
func newSim(w *world.World) *Sim {
	return New(w, worldgen.New(testSeed), nil)
}

// walkableAwayFrom finds open, non-portal land at Chebyshev distance >= want
// from (px,py), to place a companion a few tiles off its owner.
func walkableAwayFrom(gen *worldgen.Generator, px, py, want int) (int, int, bool) {
	for r := want; r < want+10; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if max(abs(dx), abs(dy)) != r {
					continue
				}
				x, y := px+dx, py+dy
				if c := gen.At(x, y); c.Walkable && c.Portal == "" {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// walkableNearGate finds open, non-portal land near the home gate to stand a
// player on.
func walkableNearGate(gen *worldgen.Generator) (int, int) {
	for r := 0; r < 60; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				x, y := worldgen.GateX+dx, worldgen.GateY+dy
				if c := gen.At(x, y); c.Walkable && c.Portal == "" {
					return x, y
				}
			}
		}
	}
	return worldgen.GateX, worldgen.GateY
}

// joinAt connects a player and stands them at (x,y) in the Wilds.
func joinAt(t *testing.T, w *world.World, name string, x, y int) {
	t.Helper()
	resolved, _ := w.Join(name)
	w.EnterArea(resolved, Area, x, y, "The Wilds")
}

func TestNoPlayersNoSpawn(t *testing.T) {
	w := world.New()
	defer w.Close()
	s := newSim(w)

	for i := 0; i < 20; i++ {
		s.Step()
	}
	if got := w.CountCreatures(Area); got != 0 {
		t.Fatalf("with no players, world spawned %d creatures, want 0", got)
	}
}

func TestEmptyWorldReclaimsCreatures(t *testing.T) {
	w := world.New()
	defer w.Close()
	s := newSim(w)
	w.SpawnCreature(world.Creature{ID: "deer-x", Kind: "deer", Area: Area, X: 3, Y: 3})

	s.Step() // nobody online → the herd is reclaimed
	if got := w.CountCreatures(Area); got != 0 {
		t.Fatalf("an empty world kept %d creatures, want 0", got)
	}
}

func TestSpawnsNearPlayer(t *testing.T) {
	w := world.New()
	defer w.Close()
	gen := worldgen.New(testSeed)
	s := New(w, gen, nil)
	px, py := walkableNearGate(gen)
	joinAt(t, w, "anna", px, py)

	for i := 0; i < 40; i++ {
		s.Step()
	}
	cs := w.CreaturesInArea(Area)
	if len(cs) == 0 {
		t.Fatal("no wildlife spawned near an online player after 40 ticks")
	}
	if len(cs) > maxTotal {
		t.Fatalf("population %d exceeds the hard cap %d", len(cs), maxTotal)
	}
	for _, c := range cs {
		if d := chebyshev(px, py, c.X, c.Y); d > despawnAt {
			t.Fatalf("creature %s spawned at distance %d, beyond despawn radius %d", c.ID, d, despawnAt)
		}
	}
}

func TestDespawnsAwayFromPlayers(t *testing.T) {
	w := world.New()
	defer w.Close()
	gen := worldgen.New(testSeed)
	s := New(w, gen, nil)
	px, py := walkableNearGate(gen)
	joinAt(t, w, "anna", px, py)

	// A creature far from any player should be reclaimed on the next step.
	w.SpawnCreature(world.Creature{ID: "fox-far", Kind: "fox", Area: Area,
		X: px + despawnAt + 25, Y: py})
	s.Step()
	for _, c := range w.CreaturesInArea(Area) {
		if c.ID == "fox-far" {
			t.Fatal("a creature far from every player was not despawned")
		}
	}
}

func TestFleeDoesNotApproach(t *testing.T) {
	w := world.New()
	defer w.Close()
	gen := worldgen.New(testSeed)
	s := New(w, gen, nil)
	px, py := walkableNearGate(gen)
	joinAt(t, w, "anna", px, py)

	// Put a rabbit right next to the player; a skittish animal must never end a
	// step closer than it started (it flees, or is blocked and holds).
	rx, ry := px+1, py
	w.SpawnCreature(world.Creature{ID: "rabbit-near", Kind: "rabbit", Area: Area, X: rx, Y: ry})
	before := chebyshev(px, py, rx, ry)

	s.move(w.PlayersInArea(Area), w.CreaturesInArea(Area))

	var found bool
	for _, c := range w.CreaturesInArea(Area) {
		if c.ID == "rabbit-near" {
			found = true
			if after := chebyshev(px, py, c.X, c.Y); after < before {
				t.Fatalf("rabbit moved toward the player: %d -> %d", before, after)
			}
		}
	}
	if !found {
		t.Fatal("the rabbit vanished during a move step")
	}
}

func TestCompanionFollowsOwner(t *testing.T) {
	w := world.New()
	defer w.Close()
	gen := worldgen.New(testSeed)
	s := New(w, gen, nil)
	px, py := walkableNearGate(gen)
	joinAt(t, w, "anna", px, py)

	cx, cy, ok := walkableAwayFrom(gen, px, py, 4)
	if !ok {
		t.Skip("no walkable tile a few steps from the gate on this seed")
	}
	w.SpawnCreature(world.Creature{ID: "fox-pet-anna-1", Kind: "fox", Area: Area,
		X: cx, Y: cy, Owner: "anna", State: "tamed", HP: 4})
	before := chebyshev(px, py, cx, cy)

	for i := 0; i < 40; i++ {
		s.Step()
	}

	var after int
	found := false
	for _, c := range w.CreaturesInArea(Area) {
		if c.ID == "fox-pet-anna-1" {
			found, after = true, chebyshev(px, py, c.X, c.Y)
		}
	}
	if !found {
		t.Fatal("the companion was despawned while its owner was present")
	}
	if after >= before {
		t.Fatalf("companion did not approach its owner: %d -> %d", before, after)
	}
}

func TestCompanionDespawnsWhenOwnerAbsent(t *testing.T) {
	w := world.New()
	defer w.Close()
	gen := worldgen.New(testSeed)
	s := New(w, gen, nil)
	px, py := walkableNearGate(gen)
	joinAt(t, w, "anna", px, py) // anna is here; bob is not

	w.SpawnCreature(world.Creature{ID: "fox-pet-bob-1", Kind: "fox", Area: Area,
		X: px + 2, Y: py, Owner: "bob", State: "tamed", HP: 4})
	s.Step()

	for _, c := range w.CreaturesInArea(Area) {
		if c.ID == "fox-pet-bob-1" {
			t.Fatal("a companion whose owner is absent was not reclaimed")
		}
	}
}
