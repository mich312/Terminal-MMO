package game

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// The point of the whole exercise: a direction that is not one of eight has to
// survive from the client all the way to where the body ends up. Walk at a
// shallow angle and the body must actually go that way — under the old model
// every one of these would have been rounded onto a compass point.
func TestSteerWalksOffTheCompass(t *testing.T) {
	for _, deg := range []float64{10, 20, 35, 67, 100, 155, 200, 260, 310, 350} {
		t.Run(fmt.Sprintf("%.0fdeg", deg), func(t *testing.T) {
			w := newWalker(t, openMap(80, 80))
			w.Enter(40, 40, 0)
			// Heading as world.FacingAngle counts them: 0 south, growing east.
			rad := deg * math.Pi / 180
			dx, dy := math.Sin(rad), math.Cos(rad)

			w.HandleCommon(SteerMsg{DX: dx, DY: dy})
			if !w.Body.Moving() {
				t.Fatal("a steer vector set no intent")
			}
			w.WalkFor(dx, dy, false, time.Second)

			// Where we ended up, as an angle from where we started.
			gotX, gotY := w.Body.FX-40.5, w.Body.FY-40.5
			if math.Hypot(gotX, gotY) < 1 {
				t.Fatalf("barely moved: (%.2f, %.2f)", gotX, gotY)
			}
			got := math.Atan2(gotX, gotY)
			diff := math.Abs(math.Atan2(math.Sin(got-rad), math.Cos(got-rad)))
			if diff > 0.02 {
				t.Errorf("steered %.0f° but travelled %.1f°",
					deg, got*180/math.Pi)
			}
			// …and the eighth-of-a-turn facing follows it, for the terminal
			// and for the combat code, which both still think in eights.
			self, _ := w.Ctx.World.Self(w.Ctx.Name)
			if want := world.FacingAngle(rad); self.Facing != want {
				t.Errorf("facing %d, want %d for %.0f°", self.Facing, want, deg)
			}
		})
	}
}

// A zero vector is how a browser says "I let go" — it must stop the body at
// once rather than leaving it to time out.
func TestSteerZeroStops(t *testing.T) {
	w := newWalker(t, openMap(40, 20))
	w.Enter(5, 5, 0)
	w.HandleCommon(SteerMsg{DX: 1})
	if !w.Body.Moving() {
		t.Fatal("steer did not start the body")
	}
	w.HandleCommon(SteerMsg{})
	if w.Body.Moving() {
		t.Error("a zero steer vector left the body walking")
	}
}

// The vector need not arrive normalized — a client summing key presses will
// hand over things like (1,1) — and its length must not become a speed.
func TestSteerLengthIsNotSpeed(t *testing.T) {
	var far, near float64
	for _, scale := range []float64{1, 25} {
		w := newWalker(t, openMap(80, 80))
		w.Enter(40, 40, 0)
		w.WalkFor(scale, 0, false, time.Second)
		if scale == 1 {
			near = w.Body.FX
		} else {
			far = w.Body.FX
		}
	}
	if math.Abs(far-near) > 0.01 {
		t.Errorf("a longer vector walked further: %.3f vs %.3f", far, near)
	}
}

// Running is the run speed, whichever way it is expressed.
func TestSteerRunIsFaster(t *testing.T) {
	walk := newWalker(t, openMap(80, 80))
	walk.Enter(40, 40, 0)
	walk.WalkFor(1, 0, false, time.Second)

	run := newWalker(t, openMap(80, 80))
	run.Enter(40, 40, 0)
	run.WalkFor(1, 0, true, time.Second)

	if run.Body.FX <= walk.Body.FX {
		t.Errorf("running (%.2f) got no further than walking (%.2f)", run.Body.FX, walk.Body.FX)
	}
	if got := run.Body.FX - 40.5; math.Abs(got-RunSpeed) > 0.05 {
		t.Errorf("a second of running covered %.2f tiles, want %.2f", got, RunSpeed)
	}
}
