package game

import "github.com/durst-group/durstworld/internal/world"

// Cleared terrain — the policy half of the clearing overlay (docs/BUILD_TOOLS_PLAN.md).
// The world stores cleared cells with a last-touch clock; this layer owns the
// regrowth rule: a cleared cell stays clear while it's tended, and grows back
// once the owner has been absent past clearLease. A cell is "active" (still
// cleared) only while within the lease — render and collision treat a lapsed
// cell as the original terrain, and ground regrows lazily on next touch.

// clearLease is how long a cleared cell survives untended before it regrows
// (seconds). Generous — a real fortnight — so a lived-in homestead stays open.
const clearLease int64 = 14 * 24 * 60 * 60

// ClearedActive reports whether a cleared record is still in force (its owner has
// touched it within the lease).
func ClearedActive(c world.Cleared, now int64) bool {
	return c.LastTouch > 0 && now-c.LastTouch <= clearLease
}

// IsCleared reports whether (x,y) is currently cleared ground (a live record).
// Cheap single-cell query for collision and the build check.
func IsCleared(ctx *Ctx, x, y int) bool {
	c, ok := ctx.World.ClearedAt(x, y)
	return ok && ClearedActive(c, nowUnix())
}

// ActiveClearedSet returns the set of currently-cleared cells in the box, for the
// renderer to overlay a window in one pass.
func ActiveClearedSet(ctx *Ctx, minX, minY, maxX, maxY int) map[[2]int]bool {
	now := nowUnix()
	var out map[[2]int]bool
	for _, c := range ctx.World.ClearedOverlapping(minX, minY, maxX, maxY) {
		if ClearedActive(c, now) {
			if out == nil {
				out = map[[2]int]bool{}
			}
			out[[2]int{c.X, c.Y}] = true
		}
	}
	return out
}

// ClearGround records (x,y) as cleared by ctx's player, if build-rights allow it
// (your land or open frontier). The caller checks the terrain is clearable and
// pays out the yield (Step C); this just writes the overlay. Returns ok.
func ClearGround(ctx *Ctx, x, y int) bool {
	if ok, _ := BuildRight(ctx, x, y); !ok {
		return false
	}
	ctx.World.ClearCell(world.Cleared{X: x, Y: y, Owner: ctx.Name, LastTouch: nowUnix()})
	return true
}

// TouchCleared refreshes the lease on a cleared cell the player is using, so a
// lived-in clearing never regrows under them.
func TouchCleared(ctx *Ctx, x, y int) {
	ctx.World.TouchCleared(x, y, ctx.Name, nowUnix())
}
