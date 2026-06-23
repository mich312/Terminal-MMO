// Package maze is the arcade's labyrinth: find the glowing exit of a
// procedurally generated maze, lit only by the torch you carry. Solve it and a
// fresh, larger maze is carved. A door by the entrance returns to the Arcade.
//
// It embeds game.Walker, so movement, wall collision, the torch light and the
// door back are all the shared machinery; the package only adds maze generation
// and the "reached the exit" check. Like every minigame it is keypress-driven,
// so it plays the same in the glyph and HD clients.
package maze

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
	startCellsW = 7  // maze is this many cells wide on the first board…
	startCellsH = 5  // …and tall
	maxCellsW   = 13 // it grows by one cell each solve, capped here
	maxCellsH   = 9
	torch       = 6 // tiles the torch lights around the player
)

func init() {
	game.Register("maze", "Maze", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "maze"}}
	})
}

type area struct {
	game.Walker
	cw, ch     int    // current maze size in cells
	goal       [2]int // the exit tile
	solved     int    // mazes cleared this visit
	steps      int    // steps taken on the current maze
	rng        *rand.Rand
	toast      string
	toastUntil time.Time
}

func (a *area) Name() string { return "Maze" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	a.cw, a.ch = startCellsW, startCellsH
	a.solved = 0
	a.generate()
	return nil
}

// generate carves a fresh perfect maze with a recursive backtracker and rebuilds
// the render tilemap, then spawns the player at the entrance.
func (a *area) generate() {
	a.steps = 0
	W, H := 2*a.cw+1, 2*a.ch+1
	wall := make([][]bool, H)
	for y := range wall {
		wall[y] = make([]bool, W)
		for x := range wall[y] {
			wall[y][x] = true // start solid; the walk carves passages
		}
	}
	visited := make([][]bool, a.ch)
	for y := range visited {
		visited[y] = make([]bool, a.cw)
	}
	type cell struct{ x, y int }
	stack := []cell{{0, 0}}
	visited[0][0] = true
	wall[1][1] = false
	dirs := []cell{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		// gather unvisited neighbours
		var nbrs []cell
		for _, d := range dirs {
			nx, ny := cur.x+d.x, cur.y+d.y
			if nx >= 0 && nx < a.cw && ny >= 0 && ny < a.ch && !visited[ny][nx] {
				nbrs = append(nbrs, cell{nx, ny})
			}
		}
		if len(nbrs) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		next := nbrs[a.rng.Intn(len(nbrs))]
		// knock down the wall between cur and next
		wall[cur.y+next.y+1][cur.x+next.x+1] = false
		wall[2*next.y+1][2*next.x+1] = false
		visited[next.y][next.x] = true
		stack = append(stack, next)
	}

	a.goal = [2]int{2*a.cw - 1, 2*a.ch - 1}
	a.Map = a.buildMap(wall, W, H)
	a.Enter(1, 1, 0) // entrance; resets the portal latch so we don't bounce out the door
}

func (a *area) buildMap(wall [][]bool, W, H int) *game.TileMap {
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		row := make([]game.Tile, W)
		for x := 0; x < W; x++ {
			if wall[y][x] {
				row[x] = game.Tile{Kind: game.TileWall, Ch: '█', Tex: game.TexRock, Ground: "#2C2A38"}
			} else {
				row[x] = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#1C1B26"}
			}
		}
		tiles[y] = row
	}
	// The exit: a glowing waypoint at the far corner.
	gx, gy := 2*a.cw-1, 2*a.ch-1
	tiles[gy][gx] = game.Tile{Kind: game.TileFloor, Ch: '◈', Walkable: true,
		Tex: game.TexFloor, Ground: "#1F2E1F", Prop: game.PropGemGlow, PropHex: "#7BD88F"}
	// The door back to the Arcade, set into the wall just above the entrance.
	tiles[0][1] = game.Tile{Kind: game.TilePortal, Ch: '◊', Walkable: true,
		Portal: "arcade", Label: "Arcade", Color: ui.HexAccent}
	return &game.TileMap{W: W, H: H, Tiles: tiles}
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "r" {
		a.generate()
		a.setToast("new maze")
		return a, nil
	}
	if _, isKey := msg.(tea.KeyMsg); isKey {
		a.steps++
	}
	if portal, handled := a.HandleCommon(msg); handled {
		if portal != "" {
			return game.Transition{To: portal}, nil
		}
		if a.X == a.goal[0] && a.Y == a.goal[1] {
			a.solved++
			if a.cw < maxCellsW {
				a.cw++
			}
			if a.ch < maxCellsH {
				a.ch++
			}
			a.generate()
			a.setToast(fmt.Sprintf("escaped! maze #%d — here's a bigger one", a.solved+1))
		}
	}
	return a, nil
}

func (a *area) setToast(s string) {
	a.toast = s
	a.toastUntil = time.Now().Add(4 * time.Second)
}

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

// HDLight implements game.HDLighter: the torch circle the HD client draws.
func (a *area) HDLight() game.Light {
	return game.Light{X: a.X, Y: a.Y, Radius: torch}
}

func (a *area) Hint() string {
	return fmt.Sprintf("maze #%d · find the green ◈ exit · r new maze · ◊ door leaves", a.solved+1)
}

func (a *area) Prompt() (string, bool) {
	return "find the glowing ◈ exit · r new maze", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("🌀 Maze"), "",
		th.ChatText.Render("Feel your way to the"),
		th.ChatText.Render("glowing green ◈ exit."), "",
		th.Dim.Render("Maze    ") + th.Bright.Render(fmt.Sprintf("#%d", a.solved+1)),
		th.Dim.Render("Size    ") + th.ChatText.Render(fmt.Sprintf("%d×%d", a.cw, a.ch)),
		th.Dim.Render("Steps   ") + th.ChatText.Render(fmt.Sprintf("%d", a.steps)),
		th.Dim.Render("Cleared ") + th.Accent.Render(fmt.Sprintf("%d", a.solved)), "",
		th.Dim.Render("WASD / arrows  move"),
		th.Dim.Render("r              new maze"),
		th.Dim.Render("◊ door         leave"),
	}
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		rows = append(rows, "", th.Accent.Render(a.toast))
	}
	panel := th.Panel.Width(30).Render(strings.Join(rows, "\n"))

	const gap = 3
	mapW := width - lipgloss.Width(panel) - gap
	if mapW < 24 {
		mapW = 24
	}
	mapView := a.RenderLit(mapW, height, torch)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}
