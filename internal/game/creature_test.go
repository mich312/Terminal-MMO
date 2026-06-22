package game

import (
	"math/rand"
	"testing"

	"github.com/durst-group/durstworld/internal/world"
)

func TestRollDropsRanges(t *testing.T) {
	r := rand.New(rand.NewSource(1))

	deer, ok := SpeciesByKind("deer")
	if !ok {
		t.Fatal("deer species missing")
	}
	for i := 0; i < 50; i++ {
		got := RollDrops(deer, r)
		if got["hide"] != 1 {
			t.Fatalf("deer hide = %d, want exactly 1", got["hide"])
		}
		if got["meat"] < 1 || got["meat"] > 2 {
			t.Fatalf("deer meat = %d, want 1..2", got["meat"])
		}
	}

	rabbit, _ := SpeciesByKind("rabbit")
	for i := 0; i < 20; i++ {
		if n := RollDrops(rabbit, r)["meat"]; n != 1 {
			t.Fatalf("rabbit meat = %d, want a guaranteed 1", n)
		}
	}
}

func TestRollDropsChance(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	fox, _ := SpeciesByKind("fox")
	meatSeen := 0
	for i := 0; i < 200; i++ {
		got := RollDrops(fox, r)
		if got["pelt"] != 1 {
			t.Fatalf("fox pelt = %d, want a guaranteed 1", got["pelt"])
		}
		if got["meat"] > 0 {
			meatSeen++
		}
	}
	// The chance-0.5 meat should land somewhere in the broad middle, never always
	// or never.
	if meatSeen == 0 || meatSeen == 200 {
		t.Fatalf("fox meat dropped %d/200 times — chance roll looks broken", meatSeen)
	}
}

// Every drop must reference a real catalog item, or the toast/inventory would
// show a phantom id.
func TestDropItemsExist(t *testing.T) {
	for _, sp := range SpeciesList() {
		for _, d := range sp.Drops {
			if _, ok := ItemByID(d.Item); !ok {
				t.Errorf("species %q drops unknown item %q", sp.Kind, d.Item)
			}
		}
	}
}

// Every creature sprite must be a clean 12×10 grid across all three views and
// both walk frames — a mis-sized row would skew or clip the animal.
func TestCreatureSpritesWellFormed(t *testing.T) {
	const w, h = 12, 10
	for _, sp := range SpeciesList() {
		cs, ok := creatureSprites[sp.Kind]
		if !ok {
			t.Errorf("species %q has no sprite", sp.Kind)
			continue
		}
		for vname, view := range map[string][2][]string{"front": cs.front, "back": cs.back, "side": cs.side} {
			for f := 0; f < 2; f++ {
				rows := view[f]
				if len(rows) != h {
					t.Errorf("%s %s frame %d: %d rows, want %d", sp.Kind, vname, f, len(rows), h)
					continue
				}
				for r, row := range rows {
					if n := len([]rune(row)); n != w {
						t.Errorf("%s %s frame %d row %d: width %d, want %d (%q)", sp.Kind, vname, f, r, n, w, row)
					}
				}
			}
		}
	}
}

// Every species resolves to a bitmap for all eight facings.
func TestCreatureBitmapAllFacings(t *testing.T) {
	dirs := []world.Dir{world.DirN, world.DirNE, world.DirE, world.DirSE, world.DirS, world.DirSW, world.DirW, world.DirNW}
	for _, sp := range SpeciesList() {
		for _, d := range dirs {
			if _, ok := CreatureBitmap(sp.Kind, d, 0); !ok {
				t.Errorf("%s has no bitmap for facing %v", sp.Kind, d)
			}
		}
	}
}
