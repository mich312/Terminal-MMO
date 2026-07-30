package game

import (
	"math"
	"testing"
	"time"
)

// openWorld is a walkability predicate with a set of blocked cells; everything
// else is open.
func openWorld(blocked ...[2]int) func(x, y int) bool {
	set := make(map[[2]int]bool, len(blocked))
	for _, b := range blocked {
		set[b] = true
	}
	return func(x, y int) bool { return !set[[2]int{x, y}] }
}

// wallColumn blocks a whole column of cells at x, tall enough to matter.
func wallColumn(x int) func(cx, cy int) bool {
	return func(cx, cy int) bool { return cx != x }
}

// drive walks a mover for d, re-asserting the intent every slice the way a live
// client does, and reports how many cells it crossed.
func drive(m *Mover, dx, dy float64, running bool, d, slice time.Duration, walk func(x, y int) bool) int {
	crossings := 0
	start := time.Now()
	for t := time.Duration(0); t < d; t += slice {
		now := start.Add(t)
		m.SetIntent(dx, dy, running, now)
		if _, crossed := m.Advance(slice, now, walk); crossed {
			crossings++
		}
	}
	return crossings
}

// A held intent moves the body at exactly the advertised speed. This is the
// number the whole feel of the game hangs off, so it is worth pinning.
func TestMoverWalksAtWalkSpeed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		running bool
		want    float64
	}{
		{"walk", false, WalkSpeed},
		{"run", true, RunSpeed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m Mover
			m.Place(5, 5)
			drive(&m, 1, 0, tc.running, time.Second, 20*time.Millisecond, openWorld())
			want := 5.5 + tc.want
			if math.Abs(m.FX-want) > 0.01 {
				t.Errorf("after 1s: FX = %.3f, want %.3f", m.FX, want)
			}
			if math.Abs(m.FY-5.5) > 1e-9 {
				t.Errorf("walking due east drifted in y: FY = %.3f, want 5.5", m.FY)
			}
		})
	}
}

// Crossing a cell boundary is the new "step": every per-step mechanic in the
// game (a cave lantern burning oil, the Wilds revealing ground and gathering
// what you walk over, a portal check) fires off it, so it must fire exactly
// once per cell entered — no misses, no doubles.
func TestMoverCrossesEachTileExactlyOnce(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	// One second at WalkSpeed=3 carries the body from 5.5 to 8.5: into 6, 7, 8.
	got := drive(&m, 1, 0, false, time.Second, 20*time.Millisecond, openWorld())
	if got != 3 {
		t.Errorf("crossed %d tiles walking from 5.5 to %.2f, want 3", got, m.FX)
	}
	if tx, _ := m.Tile(); tx != 8 {
		t.Errorf("ended on tile x=%d, want 8", tx)
	}
}

// Standing still crosses nothing, however long you wait.
func TestMoverIdleDoesNotCross(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	now := time.Now()
	for i := 0; i < 50; i++ {
		if moved, crossed := m.Advance(20*time.Millisecond, now, openWorld()); moved || crossed {
			t.Fatalf("idle mover reported moved=%v crossed=%v", moved, crossed)
		}
	}
}

// Walking straight into a wall stops the body and reports not-moved, so a walk
// cycle doesn't play on the spot.
func TestMoverStopsAtWall(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	drive(&m, 1, 0, false, 2*time.Second, 20*time.Millisecond, wallColumn(7))
	// The body's leading edge must stop short of the blocked column.
	if m.FX+BodyRadius >= 7 {
		t.Errorf("walked into the wall: FX = %.3f (edge %.3f), want edge < 7", m.FX, m.FX+BodyRadius)
	}
	if m.FX < 6 {
		t.Errorf("stopped far short of the wall: FX = %.3f, want just under 6.65", m.FX)
	}
	now := time.Now()
	m.SetIntent(1, 0, false, now)
	if moved, _ := m.Advance(20*time.Millisecond, now, wallColumn(7)); moved {
		t.Error("pressed against a wall still reported movement")
	}
}

