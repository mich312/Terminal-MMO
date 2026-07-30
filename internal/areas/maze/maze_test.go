package maze

import (
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

func newArea(t *testing.T) *area {
	t.Helper()
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("tester")
	ctx := &game.Ctx{World: w, Name: name}
	a := &area{Walker: game.Walker{Ctx: ctx, AreaID: "maze"}}
	self, _ := w.Self(name)
	a.Init(&self)
	return a
}

// A freshly carved maze has the right dimensions, a walkable entrance and exit,
// a door back to the Arcade, and — being a perfect maze — every passage cell is
// reachable from the entrance (so the exit always is too).
func TestGenerateConnected(t *testing.T) {
	a := newArea(t)
	wantW, wantH := 2*a.cw+1, 2*a.ch+1
	if a.Map.W != wantW || a.Map.H != wantH {
		t.Fatalf("maze %dx%d, want %dx%d", a.Map.W, a.Map.H, wantW, wantH)
	}
	if !a.Map.Walkable(1, 1) {
		t.Fatal("entrance (1,1) is not walkable")
	}
	if !a.Map.Walkable(a.goal[0], a.goal[1]) {
		t.Fatal("exit is not walkable")
	}
	if d := a.Map.At(1, 0); d.Kind != game.TilePortal || d.Portal != "arcade" {
		t.Fatalf("door tile = %+v, want an arcade portal", d)
	}

	// BFS the walkable graph from the entrance; the exit must be reached.
	seen := map[[2]int]bool{{1, 1}: true}
	queue := [][2]int{{1, 1}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := [2]int{p[0] + d[0], p[1] + d[1]}
			if !seen[n] && a.Map.Walkable(n[0], n[1]) {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	if !seen[a.goal] {
		t.Fatalf("exit %v unreachable from the entrance", a.goal)
	}
}

// Reaching the exit clears the maze, grows it and carves a fresh one.
func TestReachingExitRegenerates(t *testing.T) {
	a := newArea(t)
	startW, startH := a.cw, a.ch
	gx, gy := a.goal[0], a.goal[1]

	// Stand on a passage cell next to the exit and walk onto it. Movement is
	// continuous now, so this is a duration rather than a keystroke: a little
	// over the third of a second one tile takes at game.WalkSpeed, but well
	// short of the two thirds that would carry us past the exit cell.
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	moved := false
	for _, d := range dirs {
		nx, ny := gx-d[0], gy-d[1]
		if a.Map.Walkable(nx, ny) {
			a.Body.Place(nx, ny)
			a.X, a.Y = nx, ny
			a.Ctx.World.Move(a.Ctx.Name, nx, ny)
			// Walk in short bursts and stop the moment the exit is reached.
			// Holding the key through it would be legitimate — you are still
			// pressing a direction, so you walk on from the new entrance — but
			// then this test would be asserting where that walk got to rather
			// than where regenerating put you.
			for i := 0; i < 20 && a.solved == 0; i++ {
				a.WalkFor(float64(d[0]), float64(d[1]), false, 50*time.Millisecond)
			}
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("exit had no walkable neighbour to approach from")
	}
	if a.solved != 1 {
		t.Fatalf("solved=%d want 1 after reaching the exit", a.solved)
	}
	if a.cw <= startW || a.ch <= startH {
		t.Fatalf("maze did not grow: %dx%d (was %dx%d)", a.cw, a.ch, startW, startH)
	}
	// A new board was carved: the player is back at the entrance.
	if a.X != 1 || a.Y != 1 {
		t.Fatalf("after solving, player at (%d,%d) want entrance (1,1)", a.X, a.Y)
	}
}
