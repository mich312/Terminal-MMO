package game

// Weapons (docs/WEAPON_PLAN.md). A weapon is a multiplier on the single strike
// action: more damage, sometimes more reach. It is "wielded" simply by owning
// it — the same pattern as the axe/pick tools (placeable.go) — so there is no
// equip step. The unified strike resolves against a creature or a player through
// one path; this catalog only says how hard and how far a blow lands.
//
// This lives in game (beside Species and Recipe) because it is shared, static
// presentation/rules both clients and the wilds strike code read.

// Weapon is one wieldable arm.
type Weapon struct {
	Item     string // inventory item id (also the recipe output); "" for bare hands
	Name     string // display name, for prompts and toasts
	Damage   int    // HP removed per strike
	Reach    int    // 1 = melee (adjacent ring); >1 = ranged (tiles along facing)
	Cooldown int    // ticks between strikes (reserved; throttles spam in a later pass)
	Ammo     string // "" for melee; an item id consumed per shot (e.g. "arrow")
}

// Fists is the implicit weapon everyone always has: a light, melee, no-cost
// strike. BestWeapon falls back to it when the pack holds nothing better.
var Fists = Weapon{Item: "", Name: "bare hands", Damage: 1, Reach: 1}

// weapons is the craftable roster, in ascending order of clout. All inputs are
// existing inventory items (stone, wood, leather, feather), so weapons slot onto
// the recipe bench with no new forage plumbing.
var weapons = []Weapon{
	{Item: "knife", Name: "Flint Knife", Damage: 2, Reach: 1, Cooldown: 1},
	{Item: "spear", Name: "Spear", Damage: 3, Reach: 1, Cooldown: 2},
	{Item: "bow", Name: "Hunter's Bow", Damage: 2, Reach: 4, Cooldown: 2, Ammo: "arrow"},
}

var weaponByItem = func() map[string]Weapon {
	m := make(map[string]Weapon, len(weapons))
	for _, wp := range weapons {
		m[wp.Item] = wp
	}
	return m
}()

// WeaponByItem resolves an item id to its Weapon; ok is false for a non-weapon.
func WeaponByItem(item string) (Weapon, bool) {
	wp, ok := weaponByItem[item]
	return wp, ok
}

// IsWeapon reports whether an item id names a weapon.
func IsWeapon(item string) bool {
	_, ok := weaponByItem[item]
	return ok
}

// BestWeapon picks the strongest usable arm the pack holds, falling back to
// Fists. "Usable" excludes a ranged weapon with no ammo, so a bow with an empty
// quiver yields to a knife you also carry (and ultimately to your fists). The
// ranking is by Damage, with longer Reach breaking ties — a clear, no-UI wield.
func BestWeapon(inv map[string]int) Weapon {
	best := Fists
	for _, wp := range weapons {
		if inv[wp.Item] <= 0 {
			continue
		}
		if wp.Ammo != "" && inv[wp.Ammo] <= 0 {
			continue // owns the bow but has no arrows — can't loose a shot
		}
		if wp.Damage > best.Damage || (wp.Damage == best.Damage && wp.Reach > best.Reach) {
			best = wp
		}
	}
	return best
}
