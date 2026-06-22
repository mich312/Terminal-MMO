// Package bomberman is the arcade's bomb-and-blast maze: drop bombs to blow up
// the soft blocks and the roaming foes, clear the field, and don't get caught in
// your own blast. It's a game.Ticker (bombs and enemies run on the wall clock)
// and the player is a token on the grid, so it embeds game.Walker and walks the
// avatar like Sokoban/Snake. Move WASD, drop a bomb with 'e'; 'r' restarts, 'x'
// leaves.
package bomberman

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

const (
	gridW    = 15 // odd so the hard-wall lattice frames the field
	gridH    = 13
	blast    = 2  // explosion reach in each direction
	fuse     = 10 // ticks a bomb counts down (≈2s)
	flameLen = 2  // ticks a flame lingers
	maxBombs = 2
	enemyN   = 3
	tickMS   = 200
	enemyEvy = 2 // enemies step every Nth tick
	lives0   = 3
)

func init() {
	game.Register("bomberman", "Bomberman", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "bomberman"}}
	})
}

type pt = [2]int

type area struct {
	game.Walker
	soft       map[pt]bool
	bombs      map[pt]int // position → fuse remaining
	flames     map[pt]int // position → ticks remaining
	enemies    []pt
	lives      int
	over       bool
	won        bool
	tickN      int
	rng        *rand.Rand
	toast      string
	toastUntil time.Time
}

func (a *area) Name() string { return "Bomberman" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	a.reset()
	return nil
}