// The point of resolving the axes separately: walking diagonally into a wall
// keeps the component along it. Without this, any approach that isn't square to
// the gap sticks, and free-angle movement feels like flypaper.
func TestMoverSlidesAlongWall(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	drive(&m, 1, 1, false, time.Second, 20*time.Millisecond, wallColumn(7))
	if m.FX+BodyRadius >= 7 {
		t.Errorf("slid into the wall: FX = %.3f", m.FX)
	}
	// Blocked in x, the body should still have travelled in y — most of the
	// second is spent sliding, at the diagonal's 1/√2 of full speed.
	if m.FY < 5.5+1.0 {
		t.Errorf("did not slide along the wall: FY = %.3f, want well past 6.5", m.FY)
	}
}

// An intent nobody re-asserts expires. This is what stops an SSH player, whose
// terminal never sends a key-up, from walking forever after letting go.
func TestMoverIntentExpires(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	start := time.Now()
	m.SetIntent(1, 0, false, start)

	// Still held, just before the deadline.
	if moved, _ := m.Advance(20*time.Millisecond, start.Add(IntentTTL-time.Millisecond), openWorld()); !moved {
		t.Error("intent went stale before IntentTTL")
	}
	// And past it.
	if moved, _ := m.Advance(20*time.Millisecond, start.Add(IntentTTL+time.Millisecond), openWorld()); moved {
		t.Error("intent outlived IntentTTL")
	}
	if m.Moving() {
		t.Error("expired intent left the body under power")
	}
}

// Stop halts immediately, without waiting for the deadline — the browser has
// real key-up events and should not have to.
func TestMoverStopIsImmediate(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	now := time.Now()
	m.SetIntent(1, 0, false, now)
	m.Stop()
	if m.Moving() {
		t.Fatal("Stop left the body under power")
	}
	if moved, _ := m.Advance(20*time.Millisecond, now, openWorld()); moved {
		t.Error("stopped body still moved")
	}
}

// A stalled tick — a laptop waking from sleep, a debugger pause — must not
// teleport the body through a wall, because Slide only tests the cell it lands
// in, not the ones it passed over.
func TestMoverClampsAHugeTick(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	now := time.Now()
	m.SetIntent(1, 0, false, now)
	m.Advance(10*time.Second, now, openWorld())
	if got := m.FX - 5.5; got > maxStep+1e-9 {
		t.Errorf("a 10s tick moved %.3f tiles, want at most %.2f", got, maxStep)
	}
}

// Place must leave lastTile truthful, or the next Advance reports a crossing
// into the cell the body is already standing in — and every per-step mechanic
// fires spuriously on arrival.
func TestMoverPlaceDoesNotFakeACrossing(t *testing.T) {
	var m Mover
	m.Place(5, 5)
	m.Place(40, 12) // a portal arrival, far away
	now := time.Now()
	m.SetIntent(1, 0, false, now)
	if _, crossed := m.Advance(time.Millisecond, now, openWorld()); crossed {
		t.Error("the first step after Place reported a tile crossing")
	}
}

// Slide's contract on its own, away from the Mover: each axis is independent.
func TestSlideResolvesAxesSeparately(t *testing.T) {
	walk := wallColumn(7)
	// Into the wall in x, free in y.
	gotX, gotY := Slide(walk, 6.6, 5.5, 0.2, 0.2, BodyRadius)
	if gotX != 6.6 {
		t.Errorf("x moved into a wall: %.3f, want 6.6", gotX)
	}
	if math.Abs(gotY-5.7) > 1e-9 {
		t.Errorf("y was blocked by a wall in x: %.3f, want 5.7", gotY)
	}
	// Both free.
	gotX, gotY = Slide(openWorld(), 6.6, 5.5, 0.2, 0.2, BodyRadius)
	if math.Abs(gotX-6.8) > 1e-9 || math.Abs(gotY-5.7) > 1e-9 {
		t.Errorf("open ground: got (%.3f, %.3f), want (6.8, 5.7)", gotX, gotY)
	}
}

// The body is wider than a point: it must not clip a corner its centre would
// pass beside.
func TestSlideRespectsBodyWidth(t *testing.T) {
	walk := openWorld([2]int{7, 5})
	// Centre would land at x=6.9, whose leading edge (7.25) is inside the
	// blocked cell even though the centre is not.
	gotX, _ := Slide(walk, 6.7, 5.5, 0.2, 0, BodyRadius)
	if gotX != 6.7 {
		t.Errorf("body edge clipped into a blocked cell: %.3f, want 6.7", gotX)
	}
}
