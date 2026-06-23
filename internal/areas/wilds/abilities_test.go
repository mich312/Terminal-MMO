package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

func spawnRabbit(w *world.World, id string, x, y int) {
	w.SpawnCreature(world.Creature{ID: id, Kind: "rabbit", Area: "wilds", X: x, Y: y, HP: 2})
}

// Backstab adds damage when you strike a foe from behind, and not from the front.
func TestBackstabBonus(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	a := newFighter(t, w, "ada", 10, 10)
	dagger, _ := game.WeaponByItem("dagger")

	// Target at (12,10) facing east (away from ada to its west) → ada is behind.
	if !a.isBehind(12, 10, world.DirE) {
		t.Fatal("attacker west of an east-facing target should be behind it")
	}
	if got := a.weaponDamage(dagger, 12, 10, world.DirE); got != dagger.Damage+game.BackstabBonus {
		t.Fatalf("backstab damage = %d, want %d", got, dagger.Damage+game.BackstabBonus)
	}
	// Facing west (toward ada) → a face-to-face strike, no bonus.
	if a.isBehind(12, 10, world.DirW) {
		t.Fatal("a west-facing target met head-on is not backstabbed")
	}
	if got := a.weaponDamage(dagger, 12, 10, world.DirW); got != dagger.Damage {
		t.Fatalf("frontal damage = %d, want %d", got, dagger.Damage)
	}
}

// A cleaving blade catches every adjacent foe; a plain blade takes just one.
func TestCleaveHitsAllAdjacent(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	a := newFighter(t, w, "ada", 100, 100)
	a.ctx.Inventory["sword"] = 1 // Cast Blade: cleave, damage 4 (one-shots a rabbit)
	spawnRabbit(w, "r1", 101, 100)
	spawnRabbit(w, "r2", 100, 101)

	a.strike()
	if n := w.CountCreatures("wilds"); n != 0 {
		t.Fatalf("after a cleave, %d rabbits remain, want 0", n)
	}

	// A plain knife takes only one of two.
	b := newFighter(t, w, "bo", 100, 100)
	b.ctx.Inventory["knife"] = 1
	spawnRabbit(w, "r3", 101, 100)
	spawnRabbit(w, "r4", 100, 101)
	b.strike()
	if n := w.CountCreatures("wilds"); n != 1 {
		t.Fatalf("after a plain strike, %d rabbits remain, want 1", n)
	}
}

// A piercing shot rakes every foe along its line; a plain bow stops at the first.
func TestPierceHitsAlongLine(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	a := newFighter(t, w, "ada", 100, 100) // faces south by default
	a.ctx.Inventory["skypiercer"] = 1
	a.ctx.Inventory["arrow"] = 9
	spawnRabbit(w, "r1", 100, 102)
	spawnRabbit(w, "r2", 100, 104)

	a.strike()
	if n := w.CountCreatures("wilds"); n != 0 {
		t.Fatalf("after a pierce, %d rabbits remain, want 0", n)
	}

	b := newFighter(t, w, "bo", 100, 100)
	b.ctx.Inventory["bow"] = 1
	b.ctx.Inventory["arrow"] = 9
	spawnRabbit(w, "r3", 100, 102)
	spawnRabbit(w, "r4", 100, 104)
	b.strike()
	if n := w.CountCreatures("wilds"); n != 1 {
		t.Fatalf("after a plain shot, %d rabbits remain, want 1 (the far one)", n)
	}
}

// A knockback weapon shoves a surviving creature a tile away when the ground
// behind it is open.
func TestKnockbackShovesCreature(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	a := newFighter(t, w, "ada", 100, 100)
	a.ctx.Inventory["spear"] = 1 // knockback, damage 3 (a deer survives)
	w.SpawnCreature(world.Creature{ID: "d1", Kind: "deer", Area: "wilds", X: 101, Y: 100, HP: 6})

	dest := [2]int{102, 100} // pushed east, away from ada
	a.strike()

	var got world.Creature
	for _, c := range w.CreaturesInArea("wilds") {
		if c.ID == "d1" {
			got = c
		}
	}
	if got.ID == "" {
		t.Fatal("the deer should survive a single spear blow")
	}
	if a.walkableAt(dest[0], dest[1]) {
		if got.X != dest[0] || got.Y != dest[1] {
			t.Fatalf("deer at (%d,%d), want shoved to %v", got.X, got.Y, dest)
		}
	} else if got.X != 101 || got.Y != 100 {
		t.Fatalf("deer at (%d,%d), want held at (101,100) (blocked behind)", got.X, got.Y)
	}
}
