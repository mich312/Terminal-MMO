package world

import (
	"math"
	"testing"
)

// There are now three ways to name a direction: an integer delta (Facing8), a
// continuous heading (FacingAngle), and the browser's own quantization of a
// camera vector (internal/web/static/js/input.js dirIndex, which is the same
// atan2(dx, dy) rounded to eighths). They have to agree, or a player walks one
// way and their body faces another — which is exactly the bug that shipped in
// the browser's facing table, where the angles were negated and east and west
// came out mirrored.
//
// This pins the agreement at all eight compass points, including the diagonals.
func TestFacingAngleAgreesWithFacing8(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dx, dy int
		want   Dir
	}{
		{"S", 0, 1, DirS},
		{"SE", 1, 1, DirSE},
		{"E", 1, 0, DirE},
		{"NE", 1, -1, DirNE},
		{"N", 0, -1, DirN},
		{"NW", -1, -1, DirNW},
		{"W", -1, 0, DirW},
		{"SW", -1, 1, DirSW},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Facing8(tc.dx, tc.dy); got != tc.want {
				t.Errorf("Facing8(%d, %d) = %d, want %d", tc.dx, tc.dy, got, tc.want)
			}
			a := math.Atan2(float64(tc.dx), float64(tc.dy))
			if got := FacingAngle(a); got != tc.want {
				t.Errorf("FacingAngle(atan2(%d, %d)) = %d, want %d", tc.dx, tc.dy, got, tc.want)
			}
		})
	}
}

// DirAngle is FacingAngle's inverse, so a round trip through it must land back
// where it started for every facing.
func TestDirAngleRoundTrips(t *testing.T) {
	for d := DirS; d <= DirSW; d++ {
		if got := FacingAngle(DirAngle(d)); got != d {
			t.Errorf("FacingAngle(DirAngle(%d)) = %d, want %d", d, got, d)
		}
		if a := DirAngle(d); a <= -math.Pi || a > math.Pi {
			t.Errorf("DirAngle(%d) = %.4f, want it folded into (-π, π]", d, a)
		}
	}
}

// Headings between the compass points must round to the nearer one, and the
// wrap at ±π must not fall through a crack: due north is the seam.
func TestFacingAngleRoundsAndWraps(t *testing.T) {
	const eighth = math.Pi / 4
	for _, tc := range []struct {
		name string
		a    float64
		want Dir
	}{
		{"just east of south", 0.1, DirS},
		{"just short of southeast", eighth - 0.01, DirSE},
		{"just past southeast", eighth + 0.01, DirSE},
		{"north from below", math.Pi - 0.01, DirN},
		{"north from above", -math.Pi + 0.01, DirN},
		{"exactly north", math.Pi, DirN},
		{"two turns round", 2*math.Pi + eighth*2, DirE},
		{"a turn the other way", -2*math.Pi + eighth*2, DirE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FacingAngle(tc.a); got != tc.want {
				t.Errorf("FacingAngle(%.4f) = %d, want %d", tc.a, got, tc.want)
			}
		})
	}
}

// MoveTo is the continuous path; Move is the grid one. Whichever a caller uses,
// the integer cell and the continuous body must agree afterwards — everything
// downstream reads X, Y and would silently disagree with the browser otherwise.
func TestPositionRepresentationsStayInStep(t *testing.T) {
	w := New()
	defer w.Close()
	name, _ := w.Join("ada")
	w.EnterArea(name, "wilds", 10, 20, "Wilds")

	self, _ := w.Self(name)
	if self.X != 10 || self.Y != 20 {
		t.Fatalf("EnterArea put us at (%d,%d), want (10,20)", self.X, self.Y)
	}
	if self.FX != 10.5 || self.FY != 20.5 {
		t.Errorf("EnterArea left the body at (%.2f,%.2f), want the cell centre (10.5,20.5)", self.FX, self.FY)
	}

	w.Move(name, 11, 20)
	self, _ = w.Self(name)
	if self.FX != 11.5 || self.FY != 20.5 {
		t.Errorf("Move left the body at (%.2f,%.2f), want (11.5,20.5)", self.FX, self.FY)
	}
	if self.Facing != DirE {
		t.Errorf("Move east set facing %d, want DirE", self.Facing)
	}

	// A body three quarters of the way across a cell still belongs to that cell.
	w.MoveTo(name, 14.75, 20.25, math.Pi)
	self, _ = w.Self(name)
	if self.X != 14 || self.Y != 20 {
		t.Errorf("MoveTo(14.75, 20.25) reported cell (%d,%d), want (14,20)", self.X, self.Y)
	}
	if self.Facing != DirN {
		t.Errorf("MoveTo with a heading of π set facing %d, want DirN", self.Facing)
	}

	// And negative coordinates must floor, not truncate toward zero — the
	// overworld runs in every direction from the origin.
	w.MoveTo(name, -0.25, -3.5, 0)
	self, _ = w.Self(name)
	if self.X != -1 || self.Y != -4 {
		t.Errorf("MoveTo(-0.25, -3.5) reported cell (%d,%d), want (-1,-4)", self.X, self.Y)
	}
}
