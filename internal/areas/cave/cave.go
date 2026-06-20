// Package cave is the underground: dark, torch-lit caverns that open off cave
// mouths in the overworld's hills. Each mouth always leads to the same cave
// (the layout is seeded by the entrance's world coordinates) and different
// mouths to different caves, so the hills are dotted with caverns to explore.
//
// A cave is a procedurally carved cellular-automaton cavern rendered through the
// shared Walker base (movement, portals, the HD pixel renderer) under a tight
// radial light, so you only see as far as your lantern throws.
package cave

import (
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

const (
	caveW, caveH = 60, 40 // cavern grid
	lanternR     = 9      // how far the lantern reaches into the dark
)

func init() {
	game.Register("cave", "a cave", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "cave"}}
	})
}

type area struct {
	game.Walker
}

func (a *area) Name() string { return "a cave" }

// Init carves the cavern. It is seeded by the overworld coordinates of the cave
// mouth the player stepped through (carried on the player at transition time), so
// a given mouth always opens onto the same cave.
func (a *area) Init(p *world.Player) tea.Cmd {
	seed := int64(uint64(uint32(p.X))*0x9E3779B1 ^ uint64(uint32(p.Y))*0x85EBCA77 ^ 0x0CA7E)
	var sx, sy int
	a.Map, sx, sy = genCave(rand.New(rand.NewSource(seed)))
	a.Enter(sx, sy, 0)
	return nil
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if portal, handled := a.HandleCommon(msg); handled && portal != "" {
		return game.Transition{To: portal}, nil
	}
	return a, nil
}

func (a *area) Hint() string {
	if h := a.PortalHint(); h != "" {
		return h
	}
	return "🕯 a cave — explore the dark · ◊ return to the mouth to leave"
}

// HDLight gives the HD renderer a lantern around the player so the cavern falls
// away into darkness past its reach.
func (a *area) HDLight() game.Light {
	return game.Light{X: a.X, Y: a.Y, Radius: lanternR}
}

func (a *area) View(width, height int) string {
	return a.RenderLit(width, height, lanternR)
}

// --- cavern generation ---------------------------------------------------------

var (
	rockWall  = game.Tile{Kind: game.TileWall, Ch: '▓', Walkable: false, Color: "#564E5E", Tex: game.TexRock, Ground: "#3A3442"}
	caveFloor = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Color: "#9A91A0", Tex: game.TexDirt, Ground: "#6A6270"}
	caveMouth = game.Tile{Kind: game.TilePortal, Ch: '◊', Walkable: true, Color: "#9BE0FF", Portal: "wilds", Label: "the cave mouth"}
)

// genCave carves a cellular-automaton cavern and returns the tilemap plus a spawn
// cell (on the cave mouth) inside the largest connected chamber. It retries a few
// times — with the rng it's handed — if the cavern comes out too cramped, so a
// cave is always a real space to wander, never a sealed pocket.
func genCave(rng *rand.Rand) (*game.TileMap, int, int) {
	for attempt := 0; attempt < 6; attempt++ {
		wall := carve(rng)
		region, sx, sy, ok := largestOpen(wall)
		if !ok || len(region) < caveW*caveH/6 {
			continue // too cramped — recarve
		}
		// Seal every open cell that isn't part of the main chamber, so stray
		// pockets don't read as unreachable gaps in the rock.
		inMain := make(map[[2]int]bool, len(region))
		for _, c := range region {
			inMain[c] = true
		}
		tiles := make([][]game.Tile, caveH)
		for y := 0; y < caveH; y++ {
			tiles[y] = make([]game.Tile, caveW)
			for x := 0; x < caveW; x++ {
				if wall[y][x] || !inMain[[2]int{x, y}] {
					tiles[y][x] = rockWall
				} else {
					tiles[y][x] = caveFloor
				}
			}
		}
		tiles[sy][sx] = caveMouth // the way back out, at the spawn
		return &game.TileMap{W: caveW, H: caveH, Tiles: tiles}, sx, sy
	}
	// Fallback: a plain open chamber (should effectively never happen).
	tiles := make([][]game.Tile, caveH)
	for y := 0; y < caveH; y++ {
		tiles[y] = make([]game.Tile, caveW)
		for x := 0; x < caveW; x++ {
			if x == 0 || y == 0 || x == caveW-1 || y == caveH-1 {
				tiles[y][x] = rockWall
			} else {
				tiles[y][x] = caveFloor
			}
		}
	}
	tiles[caveH/2][caveW/2] = caveMouth
	return &game.TileMap{W: caveW, H: caveH, Tiles: tiles}, caveW / 2, caveH / 2
}

// carve runs the cellular-automaton: random fill, then smoothing passes that turn
// a cell to rock when it's hemmed in by rock — coalescing the noise into rounded
// chambers and winding passages. The border is always solid rock.
func carve(rng *rand.Rand) [][]bool {
	wall := make([][]bool, caveH)
	for y := 0; y < caveH; y++ {
		wall[y] = make([]bool, caveW)
		for x := 0; x < caveW; x++ {
			if x == 0 || y == 0 || x == caveW-1 || y == caveH-1 {
				wall[y][x] = true
			} else {
				wall[y][x] = rng.Float64() < 0.45
			}
		}
	}
	for it := 0; it < 5; it++ {
		next := make([][]bool, caveH)
		for y := 0; y < caveH; y++ {
			next[y] = make([]bool, caveW)
			for x := 0; x < caveW; x++ {
				if x == 0 || y == 0 || x == caveW-1 || y == caveH-1 {
					next[y][x] = true
					continue
				}
				n := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						if wall[y+dy][x+dx] {
							n++
						}
					}
				}
				next[y][x] = n >= 5
			}
		}
		wall = next
	}
	return wall
}

// largestOpen flood-fills the open cells and returns the biggest connected
// chamber, with a spawn cell inside it (the cell nearest the chamber's centroid,
// so the mouth sits in open space rather than against a wall).
func largestOpen(wall [][]bool) (region [][2]int, sx, sy int, ok bool) {
	seen := make([][]bool, caveH)
	for y := range seen {
		seen[y] = make([]bool, caveW)
	}
	var best [][2]int
	for y := 1; y < caveH-1; y++ {
		for x := 1; x < caveW-1; x++ {
			if wall[y][x] || seen[y][x] {
				continue
			}
			var comp [][2]int
			stack := [][2]int{{x, y}}
			seen[y][x] = true
			for len(stack) > 0 {
				c := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				comp = append(comp, c)
				for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					nx, ny := c[0]+d[0], c[1]+d[1]
					if nx >= 0 && nx < caveW && ny >= 0 && ny < caveH && !wall[ny][nx] && !seen[ny][nx] {
						seen[ny][nx] = true
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}
			if len(comp) > len(best) {
				best = comp
			}
		}
	}
	if len(best) == 0 {
		return nil, 0, 0, false
	}
	// Spawn at the chamber cell nearest its centroid.
	var cx, cy int
	for _, c := range best {
		cx += c[0]
		cy += c[1]
	}
	cx, cy = cx/len(best), cy/len(best)
	bestD := 1 << 30
	sx, sy = best[0][0], best[0][1]
	for _, c := range best {
		d := (c[0]-cx)*(c[0]-cx) + (c[1]-cy)*(c[1]-cy)
		if d < bestD {
			bestD, sx, sy = d, c[0], c[1]
		}
	}
	return best, sx, sy, true
}
