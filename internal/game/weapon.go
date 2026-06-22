package game

import (
	"strings"

	"github.com/durst-group/durstworld/internal/worldgen"
)

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

	// How you come by it (docs/WEAPON_PLAN.md). A weapon is one of:
	//   - craftable (the default): built at the bench from a recipe;
	//   - Found: hidden in the world, turned up by exploring (no recipe);
	//   - Unique: a one-per-world legend — it spawns in a single hidden spot, and
	//     once claimed it's gone for everyone, obtainable only by trade thereafter.
	Found  bool
	Unique bool
	Lore   string // a line of flavor for finds and legends (compendium / discovery)
	Hint   string // where a legend is rumored to lie (for /legends); Unique only
}

// Fists is the implicit weapon everyone always has: a light, melee, no-cost
// strike. BestWeapon falls back to it when the pack holds nothing better.
var Fists = Weapon{Item: "", Name: "bare hands", Damage: 1, Reach: 1}

// weapons is the full roster the combat code resolves against — craftable arms,
// hidden finds, and the unique legends. Craftable inputs are existing items, so
// they slot onto the recipe bench with no new forage plumbing; finds and legends
// have no recipe and are turned up in the world instead.
var weapons = []Weapon{
	// Craftable — built at the bench.
	{Item: "knife", Name: "Flint Knife", Damage: 2, Reach: 1, Cooldown: 1},
	{Item: "spear", Name: "Spear", Damage: 3, Reach: 1, Cooldown: 2},
	{Item: "bow", Name: "Hunter's Bow", Damage: 2, Reach: 4, Cooldown: 2, Ammo: "arrow"},
	{Item: "sword", Name: "Cast Blade", Damage: 4, Reach: 1, Cooldown: 2},
	// Found — hidden in the world, no recipe.
	{Item: "sling", Name: "Sling", Damage: 2, Reach: 3, Cooldown: 1, Ammo: "stone", Found: true,
		Lore: "A worn leather sling. Flings a gathered stone a fair way."},
	{Item: "dagger", Name: "Bone Dagger", Damage: 3, Reach: 1, Cooldown: 1, Found: true,
		Lore: "A wicked blade ground from old bone. Quick in the hand."},
	// Unique — one per world, hidden; trade-only once claimed.
	{Item: "durstbane", Name: "Durstbane", Damage: 6, Reach: 1, Cooldown: 2, Unique: true,
		Lore: "The blade that ended the long audit. There is only one.",
		Hint: "said to rest in the frozen heights, far to the cold north"},
	{Item: "skypiercer", Name: "Skypiercer", Damage: 4, Reach: 6, Cooldown: 2, Ammo: "arrow", Unique: true,
		Lore: "A bow strung with storm-sinew; its arrows never wander. The only one.",
		Hint: "lost deep in the old forest, where the canopy swallows the light"},
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

// Weapons returns the full weapon roster (for /wield listing, ownership-filtered
// by the caller).
func Weapons() []Weapon { return weapons }

// Artifacts returns the unique, one-per-world legendary weapons.
func Artifacts() []Weapon {
	var out []Weapon
	for _, wp := range weapons {
		if wp.Unique {
			out = append(out, wp)
		}
	}
	return out
}

// IsArtifact reports whether an item id names a unique legendary weapon.
func IsArtifact(item string) bool {
	wp, ok := weaponByItem[item]
	return ok && wp.Unique
}

// MatchWeapon resolves a user-typed token to a weapon by item id or by a
// case-insensitive prefix of its name ("flint" or "knife" → Flint Knife).
func MatchWeapon(s string) (Weapon, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if wp, ok := weaponByItem[s]; ok {
		return wp, true
	}
	for _, wp := range weapons {
		name := strings.ToLower(wp.Name)
		if name == s || strings.HasPrefix(name, s) || strings.Contains(name, s) {
			return wp, true
		}
	}
	return Weapon{}, false
}

// PvPAllowedAt reports whether a player at (area, x, y) may be struck: only out
// in the open Wilds — never in another area, never in the hub's peace ward, and
// never on a claimed homestead. The single source of truth shared by the strike
// action and the /pvp command (docs/WEAPON_PLAN.md).
func PvPAllowedAt(ctx *Ctx, area string, x, y int) bool {
	if area != "wilds" {
		return false
	}
	if worldgen.HubSafe(x, y) {
		return false
	}
	if ctx != nil && ctx.World != nil {
		if _, claimed := ctx.World.ClaimAt(x, y); claimed {
			return false
		}
	}
	return true
}

// IsWeapon reports whether an item id names a weapon.
func IsWeapon(item string) bool {
	_, ok := weaponByItem[item]
	return ok
}

// FistsKey is the /wield value that forces bare hands.
const FistsKey = "fists"

// WieldedWeapon resolves what the player actually fights with: their chosen
// weapon if they set one with /wield and it's still usable, otherwise the
// auto-picked best. An empty or no-longer-usable choice falls through to
// BestWeapon, so dropping or spending your pick never leaves you unarmed.
func WieldedWeapon(ctx *Ctx) Weapon {
	switch ctx.Wielded {
	case "":
		return BestWeapon(ctx.Inventory)
	case FistsKey:
		return Fists
	}
	wp, ok := weaponByItem[ctx.Wielded]
	if !ok || ctx.Inventory[wp.Item] <= 0 {
		return BestWeapon(ctx.Inventory)
	}
	if wp.Ammo != "" && ctx.Inventory[wp.Ammo] <= 0 {
		return BestWeapon(ctx.Inventory) // chosen bow, but out of arrows
	}
	return wp
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
