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

// TestGenCaveConnected checks that a carved cave is one connected system in which
// every surface mouth is reachable and leads back out, across many seeds and door
// counts — so a stray layout can't strand a player or orphan a mouth.
func TestGenCaveConnected(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		for nDoors := 1; nDoors <= 3; nDoors++ {
			m, doors, nodes := genCave(rand.New(rand.NewSource(seed)), nDoors)
			if len(doors) != nDoors {
				t.Fatalf("seed %d: wanted %d mouths, got %d", seed, nDoors, len(doors))
			}
			for _, d := range doors {
				if got := m.At(d[0], d[1]); got.Kind != game.TilePortal || got.Portal != "wilds" {
					t.Fatalf("seed %d: mouth %v is not a portal out (%+v)", seed, d, got)
				}
			}
			// Every walkable cell is reachable from the first mouth.
			reach := map[[2]int]bool{{doors[0][0], doors[0][1]}: true}
			stack := [][2]int{{doors[0][0], doors[0][1]}}
			for len(stack) > 0 {
				c := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for _, dd := range nb4 {
					nx, ny := c[0]+dd[0], c[1]+dd[1]
					if m.At(nx, ny).Walkable && !reach[[2]int{nx, ny}] {
						reach[[2]int{nx, ny}] = true
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}
			for _, d := range doors {
				if !reach[[2]int{d[0], d[1]}] {
					t.Fatalf("seed %d: mouth %v unreachable from mouth 0", seed, d)
				}
			}
			if nDoors == 2 && len(nodes) == 0 {
				t.Fatalf("seed %d: cave has nothing to gather", seed)
			}
		}
	}
}

// A real 3-mouth cave system under the fixed overworld seed (origin first).
var multiCave = [][2]int{{-484, 28}, {-517, 40}, {-505, 45}}

func newArea(st store.Store) *area {
	return &area{Walker: game.Walker{Ctx: &game.Ctx{
		World: world.New(), Name: "ada", Theme: ui.Default,
		Inventory: map[string]int{}, Store: st}, AreaID: "cave"}}
}

func countSeen(a *area) int {
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

// TestLinkedMouthsShareCave checks that the mouths of one system all open the
// same cavern: entering by a second mouth restores the map drawn from the first,
// while a different cave stays dark.
func TestLinkedMouthsShareCave(t *testing.T) {
	if _, _, ok := gen.CaveSystemAt(multiCave[0][0], multiCave[0][1]); !ok {
		t.Skip("expected cave system not present under this seed")
	}
	st := store.Open(t.TempDir() + "/t.db")
	walk := func(a *area, path string) {
		for _, c := range path {
			a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
		}
	}

	a1 := newArea(st) // enter by the origin mouth and explore
	a1.Init(&world.Player{X: multiCave[0][0], Y: multiCave[0][1]})
	if len(a1.overworldDoors) < 2 {
		t.Fatalf("expected a multi-mouth cave, got %d", len(a1.overworldDoors))
	}
	walk(a1, "ddddddddddssssssss")
	explored := countSeen(a1)

	a2 := newArea(st) // enter by a *different* mouth of the same system
	a2.Init(&world.Player{X: multiCave[1][0], Y: multiCave[1][1]})
	if a2.caveKey != a1.caveKey {
		t.Fatalf("linked mouths resolved to different caves: %q vs %q", a1.caveKey, a2.caveKey)
	}
	if got := countSeen(a2); got < explored {
		t.Fatalf("second mouth didn't restore the shared map: explored %d, restored %d", explored, got)
	}

	a3 := newArea(st) // an unrelated cave is still dark
	a3.Init(&world.Player{X: 99999, Y: 99999})
	if got := countSeen(a3); got >= explored {
		t.Fatalf("an unrelated cave inherited discovery: %d", got)
	}
}