func (a *area) reset() {
	a.bombs = map[pt]int{}
	a.flames = map[pt]int{}
	a.lives = lives0
	a.over, a.won = false, false
	a.layout()
	a.X, a.Y = 1, 1
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

// hard reports the indestructible lattice: the border and every (even,even) cell.
func hard(x, y int) bool {
	return x == 0 || y == 0 || x == gridW-1 || y == gridH-1 || (x%2 == 0 && y%2 == 0)
}

// layout scatters soft blocks (clear of the spawn corner) and places the enemies.
func (a *area) layout() {
	a.soft = map[pt]bool{}
	spawnSafe := map[pt]bool{{1, 1}: true, {1, 2}: true, {2, 1}: true}
	for y := 1; y < gridH-1; y++ {
		for x := 1; x < gridW-1; x++ {
			if hard(x, y) || spawnSafe[pt{x, y}] {
				continue
			}
			if a.rng.Float64() < 0.62 {
				a.soft[pt{x, y}] = true
			}
		}
	}
	a.enemies = nil
	for len(a.enemies) < enemyN {
		x, y := 1+a.rng.Intn(gridW-2), 1+a.rng.Intn(gridH-2)
		c := pt{x, y}
		// place on open ground, away from the spawn corner
		if hard(x, y) || a.soft[c] || (x < 4 && y < 4) || a.enemyAt(c) {
			continue
		}
		a.enemies = append(a.enemies, c)
	}
}

func (a *area) enemyAt(c pt) bool {
	for _, e := range a.enemies {
		if e == c {
			return true
		}
	}
	return false
}

// passable reports whether a body may enter (x,y): not a wall, block or bomb.
func (a *area) passable(x, y int) bool {
	return !hard(x, y) && !a.soft[pt{x, y}] && a.bombs[pt{x, y}] == 0
}

func (a *area) TickInterval() time.Duration { return tickMS * time.Millisecond }

func (a *area) GameTick() game.Area {
	if a.over {
		return a
	}
	a.tickN++

	// Flames fade.
	for c, n := range a.flames {
		if n <= 1 {
			delete(a.flames, c)
		} else {
			a.flames[c] = n - 1
		}
	}
	// Bomb fuses; detonate any that reach zero (with chain reactions).
	var pop []pt
	for c, n := range a.bombs {
		if n <= 1 {
			pop = append(pop, c)
		} else {
			a.bombs[c] = n - 1
		}
	}
	for _, c := range pop {
		a.detonate(c)
	}
	// Enemies wander; standing in flame kills them.
	if a.tickN%enemyEvy == 0 {
		a.moveEnemies()
	}
	a.cullEnemiesInFlame()
	a.checkPlayer()
	if len(a.enemies) == 0 && !a.over {
		a.over, a.won = true, true
		a.setToast("🏆 field cleared! · r play again · x leave")
	}
	a.rebuild()
	return a
}

// detonate blows up the bomb at c: a cross of flame, stopped by walls, eating the
// first soft block it meets in each arm and chaining into other bombs it reaches.
func (a *area) detonate(c pt) {
	if _, ok := a.bombs[c]; !ok {
		return
	}
	delete(a.bombs, c)
	a.flames[c] = flameLen
	for _, d := range []pt{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		for r := 1; r <= blast; r++ {
			x, y := c[0]+d[0]*r, c[1]+d[1]*r
			if hard(x, y) {
				break
			}
			a.flames[pt{x, y}] = flameLen
			if a.soft[pt{x, y}] {
				delete(a.soft, pt{x, y})
				break // the wall absorbs the rest of this arm
			}
			if _, ok := a.bombs[pt{x, y}]; ok {
				a.detonate(pt{x, y}) // chain reaction
			}
		}
	}
}

func (a *area) moveEnemies() {
	for i, e := range a.enemies {
		dirs := []pt{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
		a.rng.Shuffle(len(dirs), func(p, q int) { dirs[p], dirs[q] = dirs[q], dirs[p] })
		for _, d := range dirs {
			nx, ny := e[0]+d[0], e[1]+d[1]
			if a.passable(nx, ny) && !a.enemyAt(pt{nx, ny}) {
				a.enemies[i] = pt{nx, ny}
				break
			}
		}
	}
}

func (a *area) cullEnemiesInFlame() {
	kept := a.enemies[:0]
	for _, e := range a.enemies {
		if a.flames[e] == 0 {
			kept = append(kept, e)
		}
	}
	a.enemies = kept
}

// checkPlayer kills the player if they share a cell with a flame or an enemy.
func (a *area) checkPlayer() {
	p := pt{a.X, a.Y}
	if a.flames[p] > 0 || a.enemyAt(p) {
		a.lives--
		if a.lives <= 0 {
			a.over = true
			a.setToast("💀 game over · r retry · x leave")
			return
		}
		a.setToast(fmt.Sprintf("caught! %d lives left", a.lives))
		a.X, a.Y = 1, 1 // back to the spawn corner
		a.Ctx.World.Move(a.Ctx.Name, a.X, a.Y)
	}
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	switch key.String() {
	case "x":
		return game.Transition{To: "arcade"}, nil
	case "r":
		a.reset()
		return a, nil
	case "e", " ": // drop a bomb under yourself
		if a.over {
			return a, nil
		}
		if a.bombs[pt{a.X, a.Y}] == 0 && len(a.bombs) < maxBombs {
			a.bombs[pt{a.X, a.Y}] = fuse
			a.rebuild()
		}
		return a, nil
	}
	if a.over {
		return a, nil
	}
	if dx, dy, _, ok := game.MoveKey(key.String()); ok && (dx == 0) != (dy == 0) {
		nx, ny := a.X+dx, a.Y+dy
		if a.passable(nx, ny) {
			a.X, a.Y = nx, ny
			a.Ctx.World.Move(a.Ctx.Name, a.X, a.Y)
			a.checkPlayer() // walked into a flame or a foe
			a.rebuild()
		}
	}
	return a, nil
}

func (a *area) rebuild() {
	tiles := make([][]game.Tile, gridH)
	for y := 0; y < gridH; y++ {
		row := make([]game.Tile, gridW)
		for x := 0; x < gridW; x++ {
			switch {
			case hard(x, y):
				row[x] = game.Tile{Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#4A4658"}
			default:
				row[x] = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#1C2A1C"}
			}
		}
		tiles[y] = row
	}
	for c := range a.soft {
		tiles[c[1]][c[0]] = game.Tile{Kind: game.TileDecor, Ch: '▢', Walkable: false,
			Tex: game.TexFloor, Ground: "#5A4A30", Prop: game.PropCrate, PropHex: "#B08A4C"}
	}
	for c := range a.bombs {
		tiles[c[1]][c[0]] = game.Tile{Kind: game.TileDecor, Ch: '◉', Walkable: false,
			Tex: game.TexFloor, Ground: "#16160F", Prop: game.PropGemGlow, PropHex: "#FF4040", Anim: &game.TileAnim{
				ColorA: "#FF4040", ColorB: "#FFD166", Speed: 1}}
	}
	for c := range a.flames {
		tiles[c[1]][c[0]] = game.Tile{Kind: game.TileFloor, Ch: '✸', Walkable: true,
			Tex: game.TexFloor, Ground: "#5A2A00", Prop: game.PropGemGlow, PropHex: "#FF8A1E", Anim: &game.TileAnim{
				ColorA: "#FFD166", ColorB: "#FF5D1E", Speed: 1}}
	}
	for _, e := range a.enemies {
		tiles[e[1]][e[0]] = game.Tile{Kind: game.TileDecor, Ch: '☻', Walkable: false,
			Tex: game.TexFloor, Ground: "#3A1E3A", Prop: game.PropGemGlow, PropHex: "#E060E0"}
	}
	a.Map = &game.TileMap{W: gridW, H: gridH, Tiles: tiles}
}

func (a *area) setToast(s string) { a.toast, a.toastUntil = s, time.Now().Add(5*time.Second) }

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	return fmt.Sprintf("foes %d · lives %d · WASD move · e bomb · r restart · x leave", len(a.enemies), a.lives)
}

func (a *area) Prompt() (string, bool) {
	if a.over {
		return "game over · r play again · x leave", true
	}
	return "WASD move · e drop a bomb · clear the foes · x leave", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("💣 Bomberman"), "",
		th.ChatText.Render("Bomb the blocks and the"),
		th.ChatText.Render("☻ foes. Mind the blast!"), "",
		th.Dim.Render("Foes    ") + th.Bright.Render(fmt.Sprintf("%d", len(a.enemies))),
		th.Dim.Render("Lives   ") + th.Accent.Render(fmt.Sprintf("%d", a.lives)),
		th.Dim.Render("Bombs   ") + th.ChatText.Render(fmt.Sprintf("%d/%d", len(a.bombs), maxBombs)), "",
		th.Dim.Render("WASD / arrows  move"),
		th.Dim.Render("e              bomb"),
		th.Dim.Render("r              restart"),
		th.Dim.Render("x              leave"),
	}
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		rows = append(rows, "", th.Accent.Render(a.toast))
	}
	panel := th.Panel.Width(28).Render(strings.Join(rows, "\n"))

	const gap = 3
	mapW := width - lipgloss.Width(panel) - gap
	if mapW < 24 {
		mapW = 24
	}
	mapView := a.RenderViewport(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}
