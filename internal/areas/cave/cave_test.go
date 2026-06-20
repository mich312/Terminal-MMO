package cave

import (
	"math/rand"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
)

// TestGenCaveConnected checks that a carved cave is a single connected system
// reachable from the spawn (the cave mouth), that the mouth leads back out, and
// that it's stocked with gatherable life — across many seeds, so a stray layout
// can't strand a player.
func TestGenCaveConnected(t *testing.T) {
	for seed := int64(0); seed < 60; seed++ {
		m, sx, sy, nodes := genCave(rand.New(rand.NewSource(seed)))

		if got := m.At(sx, sy); got.Kind != game.TilePortal || got.Portal != "wilds" {
			t.Fatalf("seed %d: spawn is not the cave mouth back to the wilds (%+v)", seed, got)
		}

		// Flood-fill the walkable cells from the spawn and confirm every floor tile
		// in the map is reachable — no sealed-off chambers.
		reach := map[[2]int]bool{{sx, sy}: true}
		stack := [][2]int{{sx, sy}}
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, d := range nb4 {
				nx, ny := c[0]+d[0], c[1]+d[1]
				if m.At(nx, ny).Walkable && !reach[[2]int{nx, ny}] {
					reach[[2]int{nx, ny}] = true
					stack = append(stack, [2]int{nx, ny})
				}
			}
		}
		for y := 0; y < caveH; y++ {
			for x := 0; x < caveW; x++ {
				if m.At(x, y).Walkable && !reach[[2]int{x, y}] {
					t.Fatalf("seed %d: walkable cell (%d,%d) is unreachable from the mouth", seed, x, y)
				}
			}
		}

		if len(nodes) == 0 {
			t.Fatalf("seed %d: cave has no gatherable seams or mushrooms", seed)
		}
		// Every gatherable yields a known catalog item.
		for pos, item := range nodes {
			if _, ok := game.ItemByID(item); !ok {
				t.Fatalf("seed %d: node at %v yields unknown item %q", seed, pos, item)
			}
		}
	}
}
