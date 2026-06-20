// Package cave is the underground: dark, bioluminescent caverns that open off
// cave mouths in the overworld's hills. Each mouth always leads to the same cave
// (the layout is seeded by the entrance's world coordinates) and different mouths
// to different caves, so the hills are dotted with caverns to explore.
//
// A cave is a procedurally carved cellular-automaton cave system — rounded
// chambers joined by winding passages — rendered through the shared Walker base
// (movement, the HD pixel renderer) under a tight lantern, so you only see as far
// as your light throws. The dark is broken by the cave's own life: clusters of
// glowing mushrooms, still pools lit from within, and seams of ice crystal that
// twinkle their own cold light — all of which you can mine or gather.
package cave

import (
	"math/rand"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
)

const (
	caveW, caveH = 96, 60 // cavern grid — a sprawling system, not one room
	lanternR     = 10     // how far the lantern reaches into the dark
)

func init() {
	game.Register("cave", "a cave", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "cave"}}
	})
}

type area struct {
	game.Walker
	nodes      map[[2]int]string // gatherable position → item id
	mined      map[[2]int]bool   // worked out this visit
	toast      string
	toastUntil time.Time
}

func (a *area) Name() string { return "a cave" }

// Init carves the cavern, seeded by the overworld coordinates of the cave mouth
// the player stepped through (carried on the player at transition time), so a
// given mouth always opens onto the same cave.
func (a *area) Init(p *world.Player) tea.Cmd {
	if a.Ctx.Inventory == nil {
		a.Ctx.Inventory = map[string]int{}
	}
	seed := int64(uint64(uint32(p.X))*0x9E3779B1 ^ uint64(uint32(p.Y))*0x85EBCA77 ^ 0x0CA7E)
	var sx, sy int
	a.Map, sx, sy, a.nodes = genCave(rand.New(rand.NewSource(seed)))
	a.mined = map[[2]int]bool{}
	a.Enter(sx, sy, 0)
	return nil
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && (key.String() == "e" || key.String() == " ") {
		if pos, item, ok := a.nodeNear(); ok {
			a.gather(pos, item)
		}
		return a, nil
	}
	if portal, handled := a.HandleCommon(msg); handled && portal != "" {
		return game.Transition{To: portal}, nil
	}
	return a, nil
}

// nodeNear returns the first ungathered seam or mushroom on or one tile around
// the player.
func (a *area) nodeNear() ([2]int, string, bool) {
	for dy := -1; dy <= game.PlayerH; dy++ {
		for dx := -1; dx <= game.PlayerW; dx++ {
			p := [2]int{a.X + dx, a.Y + dy}
			if item, ok := a.nodes[p]; ok && !a.mined[p] {
				return p, item, true
			}
		}
	}
	return [2]int{}, "", false
}

// gather works out a seam or picks a mushroom: it drops into the player's pack,
// the spot becomes plain cave floor, and a toast confirms the haul.
func (a *area) gather(pos [2]int, item string) {
	a.mined[pos] = true
	a.Map.Tiles[pos[1]][pos[0]] = caveFloor
	a.Ctx.Inventory[item]++
	a.Ctx.Store.AddItem(a.Ctx.Name, item)
	name := item
	if it, ok := game.ItemByID(item); ok {
		name = it.Name
	}
	verb := "⛏ mined"
	if item == "mushroom" {
		verb = "🍄 picked"
	}
	a.setToast(verb + " " + name)
}

func (a *area) setToast(s string) { a.toast, a.toastUntil = s, time.Now().Add(3*time.Second) }

// Toast implements game.Toaster so both renderers surface the gathering message.
func (a *area) Toast() (string, bool) {
	return a.toast, a.toast != "" && time.Now().Before(a.toastUntil)
}

