package game

// Weapon-in-hand sprites (docs/WEAPON_PLAN.md). The HD renderer draws each
// player as a 12×12 pixel avatar; a wielded weapon is an overlay laid over that
// body the same way a hat is, so your arm shows on the character. The glyph
// client draws players as a single name token, so — like hats — weapons only
// read as sprites in HD.
//
// Overlays sit on the figure's right side (and mirror with the body when facing
// left). Transparent cells are SPACES (keep the body pixel); a '.' would punch a
// hole, so only spaces and the weapon codes K/k/G appear here.
//
// Codes (resolved in spritePixel): K steel/blade highlight, k wood/bone/haft,
// G a legendary glow accent.

// weaponShapeOf maps a weapon item id to its in-hand sprite shape. Several
// weapons share a silhouette (a knife and a dagger are both short blades); the
// legends get their own glowing variants so they read as special.
func weaponShapeOf(item string) string {
	switch item {
	case "knife", "dagger":
		return "blade_s"
	case "sword":
		return "blade"
	case "spear":
		return "polearm"
	case "bow":
		return "bow"
	case "sling":
		return "sling"
	case "durstbane":
		return "blade_legend"
	case "skypiercer":
		return "bow_legend"
	default:
		return ""
	}
}

// weaponSprites is the overlay art per shape, 12 rows of 12 columns.
var weaponSprites = map[string][]string{
	"blade_s": { // a short blade held at the side
		"            ",
		"            ",
		"            ",
		"            ",
		"          K ",
		"          K ",
		"         kKk",
		"          k ",
		"            ",
		"            ",
		"            ",
		"            ",
	},
	"blade": { // a full sword: blade up, crossguard, grip
		"          K ",
		"          K ",
		"          K ",
		"          K ",
		"          K ",
		"         kKk",
		"          k ",
		"          k ",
		"            ",
		"            ",
		"            ",
		"            ",
	},
	"polearm": { // a spear: long hafted shaft, tip up high
		"          K ",
		"          K ",
		"          k ",
		"          k ",
		"          k ",
		"          k ",
		"          k ",
		"          k ",
		"          k ",
		"          k ",
		"          k ",
		"            ",
	},
	"bow": { // a recurve bow belly facing out, string drawn
		"         k  ",
		"        k K ",
		"        k  K",
		"        k  K",
		"        k  K",
		"        k  K",
		"        k  K",
		"        k  K",
		"        k  K",
		"        k K ",
		"         k  ",
		"            ",
	},
	"sling": { // a small pouch on a cord
		"            ",
		"            ",
		"            ",
		"            ",
		"            ",
		"         K K",
		"          K ",
		"         kkk",
		"          k ",
		"            ",
		"            ",
		"            ",
	},
	"blade_legend": { // Durstbane: a grand blade, glowing
		"          G ",
		"          K ",
		"          K ",
		"          K ",
		"          K ",
		"         GKG",
		"          k ",
		"          k ",
		"            ",
		"            ",
		"            ",
		"            ",
	},
	"bow_legend": { // Skypiercer: a great bow with a glowing string
		"         k  ",
		"        k G ",
		"        k  G",
		"        k  K",
		"        k  K",
		"        k  G",
		"        k  G",
		"        k  K",
		"        k  K",
		"        k G ",
		"         k  ",
		"            ",
	},
}

// overlayWeapon lays a wielded weapon's sprite over the body rows, like an
// accessory: a space keeps the body pixel, anything else draws the weapon.
func overlayWeapon(rows []string, weapon string) []string {
	ov := weaponSprites[weaponShapeOf(weapon)]
	if len(ov) == 0 {
		return rows
	}
	out := make([]string, len(rows))
	copy(out, rows)
	for r := 0; r < len(ov) && r < len(out); r++ {
		dst := []rune(out[r])
		src := []rune(ov[r])
		for c := 0; c < len(src) && c < len(dst); c++ {
			if src[c] != ' ' {
				dst[c] = src[c]
			}
		}
		out[r] = string(dst)
	}
	return out
}
