package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/worldgen"
)

// TestForestTraversable measures whether the dense forest can actually be
// walked through: BFS the walkable (non-tree) cells with the 8-way movement the
// game allows, and report reachability + how dense the thickest stand gets.
func TestForestTraversable(t *testing.T) {
	g := worldgen.New(worldSeed)

	// Box over the big forest from the review shots (down-left of spawn).
	cx, cy := worldgen.GateX+30, worldgen.GateY+34
	const half = 35
	x0, y0, x1, y1 := cx-half, cy-half, cx+half, cy+half

	walk := func(x, y int) bool { return g.Walkable(x, y) }

	// Walkable fraction over the box, and over the densest 12x12 sub-window.
	total, open := 0, 0
	minOpen := 1.0
	for by := y0; by <= y1; by++ {
		for bx := x0; bx <= x1; bx++ {
			total++
			if walk(bx, by) {
				open++
			}
		}
	}
	for by := y0; by+12 <= y1; by += 4 {
		for bx := x0; bx+12 <= x1; bx += 4 {
			o := 0
			for dy := 0; dy < 12; dy++ {
				for dx := 0; dx < 12; dx++ {
					if walk(bx+dx, by+dy) {
						o++
					}
				}
			}
			if f := float64(o) / 144; f < minOpen {
				minOpen = f
			}
		}
	}

	// BFS 8-connected from the nearest walkable cell to centre.
	start := nearestWalkable(walk, cx, cy, half)
	seen := map[[2]int]bool{start: true}
	q := [][2]int{start}
	dirs := [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	touch := [4]bool{} // left, right, top, bottom of the box reached
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		if c[0] <= x0 {
			touch[0] = true
		}
		if c[0] >= x1 {
			touch[1] = true
		}
		if c[1] <= y0 {
			touch[2] = true
		}
		if c[1] >= y1 {
			touch[3] = true
		}
		for _, d := range dirs {
			n := [2]int{c[0] + d[0], c[1] + d[1]}
			if n[0] < x0 || n[0] > x1 || n[1] < y0 || n[1] > y1 || seen[n] || !walk(n[0], n[1]) {
				continue
			}
			seen[n] = true
			q = append(q, n)
		}
	}

	reach := len(seen)
	trapped := open - reach // walkable cells NOT reachable from centre (pockets)
	t.Logf("box %dx%d: walkable %.0f%% overall, densest 12x12 %.0f%% open",
		2*half+1, 2*half+1, 100*float64(open)/float64(total), 100*minOpen)
	t.Logf("BFS from centre reached %d of %d walkable cells (%.0f%%); %d trapped in pockets",
		reach, open, 100*float64(reach)/float64(open), trapped)
	t.Logf("reached box edges L=%v R=%v T=%v B=%v (can cross/exit)", touch[0], touch[1], touch[2], touch[3])

	if !(touch[0] && touch[1] && touch[2] && touch[3]) {
		t.Errorf("forest not crossable from centre in all directions: %v", touch)
	}
	if frac := float64(reach) / float64(open); frac < 0.85 {
		t.Errorf("only %.0f%% of walkable cells reachable from centre — too many trapped pockets", 100*frac)
	}
}

func nearestWalkable(walk func(x, y int) bool, cx, cy, r int) [2]int {
	for d := 0; d <= r; d++ {
		for dy := -d; dy <= d; dy++ {
			for dx := -d; dx <= d; dx++ {
				if walk(cx+dx, cy+dy) {
					return [2]int{cx + dx, cy + dy}
				}
			}
		}
	}
	return [2]int{cx, cy}
}
