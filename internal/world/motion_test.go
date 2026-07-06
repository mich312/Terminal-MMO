package world

import "testing"

// Move records the motion trail the renderers read: the vacated tile, and
// whether the step was a dash (2 tiles). Doors and respawns leave no trail.
func TestMoveRecordsTrail(t *testing.T) {
	w := New()
	defer w.Close()
	n, _ := w.Join("ada")
	w.EnterArea(n, "lobby", 5, 5, "Lobby")

	w.Move(n, 7, 5) // a run collapses into one 2-tile Move
	p, _ := w.Self(n)
	if p.PrevX != 5 || p.PrevY != 5 || !p.Ran {
		t.Fatalf("dash trail = (%d,%d) ran=%v, want (5,5) true", p.PrevX, p.PrevY, p.Ran)
	}

	w.Move(n, 8, 5) // a plain step
	p, _ = w.Self(n)
	if p.PrevX != 7 || p.PrevY != 5 || p.Ran {
		t.Fatalf("walk trail = (%d,%d) ran=%v, want (7,5) false", p.PrevX, p.PrevY, p.Ran)
	}

	w.EnterArea(n, "arcade", 3, 3, "Arcade") // a door is not a dash
	p, _ = w.Self(n)
	if p.PrevX != 3 || p.PrevY != 3 || p.Ran {
		t.Fatalf("after a door: trail = (%d,%d) ran=%v, want (3,3) false", p.PrevX, p.PrevY, p.Ran)
	}
}
