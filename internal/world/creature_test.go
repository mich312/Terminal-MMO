package world

import "testing"

func TestCreatureRegistry(t *testing.T) {
	w := New()
	defer w.Close()

	if got := w.CountCreatures("wilds"); got != 0 {
		t.Fatalf("fresh world has %d creatures, want 0", got)
	}

	w.SpawnCreature(Creature{ID: "deer-1", Kind: "deer", Area: "wilds", X: 3, Y: 4})
	w.SpawnCreature(Creature{ID: "fox-1", Kind: "fox", Area: "wilds", X: 9, Y: 1})
	w.SpawnCreature(Creature{ID: "fish-1", Kind: "fish", Area: "lake", X: 0, Y: 0})

	if got := w.CountCreatures("wilds"); got != 2 {
		t.Fatalf("CountCreatures(wilds) = %d, want 2", got)
	}
	if got := len(w.CreaturesInArea("lake")); got != 1 {
		t.Fatalf("CreaturesInArea(lake) = %d, want 1", got)
	}

	w.DespawnCreature("deer-1")
	if got := w.CountCreatures("wilds"); got != 1 {
		t.Fatalf("after despawn CountCreatures(wilds) = %d, want 1", got)
	}
}

func TestCreaturesInAreaSnapshotIsolated(t *testing.T) {
	w := New()
	defer w.Close()
	w.SpawnCreature(Creature{ID: "rabbit-1", Kind: "rabbit", Area: "wilds", X: 1, Y: 1})

	snap := w.CreaturesInArea("wilds")
	snap[0].X = 999 // mutating the snapshot must not touch shared state

	again := w.CreaturesInArea("wilds")
	if again[0].X != 1 {
		t.Fatalf("snapshot mutation leaked into world state: X = %d, want 1", again[0].X)
	}
}

func TestMutateCreature(t *testing.T) {
	w := New()
	defer w.Close()
	w.SpawnCreature(Creature{ID: "rabbit-1", Kind: "rabbit", Area: "wilds", X: 1, Y: 1})

	if w.MutateCreature("ghost", func(*Creature) bool { return true }) {
		t.Fatal("MutateCreature on a missing id returned true")
	}

	ok := w.MutateCreature("rabbit-1", func(c *Creature) bool {
		c.X, c.Y = 5, 6
		return true
	})
	if !ok {
		t.Fatal("MutateCreature returned false for an existing creature")
	}
	got := w.CreaturesInArea("wilds")[0]
	if got.X != 5 || got.Y != 6 {
		t.Fatalf("mutation not applied: got (%d,%d), want (5,6)", got.X, got.Y)
	}
}