func (a *area) Hint() string {
	if _, item, ok := a.nodeNear(); ok {
		name := item
		if it, ok := game.ItemByID(item); ok {
			name = it.Name
		}
		verb := "mine"
		if item == "mushroom" {
			verb = "pick"
		}
		return "e — " + verb + " the " + name
	}
	if h := a.PortalHint(); h != "" {
		return h
	}
	return "🕯 a cave — follow the glow into the dark · ∩ return to the mouth to leave"
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
	// The mouth is a cave-mouth sprite (not a glowing gate); the warm hex is the
	// daylight beyond it. Its prop is kept by Walker.HDView instead of the portal.
	caveMouth = game.Tile{Kind: game.TilePortal, Ch: '∩', Walkable: true, Color: "#C8BFA0",
		Portal: "wilds", Label: "the cave mouth", Prop: game.PropCaveMouth, PropHex: "#B6A483", Tex: game.TexRock, Ground: "#6B5A44"}
	mushroom = game.Tile{Kind: game.TileObject, Ch: 'ψ', Walkable: true, Color: "#7CF2C4",
		Tex: game.TexDirt, Ground: "#6A6270", Prop: game.PropCaveShroom, PropHex: "#7CF2C4"}
	glowPool = game.Tile{Kind: game.TileFloor, Ch: '≈', Walkable: true, Color: "#5BD8E0",
		Tex: game.TexWater, Ground: "#1E5560", Prop: game.PropGlowPool, PropHex: "#6CE0E6"}
)

var nb4 = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// seam is one mineable mineral: the item it yields and the tile that marks it in
// the rock. Stone is common, gold rarer, glittering ice crystals (which twinkle
// their own light in the dark) rarest.
type seam struct {
	item string
	tile game.Tile
}

var seams = []seam{
	{"stone", game.Tile{Kind: game.TileObject, Ch: '◊', Walkable: true, Color: "#C2C8D0", Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropStone, PropHex: "#C2C8D0"}},
	{"stone", game.Tile{Kind: game.TileObject, Ch: '◊', Walkable: true, Color: "#C2C8D0", Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropStone, PropHex: "#C2C8D0"}},
	{"nugget", game.Tile{Kind: game.TileObject, Ch: '◆', Walkable: true, Color: "#FFC861", Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropGem, PropHex: "#FFC861"}},
	{"crystal", game.Tile{Kind: game.TileObject, Ch: '◆', Walkable: true, Color: "#7DF0FF", Tex: game.TexRock, Ground: "#6A6270", Prop: game.PropGemGlow, PropHex: "#7DF0FF"}},
}

func genCave(rng *rand.Rand) (*game.TileMap, int, int, map[[2]int]string) {
	wall := carveConnected(rng)
	region, sx, sy, ok := largestOpen(wall)
	if !ok || len(region) < caveW*caveH/8 {
		return fallbackCave()
	}
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
	tiles[sy][sx] = caveMouth
	nodes := scatterLife(rng, tiles, region, sx, sy)
	return &game.TileMap{W: caveW, H: caveH, Tiles: tiles}, sx, sy, nodes
}

// carveConnected carves a cellular-automaton cave then joins its separate
// chambers into one system: tiny pockets are sealed back into rock, and every
// other chamber is linked to the largest by a winding tunnel — so the cave reads
// as rounded chambers strung together by passages rather than a single blob.
func carveConnected(rng *rand.Rand) [][]bool {
	for attempt := 0; attempt < 8; attempt++ {
		wall := carve(rng)
		comps := components(wall)
		if len(comps) == 0 {
			continue
		}
		sort.Slice(comps, func(i, j int) bool { return len(comps[i]) > len(comps[j]) })
		if len(comps[0]) < caveW*caveH/8 {
			continue // even the biggest chamber is too cramped — recarve
		}
		main := comps[0]
		for _, comp := range comps[1:] {
			if len(comp) < 14 { // a stray pocket — wall it off
				for _, c := range comp {
					wall[c[1]][c[0]] = true
				}
				continue
			}
			a := comp[rng.Intn(len(comp))]
			b := nearestIn(main, a)
			carveTunnel(wall, rng, a, b)
		}
		return wall
	}
	wall := make([][]bool, caveH)
	for y := range wall {
		wall[y] = make([]bool, caveW)
	}
	return wall
}

// carveTunnel bores a winding two-wide passage from a toward b (a drunkard's walk
// biased at the target), opening rock as it goes.
func carveTunnel(wall [][]bool, rng *rand.Rand, a, b [2]int) {
	x, y := a[0], a[1]
	open := func(px, py int) {
		for dy := 0; dy <= 1; dy++ {
			for dx := 0; dx <= 1; dx++ {
				nx, ny := px+dx, py+dy
				if nx > 0 && ny > 0 && nx < caveW-1 && ny < caveH-1 {
					wall[ny][nx] = false
				}
			}
		}
	}
	for i := 0; i < 4000; i++ {
		open(x, y)
		if x == b[0] && y == b[1] {
			return
		}
		if rng.Float64() < 0.78 { // mostly head for the target…
			if abs(b[0]-x) > abs(b[1]-y) {
				x += sign(b[0] - x)
			} else {
				y += sign(b[1] - y)
			}
		} else { // …else wander a step
			if rng.Intn(2) == 0 {
				x += sign(rng.Intn(3) - 1)
			} else {
				y += sign(rng.Intn(3) - 1)
			}
		}
		x = clamp(x, 1, caveW-2)
		y = clamp(y, 1, caveH-2)
	}
}

