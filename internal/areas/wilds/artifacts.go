package wilds

import (
	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// Hidden legends — where each unique weapon lies (docs/WEAPON_PLAN.md). The spot
// is a pure function of the world (the fixed overworld seed), so every session
// agrees on it and players can race for the same blade; the world's artifact
// registry then decides who actually claims it, once. Each legend is themed to a
// biome and a rough quarter of the map matching its rumor, so the /legends hint
// actually points you the right way.
type artifactSite struct {
	id    string           // weapon item id (game.Artifacts)
	biome worldgen.Biome   // terrain it hides in
	ax, ay int             // search anchor (a quarter of the map out)
}

var artifactSites = []artifactSite{
	{id: "durstbane", biome: worldgen.Snow, ax: 0, ay: -240},    // the frozen north
	{id: "skypiercer", biome: worldgen.Forest, ax: 200, ay: 150}, // the deep old forest
}

// artifactSearchR bounds the ring search for a themed, standable cell around an
// anchor before falling back to any open ground, so placement always succeeds
// and Init never spins.
const artifactSearchR = 360

// computeArtifactCells resolves every legend's hidden cell once, caching the
// cell↔id maps the renderer and pickup read. Deterministic: same seed → same
// spots for everyone.
func (a *area) computeArtifactCells() {
	a.artifactCell = map[string][2]int{}
	a.artifactAtCell = map[[2]int]string{}
	for _, s := range artifactSites {
		x, y, ok := a.nearestStandable(s.ax, s.ay, s.biome)
		if !ok {
			continue
		}
		a.artifactCell[s.id] = [2]int{x, y}
		a.artifactAtCell[[2]int{x, y}] = s.id
	}
}

// nearestStandable scans outward from (ax,ay) for the closest cell of the wanted
// biome the player can stand on (and that isn't a portal). If the biome never
// turns up within the search radius it falls back to the nearest open ground, so
// a legend always has a home.
func (a *area) nearestStandable(ax, ay int, biome worldgen.Biome) (int, int, bool) {
	fbx, fby, haveFB := 0, 0, false
	for r := 0; r <= artifactSearchR; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if r > 0 && dx != -r && dx != r && dy != -r && dy != r {
					continue // ring only
				}
				x, y := ax+dx, ay+dy
				if !a.fits(x, y) {
					continue
				}
				if _, isPortal := a.portalUnder(x, y); isPortal {
					continue
				}
				if a.gen.At(x, y).Biome == biome {
					return x, y, true
				}
				if !haveFB {
					fbx, fby, haveFB = x, y, true // remember the first open ground seen
				}
			}
		}
	}
	if haveFB {
		return fbx, fby, true
	}
	return 0, 0, false
}

// artifactUnclaimed reports whether the legend with this id is still out in the
// world (not yet discovered by anyone).
func (a *area) artifactUnclaimed(id string) bool {
	_, claimed := a.ctx.World.ArtifactClaimed(id)
	return !claimed
}

// artifactUnderBody returns the unclaimed legend lying under the player's
// footprint, if any.
func (a *area) artifactUnderBody() (game.Weapon, int, int, bool) {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			x, y := a.wx+dx, a.wy+dy
			id, ok := a.artifactAtCell[[2]int{x, y}]
			if !ok || !a.artifactUnclaimed(id) {
				continue
			}
			if wp, ok := game.WeaponByItem(id); ok {
				return wp, x, y, true
			}
		}
	}
	return game.Weapon{}, 0, 0, false
}

// claimArtifact takes a legend for the player: the world decides the claim
// atomically (first finder wins), then it drops into the pack as a normal item —
// so from here it persists, shows, and trades like anything else, but can never
// be found in the world again.
func (a *area) claimArtifact(wp game.Weapon) {
	if !a.ctx.World.ClaimArtifact(wp.Item, a.ctx.Name) {
		a.setToast("the " + wp.Name + " is already gone — someone got here first")
		return
	}
	a.ctx.Inventory[wp.Item]++
	a.ctx.Store.AddItem(a.ctx.Name, wp.Item)
	a.setToast("✦ you claim " + wp.Name + " — a legend, and it's yours alone!")
}
