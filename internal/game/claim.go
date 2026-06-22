package game

import "github.com/durst-group/durstworld/internal/world"

// Land tenure & protected building (docs/CLAIMS_PLAN.md). Two tiers, one rule:
//   - Town plots: deed a settlement building (a Workspace Charter) → a named,
//     bounded parcel only you may build on.
//   - Open Wilds: your own structures project a small buffer others can't build
//     inside.
// Both lapse on owner absence and resolve through one predicate, BuildRight,
// which the build ghost and demolition consult. Movement is never affected —
// claims gate building, never walking. Voice stays corporate × medieval.

const (
	// parcelMargin is how many tiles of buildable ground a claimed building grants
	// around its footprint.
	parcelMargin = 2
	// wildsBuffer is the protected radius (Chebyshev) around your own structures in
	// the open Wilds.
	wildsBuffer = 3
	// leasePeriod is how long an owner can be absent before a claim lapses and the
	// plot becomes re-deedable (seconds). ~7 real days.
	leasePeriod int64 = 7 * 24 * 60 * 60
)

// ParcelBox is the build-rights region a claimed building grants: its footprint
// (anchor ax,ay with size w×h) grown by parcelMargin on every side.
func ParcelBox(ax, ay, w, h int) (minX, minY, maxX, maxY int) {
	return ax - parcelMargin, ay - parcelMargin, ax + w - 1 + parcelMargin, ay + h - 1 + parcelMargin
}

// ClaimActive reports whether a claim is still held — its owner has been present
// within the lease period.
func ClaimActive(c world.Claim, now int64) bool {
	return c.LastTouch > 0 && now-c.LastTouch <= leasePeriod
}

// BuildRight reports whether ctx's player may place or remove a structure at
// (x,y). Inside a live claim, only the holder may; inside another player's wilds
// buffer, only they may; everywhere else (open, unclaimed, or lapsed) anyone may.
// owner names the blocker when ok is false, for the toast/HUD.
func BuildRight(ctx *Ctx, x, y int) (ok bool, owner string) {
	now := nowUnix()
	// 1) A live town claim wins: its parcel is the holder's alone.
	if c, found := ctx.World.ClaimAt(x, y); found && ClaimActive(c, now) {
		if c.Owner == ctx.Name {
			return true, ""
		}
		return false, c.Owner
	}
	// 2) The open-Wilds proximity buffer: a foreign-owned structure nearby reserves
	// the ground around it.
	for _, p := range ctx.World.PlacementsNear(x, y, wildsBuffer) {
		if p.Owner != "" && p.Owner != ctx.Name {
			return false, p.Owner
		}
	}
	return true, ""
}

// ClaimWorkspace deeds the plot (its stable id and footprint) to ctx's player if
// it is unclaimed or has lapsed, snapshotting the parcel box so the claim stays
// self-describing. Returns false if another player holds it live.
func ClaimWorkspace(ctx *Ctx, plotID string, ax, ay, w, h int) bool {
	now := nowUnix()
	minX, minY, maxX, maxY := ParcelBox(ax, ay, w, h)
	c := world.Claim{PlotID: plotID, Owner: ctx.Name,
		MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY, LastTouch: now}
	return ctx.World.ClaimPlot(c, now-leasePeriod)
}

// ReleaseWorkspace gives up ctx's player's claim on a plot.
func ReleaseWorkspace(ctx *Ctx, plotID string) bool {
	return ctx.World.ReleaseClaim(plotID, ctx.Name)
}

// TouchWorkspace refreshes the lease on ctx's player's claim — call it while they
// stand in or build on their own parcel.
func TouchWorkspace(ctx *Ctx, plotID string) {
	ctx.World.TouchClaim(plotID, ctx.Name, nowUnix())
}

// WorkspaceAt returns the live claim covering (x,y) for the HUD, plus whether
// ctx's player owns it. ok is false if the cell is unclaimed or the claim lapsed.
func WorkspaceAt(ctx *Ctx, x, y int) (c world.Claim, mine, ok bool) {
	cl, found := ctx.World.ClaimAt(x, y)
	if !found || !ClaimActive(cl, nowUnix()) {
		return world.Claim{}, false, false
	}
	return cl, cl.Owner == ctx.Name, true
}