// carve builds the cave as a handful of rounded chambers bored out of solid rock
// and strung together by winding passages — the structure a real cave system has,
// which full-grid noise (one wide-open room) does not. A light smoothing pass then
// roughens the chamber walls so nothing reads as a tidy circle.
func carve(rng *rand.Rand) [][]bool {
	wall := make([][]bool, caveH)
	for y := range wall {
		wall[y] = make([]bool, caveW)
		for x := range wall[y] {
			wall[y][x] = true // start solid; we hollow chambers out of it
		}
	}
	k := 6 + rng.Intn(5) // 6–10 chambers
	centers := make([][2]int, k)
	for i := range centers {
		centers[i] = [2]int{5 + rng.Intn(caveW-10), 5 + rng.Intn(caveH-10)}
		carveBlob(wall, rng, centers[i][0], centers[i][1], 4+rng.Intn(5))
	}
	for i := 1; i < k; i++ { // a passage chain through every chamber…
		carveTunnel(wall, rng, centers[i-1], centers[i])
	}
	for e := 0; e < 2+rng.Intn(2); e++ { // …plus a few extra links for loops
		carveTunnel(wall, rng, centers[rng.Intn(k)], centers[rng.Intn(k)])
	}
	smooth(wall, 2)
	return wall
}

// carveBlob hollows an organic chamber by overlapping a few discs (a cheap
// metaball), so chambers come out lumpy rather than perfectly round.
func carveBlob(wall [][]bool, rng *rand.Rand, cx, cy, r int) {
	for d := 0; d < 3+rng.Intn(3); d++ {
		ox := cx + rng.Intn(2*r+1) - r
		oy := cy + rng.Intn(r+1) - r/2
		carveDisc(wall, ox, oy, r/2+rng.Intn(r/2+1))
	}
}

func carveDisc(wall [][]bool, cx, cy, r int) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				if x, y := cx+dx, cy+dy; x > 0 && y > 0 && x < caveW-1 && y < caveH-1 {
					wall[y][x] = false
				}
			}
		}
	}
}

// smooth roughens chamber walls with a majority rule, leaving the border solid.
// It's gentle enough not to pinch off the two-wide passages.
func smooth(wall [][]bool, passes int) {
	for it := 0; it < passes; it++ {
		next := make([][]bool, caveH)
		for y := 0; y < caveH; y++ {
			next[y] = make([]bool, caveW)
			copy(next[y], wall[y])
		}
		for y := 1; y < caveH-1; y++ {
			for x := 1; x < caveW-1; x++ {
				n := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if (dx != 0 || dy != 0) && wall[y+dy][x+dx] {
							n++
						}
					}
				}
				if n >= 5 {
					next[y][x] = true
				} else if n <= 2 {
					next[y][x] = false
				}
			}
		}
		wall = next
	}
}

// components returns every connected open region (4-connected).
func components(wall [][]bool) [][][2]int {
	seen := make([][]bool, caveH)
	for y := range seen {
		seen[y] = make([]bool, caveW)
	}
	var out [][][2]int
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
				for _, d := range nb4 {
					nx, ny := c[0]+d[0], c[1]+d[1]
					if nx >= 0 && nx < caveW && ny >= 0 && ny < caveH && !wall[ny][nx] && !seen[ny][nx] {
						seen[ny][nx] = true
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}
			out = append(out, comp)
		}
	}
	return out
}

// largestOpen returns the biggest connected open region with a spawn cell near
// its centroid (so the mouth sits in open space, not against a wall).
func largestOpen(wall [][]bool) (region [][2]int, sx, sy int, ok bool) {
	comps := components(wall)
	if len(comps) == 0 {
		return nil, 0, 0, false
	}
	best := comps[0]
	for _, c := range comps[1:] {
		if len(c) > len(best) {
			best = c
		}
	}
	var cx, cy int
	for _, c := range best {
		cx, cy = cx+c[0], cy+c[1]
	}
	cx, cy = cx/len(best), cy/len(best)
	bestD := 1 << 30
	sx, sy = best[0][0], best[0][1]
	for _, c := range best {
		if d := (c[0]-cx)*(c[0]-cx) + (c[1]-cy)*(c[1]-cy); d < bestD {
			bestD, sx, sy = d, c[0], c[1]
		}
	}
	return best, sx, sy, true
}

