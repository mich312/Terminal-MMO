package game

import "testing"

func TestBestWeaponFallsBackToFists(t *testing.T) {
	if got := BestWeapon(map[string]int{}); got.Item != "" || got.Damage != Fists.Damage {
		t.Fatalf("empty pack should wield fists, got %+v", got)
	}
	// Raw materials are not weapons.
	if got := BestWeapon(map[string]int{"wood": 9, "stone": 9}); got.Item != "" {
		t.Fatalf("non-weapons should not be wielded, got %q", got.Item)
	}
}

func TestBestWeaponPicksHighestDamage(t *testing.T) {
	got := BestWeapon(map[string]int{"knife": 1, "spear": 1})
	if got.Item != "spear" {
		t.Fatalf("with a knife and a spear, want spear (more damage), got %q", got.Item)
	}
}

func TestBestWeaponBowNeedsArrows(t *testing.T) {
	// A bow with no arrows can't loose a shot, so a knife wins.
	got := BestWeapon(map[string]int{"bow": 1, "knife": 1})
	if got.Item != "knife" {
		t.Fatalf("bow with empty quiver should yield to knife, got %q", got.Item)
	}
	// Give it arrows and the ranged option becomes usable; on a damage tie with
	// the knife (both 2), longer reach wins.
	got = BestWeapon(map[string]int{"bow": 1, "arrow": 3, "knife": 1})
	if got.Item != "bow" {
		t.Fatalf("bow with arrows should win on reach tie, got %q", got.Item)
	}
}

func TestWeaponRecipesResolveToRealItems(t *testing.T) {
	for _, wp := range weapons {
		if _, ok := ItemByID(wp.Item); !ok {
			t.Errorf("weapon %q has no inventory item", wp.Item)
		}
		var rec *Recipe
		for i := range Recipes {
			if Recipes[i].Out == wp.Item {
				rec = &Recipes[i]
				break
			}
		}
		if rec == nil {
			t.Errorf("weapon %q has no recipe", wp.Item)
			continue
		}
		for _, in := range rec.In {
			if _, ok := ItemByID(in.Item); !ok {
				t.Errorf("weapon %q recipe needs unknown item %q", wp.Item, in.Item)
			}
		}
		if wp.Ammo != "" {
			if _, ok := ItemByID(wp.Ammo); !ok {
				t.Errorf("weapon %q ammo %q is not an item", wp.Item, wp.Ammo)
			}
		}
	}
}
