package game

import (
	"math"
	"time"
)

/*
Continuous movement.

The world is still a grid and always will be: walls, placements, claims, cleared
ground, fog-of-war chunks and both terminal renderers are all keyed by the
integer cell. What stops being a grid is a body's position *within* it. A Mover
holds that position and integrates it from a steering intent, so a player walks
at whatever angle they're pointed and stops wherever they let go, instead of
teleporting between cell centres ten times a second.

The unit of gameplay stays the tile. Advance reports when the body crosses into
a new one, and everything that used to happen "per step" hangs off that: a cave
lantern burning oil, the Wilds revealing ground and gathering what you walk
over, a portal check, a maze's step counter. The mapping is exact rather than
approximate — under the old model a step was a tile, and it still is. What
changes is only that a step now takes a third of a second of walking rather
than being the atomic unit of input.
*/

const (
	// WalkSpeed and RunSpeed are tiles per second. The old model moved a tile
	// per input at ten inputs a second, i.e. 10 and 20 — a sprint and a
	// dead sprint, chosen because a discrete hop reads as slow even when it
	// isn't. A continuously moving body doesn't need the help.
	WalkSpeed = 3.0
	RunSpeed  = 5.5

	// IntentTTL is how long a steering intent stands before the body coasts to a
	// stop on its own.
	//
	// It exists for SSH. A terminal never sees a key-up — the client infers "still
	// held" from auto-repeat bytes continuing to arrive (cmd/durstworld/hd.go
	// heldTimeout, which this matches) — so the only safe contract is that an
	// intent expires unless it is re-asserted. A browser, which does get key-up,
	// simply sends a zero vector and stops immediately.
	//
	// It is also the worst-case overshoot past a released key. At WalkSpeed that
	// is three quarters of a tile; under the old 10-tiles-per-second pace the
	// same 250ms would have been two and a half.
	IntentTTL = 250 * time.Millisecond

	// maxStep bounds how far one Advance may carry a body, however long the
	// caller says dt was. A stalled tick, a laptop waking from sleep or a
	// debugger pause would otherwise hand us a dt of seconds and teleport the
	// player through a wall — Slide only tests the cell it lands in, not the
	// ones it passed over.
	maxStep = 0.5
)

// Mover is a body's continuous position and the intent driving it. Areas embed
// it (see Walker) rather than keeping their own int coordinates.
//
// It owns no locking and no world state: an area advances its own Mover on its
// own goroutine and pushes the result to world.MoveTo.
type Mover struct {
	FX, FY float64 // body centre, in tiles
	Angle  float64 // heading in radians, as world.FacingAngle counts them

	intentX, intentY float64 // unit steering vector; zero means standing still
	running          bool
	expires          time.Time // when the intent goes stale
	lastTile         [2]int    // the cell Advance last reported crossing into
}

// SetIntent points the body. (dx,dy) is a steering vector in world axes — x
// east, y south — and need not be normalized; a zero vector stops. The intent
// stands for IntentTTL unless refreshed before then.
//
// The heading is taken from the intent rather than from the resulting movement
// on purpose. A body sliding along a wall has a frame-to-frame delta that swings
// wildly as the blocked component is dropped and restored, and turning to follow
// it would make the avatar shimmy against every fence it brushed. Where you are
// steering is also simply what you mean by which way you are facing.
func (m *Mover) SetIntent(dx, dy float64, running bool, now time.Time) {
	d := math.Hypot(dx, dy)
	if d < 1e-9 {
		m.intentX, m.intentY = 0, 0
		return
	}
	m.intentX, m.intentY = dx/d, dy/d
	m.running = running
	m.expires = now.Add(IntentTTL)
	m.Angle = math.Atan2(dx, dy)
}

// Stop halts the body without waiting for the intent to expire — a key-up, a
// knock-out, a panel opening over the view.
func (m *Mover) Stop() {
	m.intentX, m.intentY = 0, 0
}

// Moving reports whether the body is under power, for walk cycles and for
// deciding whether a frame is worth sending.
func (m *Mover) Moving() bool {
	return m.intentX != 0 || m.intentY != 0
}

// Running reports whether the current intent is a run.
func (m *Mover) Running() bool {
	return m.running && m.Moving()
}

// Tile is the cell the body's centre stands in — the integer coordinate every
// grid-shaped thing downstream keeps using.
func (m *Mover) Tile() (int, int) {
	return int(math.Floor(m.FX)), int(math.Floor(m.FY))
}

// Place puts the body in the middle of a cell and drops any intent: spawning,
// arriving through a portal, respawning, being knocked back. Callers that place
// a body must use this rather than assigning FX/FY, so lastTile stays truthful
// and the next Advance doesn't report a spurious crossing.
func (m *Mover) Place(x, y int) {
	m.FX, m.FY = float64(x)+0.5, float64(y)+0.5
	m.lastTile = [2]int{x, y}
	m.Stop()
}

// Advance integrates dt of movement against walk, the same "is this cell open"
// predicate CanStep takes.
//
// moved reports that the body actually shifted — false when standing still, and
// false when walking straight into a wall, so a walk cycle doesn't play on the
// spot. crossed reports that it ended the step in a different cell than it
// started in: that is the event every per-step mechanic hangs off, and it fires
// exactly once per cell entered.
func (m *Mover) Advance(dt time.Duration, now time.Time, walk func(x, y int) bool) (moved, crossed bool) {
	if !m.Moving() {
		return false, false
	}
	if now.After(m.expires) {
		// Nobody re-asserted the intent: the key was released and the client
		// never said so, or the client went away mid-stride. Either way, stop —
		// the alternative is a body that walks until it hits something.
		m.Stop()
		return false, false
	}
	speed := WalkSpeed
	if m.running {
		speed = RunSpeed
	}
	d := math.Min(speed*dt.Seconds(), maxStep)
	if d <= 0 {
		return false, false
	}

	fx, fy := m.FX, m.FY
	m.FX, m.FY = Slide(walk, fx, fy, m.intentX*d, m.intentY*d, BodyRadius)
	if m.FX == fx && m.FY == fy {
		return false, false // hard against a wall
	}
	tx, ty := m.Tile()
	if tx != m.lastTile[0] || ty != m.lastTile[1] {
		m.lastTile = [2]int{tx, ty}
		return true, true
	}
	return true, false
}