// scatterLife stocks the cave with its mineral and living features and returns
// the gatherable ones (position → item). Mineral seams stud the rock faces; cave
// mushrooms cluster on the floor of the deep dark away from the mouth; still
// glow-pools pool in the wider chambers. All three light the dark.
func scatterLife(rng *rand.Rand, tiles [][]game.Tile, region [][2]int, sx, sy int) map[[2]int]string {
	nodes := map[[2]int]string{}
	free := func(c [2]int) bool { return tiles[c[1]][c[0]].Kind == game.TileFloor && !(c[0] == sx && c[1] == sy) }
	openCount := func(c [2]int) int {
		n := 0
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if x, y := c[0]+dx, c[1]+dy; x >= 0 && y >= 0 && x < caveW && y < caveH && tiles[y][x].Kind != game.TileWall {
					n++
				}
			}
		}
		return n
	}
	far := func(c [2]int, d int) bool { return abs(c[0]-sx)+abs(c[1]-sy) > d }

	// Mineral seams on rock faces — you work the cavern walls.
	var faces [][2]int
	for _, c := range region {
		if !free(c) {
			continue
		}
		for _, d := range nb4 {
			if nx, ny := c[0]+d[0], c[1]+d[1]; nx >= 0 && ny >= 0 && nx < caveW && ny < caveH && tiles[ny][nx].Kind == game.TileWall {
				faces = append(faces, c)
				break
			}
		}
	}
	rng.Shuffle(len(faces), func(i, j int) { faces[i], faces[j] = faces[j], faces[i] })
	for i := 0; i < 26+rng.Intn(18) && i < len(faces); i++ {
		c := faces[i]
		s := seams[rng.Intn(len(seams))]
		nodes[c] = s.item
		tiles[c[1]][c[0]] = s.tile
	}

	// Mushroom clusters in the deep dark.
	var deep [][2]int
	for _, c := range region {
		if free(c) && far(c, 16) {
			deep = append(deep, c)
		}
	}
	rng.Shuffle(len(deep), func(i, j int) { deep[i], deep[j] = deep[j], deep[i] })
	for i := 0; i < 7+rng.Intn(6) && i < len(deep); i++ {
		seed := deep[i]
		cluster := append([][2]int{seed}, neighboursOf(seed, rng)...)
		for _, c := range cluster {
			if free(c) {
				if _, taken := nodes[c]; !taken {
					nodes[c] = "mushroom"
					tiles[c[1]][c[0]] = mushroom
				}
			}
		}
	}

	// Glow-pools in the wider chambers (kept walkable so they never seal a way).
	var chambers [][2]int
	for _, c := range region {
		if free(c) && openCount(c) >= 8 && far(c, 10) {
			chambers = append(chambers, c)
		}
	}
	rng.Shuffle(len(chambers), func(i, j int) { chambers[i], chambers[j] = chambers[j], chambers[i] })
	for i := 0; i < 4+rng.Intn(4) && i < len(chambers); i++ {
		seed := chambers[i]
		for _, c := range append([][2]int{seed}, neighboursOf(seed, rng)...) {
			if free(c) {
				if _, taken := nodes[c]; !taken {
					tiles[c[1]][c[0]] = glowPool
				}
			}
		}
	}
	return nodes
}

// neighboursOf returns a couple of random orthogonal neighbours of c, for growing
// little clusters.
func neighboursOf(c [2]int, rng *rand.Rand) [][2]int {
	var out [][2]int
	for _, d := range nb4 {
		if rng.Float64() < 0.55 {
			out = append(out, [2]int{c[0] + d[0], c[1] + d[1]})
		}
	}
	return out
}

func fallbackCave() (*game.TileMap, int, int, map[[2]int]string) {
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
	return &game.TileMap{W: caveW, H: caveH, Tiles: tiles}, caveW / 2, caveH / 2, map[[2]int]string{}
}

func nearestIn(region [][2]int, p [2]int) [2]int {
	best, bestD := region[0], 1<<30
	for _, c := range region {
		if d := (c[0]-p[0])*(c[0]-p[0]) + (c[1]-p[1])*(c[1]-p[1]); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
