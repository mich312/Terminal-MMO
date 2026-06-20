package cave

import (
	"math/rand"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// TestCaveDiscoveryPersists checks that fog-of-war is remembered per player and
// per cave: re-entering the same mouth restores what you'd uncovered, while a
// different mouth opens a cave that's still dark.
func TestCaveDiscoveryPersists(t *testing.T) {
	st := store.Open(t.TempDir() + "/t.db")
	newArea := func() *area {
		return &area{Walker: game.Walker{Ctx: &game.Ctx{
			World: world.New(), Name: "ada", Theme: ui.Default,
			Inventory: map[string]int{}, Store: st}, AreaID: "cave"}}
	}
	countSeen := func(a *area) int {
		n := 0
		for y := 0; y < caveH; y++ {
			for x := 0; x < caveW; x++ {
				if a.seen(x, y) {
					n++
				}
			}
		}
		return n
	}
	walk := func(a *area, path string) {
		for _, c := range path {
			a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
		}
	}

	a1 := newArea()
	a1.Init(&world.Player{X: 242, Y: -377})
	walk(a1, "ddddddddddssssssss")
	explored := countSeen(a1)

	a2 := newArea() // same mouth → discovery restored
	a2.Init(&world.Player{X: 242, Y: -377})
	if got := countSeen(a2); got < explored {
		t.Fatalf("re-entry lost discovery: explored %d, restored %d", explored, got)
	}

	a3 := newArea() // a different mouth → its own, still-dark cave
	a3.Init(&world.Player{X: 110, Y: -378})
	if got := countSeen(a3); got >= explored {
		t.Fatalf("a different cave inherited discovery: %d cells seen", got)
	}
}

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
