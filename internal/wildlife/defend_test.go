package wildlife

import (
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

// findCreature re-reads a live creature by id.
func findCreature(w *world.World, id string) (world.Creature, bool) {
	for _, c := range w.CreaturesInArea(Area) {
		if c.ID == id {
			return c, true
		}
	}
	return world.Creature{}, false
}

// A tamed companion bites the player who just struck its owner — out in the open
// Wilds, where fighting is allowed.
func TestCompanionDefendsOwner(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	s := newSim(w)
	gen := s.gen

	px, py, ok := walkableAwayFrom(gen, 0, 0, 40) // a spot outside the hub ward
	if !ok {
		t.Skip("no open ground found far from the hub")
	}
	owner, _ := w.Join("owner")
	w.EnterArea(owner, Area, px, py, "")
	rogue, _ := w.Join("rogue")
	w.EnterArea(rogue, Area, px, py, "") // stand on the owner for a clean in-place bite
	w.SpawnCreature(world.Creature{ID: "pet", Kind: "fox", Area: Area, X: px, Y: py, State: "tamed", Owner: owner, HP: 4})

	// The rogue strikes the owner — rousing the companion.
	if _, _, hit := w.Strike(rogue, owner, "knife", 1, time.Second); !hit {
		t.Fatal("setup strike on owner failed")
	}
	pet, _ := findCreature(w, "pet")
	if foe, ok := s.defendTarget(pet, w.PlayersInArea(Area)); !ok || foe.Name != rogue {
		t.Fatalf("companion should target the rogue, got %q ok=%v", foe.Name, ok)
	}

	before, _ := w.Self(rogue)
	sp, _ := game.SpeciesByKind("fox")
	for i := 0; i < 12; i++ {
		s.frame++
		pet, _ = findCreature(w, "pet")
		if foe, ok := s.defendTarget(pet, w.PlayersInArea(Area)); ok {
			s.defend(pet, sp, foe)
		}
	}
	after, _ := w.Self(rogue)
	if after.HP >= before.HP {
		t.Fatalf("rogue HP %d did not drop from %d — the pet never bit", after.HP, before.HP)
	}
}

// In a safe zone (the hub ward), a companion does not retaliate, even if its
// owner was struck.
func TestCompanionHoldsInSafeZone(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	s := newSim(w)

	owner, _ := w.Join("owner")
	w.EnterArea(owner, Area, 0, 0, "") // the hub heart — a sanctuary
	rogue, _ := w.Join("rogue")
	w.EnterArea(rogue, Area, 0, 0, "")
	w.SpawnCreature(world.Creature{ID: "pet", Kind: "fox", Area: Area, X: 0, Y: 0, State: "tamed", Owner: owner, HP: 4})
	w.Strike(rogue, owner, "knife", 1, time.Second)

	pet, _ := findCreature(w, "pet")
	if _, ok := s.defendTarget(pet, w.PlayersInArea(Area)); ok {
		t.Fatal("a companion must not fight in the hub's peace ward")
	}
}
