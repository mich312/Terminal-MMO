// Package snake is the arcade's real-time classic: steer a growing snake around
// a walled pit, eat the glowing pellets, and don't bite your own tail or hit the
// wall. It is the worked example of a game.Ticker — an area that advances on its
// own wall clock rather than only on key presses — so both the glyph and HD
// clients drive it at a steady cadence (see GameTick/TickInterval).
//
// It embeds game.Walker for the shared tilemap and HD rendering; the player's
// world position rides the snake's head so the camera (and the "you" avatar)
// follow it. Steering keys set the next direction; the actual move happens on the
// tick. 'r' restarts, 'x' leaves for the Arcade.
package snake

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
	boardW = 21  // tilemap width (including the wall border)
	boardH = 15  // tilemap height
	baseMS = 160 // tick at the start, in milliseconds…
	minMS  = 80  // …speeding up as you grow, down to this floor
	perEat = 6   // milliseconds shaved off the tick per pellet eaten
)

func init() {
	game.Register("snake", "Snake", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "snake"}}
	})
}

type pt = [2]int

type area struct {
	game.Walker
	body    []pt // head first … tail last
	dir     pt   // current heading
	nextDir pt   // heading to apply on the next tick (debounced against reversal)
	food    pt
	dead    bool
	score   int
	best    int // best score this visit
	rng     *rand.Rand

	toast      string
	toastUntil time.Time
}

func (a *area) Name() string { return "Snake" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	a.reset()
	return nil
}

// reset starts a fresh run: a length-3 snake mid-board heading right.
func (a *area) reset() {
	cx, cy := boardW/2, boardH/2
	a.body = []pt{{cx, cy}, {cx - 1, cy}, {cx - 2, cy}}
	a.dir, a.nextDir = pt{1, 0}, pt{1, 0}
	a.dead = false
	a.score = 0
	a.placeFood()
	a.X, a.Y = a.body[0][0], a.body[0][1]
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

// placeFood drops a pellet on a random open interior cell.
func (a *area) placeFood() {
	for {
		f := pt{1 + a.rng.Intn(boardW-2), 1 + a.rng.Intn(boardH-2)}
		if !a.onSnake(f) {
			a.food = f
			return
		}
	}
}

func (a *area) onSnake(c pt) bool {
	for _, b := range a.body {
		if b == c {
			return true
		}
	}
	return false
}

// TickInterval implements game.Ticker: the snake quickens as it grows.
func (a *area) TickInterval() time.Duration {
	ms := baseMS - a.score*perEat
	if ms < minMS {
		ms = minMS
	}
	return time.Duration(ms) * time.Millisecond
}

// GameTick implements game.Ticker: advance the snake one cell.
func (a *area) GameTick() game.Area {
	if a.dead {
		return a
	}
	a.dir = a.nextDir
	head := a.body[0]
	nh := pt{head[0] + a.dir[0], head[1] + a.dir[1]}

	// Wall, or biting the body (excluding the tail tip, which will move away —
	// unless we're about to grow into it).
	hitsSelf := a.onSnake(nh) && !(nh == a.body[len(a.body)-1] && nh != a.food)
	if nh[0] <= 0 || nh[0] >= boardW-1 || nh[1] <= 0 || nh[1] >= boardH-1 || hitsSelf {
		a.dead = true
		if a.score > a.best {
			a.best = a.score
		}
		a.setToast(fmt.Sprintf("💥 game over — score %d · r restart · x leave", a.score))
		return a
	}

	a.body = append([]pt{nh}, a.body...) // grow at the head
	if nh == a.food {
		a.score++
		a.placeFood()
	} else {
		a.body = a.body[:len(a.body)-1] // no food → drop the tail
	}

	a.X, a.Y = nh[0], nh[1]
	a.Ctx.World.Move(a.Ctx.Name, a.X, a.Y)
	a.rebuild()
	return a
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	switch key.String() {
	case "x": // leave for the Arcade
		return game.Transition{To: "arcade"}, nil
	case "r": // restart
		a.reset()
		return a, nil
	}
	if dx, dy, _, ok := game.MoveKey(key.String()); ok && (dx == 0) != (dy == 0) {
		// Cardinal only, and you can't reverse straight back onto your neck.
		if dx != -a.dir[0] || dy != -a.dir[1] {
			a.nextDir = pt{dx, dy}
		}
	}
	return a, nil
}

// rebuild regenerates the render tilemap from the run state. The head is left as
// floor — the renderer stamps the player's avatar there from world position.
func (a *area) rebuild() {
	tiles := make([][]game.Tile, boardH)
	for y := 0; y < boardH; y++ {
		row := make([]game.Tile, boardW)
		for x := 0; x < boardW; x++ {
			if x == 0 || x == boardW-1 || y == 0 || y == boardH-1 {
				row[x] = game.Tile{Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#2A3550"}
			} else {
				row[x] = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#16201A"}
			}
		}
		tiles[y] = row
	}
	bodyHex := "#3FB65B"
	if a.dead {
		bodyHex = "#8A6A6A" // a dull, lifeless coil
	}
	for i, b := range a.body {
		if i == 0 {
			continue // head = avatar
		}
		tiles[b[1]][b[0]] = game.Tile{Kind: game.TileDecor, Ch: '█', Walkable: false,
			Tex: game.TexFloor, Ground: bodyHex, Color: bodyHex}
	}
	tiles[a.food[1]][a.food[0]] = game.Tile{Kind: game.TileFloor, Ch: '◆', Walkable: true,
		Tex: game.TexFloor, Ground: "#3A1E22", Prop: game.PropGemGlow, PropHex: "#FF5D5D"}
	a.Map = &game.TileMap{W: boardW, H: boardH, Tiles: tiles}
}

func (a *area) setToast(s string) {
	a.toast = s
	a.toastUntil = time.Now().Add(5 * time.Second)
}

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	if a.dead {
		return fmt.Sprintf("game over · score %d · r restart · x leave", a.score)
	}
	return fmt.Sprintf("score %d · steer with WASD/↑↓←→ · x leave", a.score)
}

func (a *area) Prompt() (string, bool) {
	if a.dead {
		return "game over · r restart · x leave", true
	}
	return "eat the ◆ pellets · don't hit the wall or yourself · x leave", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	status := th.Accent.Render("● playing")
	if a.dead {
		status = th.Warn.Render("✖ game over")
	}
	rows := []string{
		th.PanelTitle.Render("🐍 Snake"), "",
		th.ChatText.Render("Eat the ◆ pellets. Don't"),
		th.ChatText.Render("hit the wall or yourself."), "",
		th.Dim.Render("Status  ") + status,
		th.Dim.Render("Score   ") + th.Bright.Render(fmt.Sprintf("%d", a.score)),
		th.Dim.Render("Length  ") + th.ChatText.Render(fmt.Sprintf("%d", len(a.body))),
		th.Dim.Render("Best    ") + th.Accent.Render(fmt.Sprintf("%d", a.best)), "",
		th.Dim.Render("WASD / arrows  steer"),
		th.Dim.Render("r              restart"),
		th.Dim.Render("x              leave"),
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
	mapView := a.RenderViewport(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}
