// Package breakout is the arcade's brick-buster: bounce the ball off your paddle
// to clear every brick; miss and you lose a life. It's a game.Ticker (the ball
// runs on the wall clock) and a board game (camera-framed, no avatar). 'r'
// restarts, 'x' leaves.
package breakout

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

const (
	boardW    = 29
	boardH    = 21
	padW      = 6 // paddle width
	padRow    = boardH - 2
	startLife = 3
	tickMS    = 90
)

// brickRows are the coloured courses near the top, top-to-bottom.
var brickRows = []string{"#FF5D5D", "#FF8A4C", "#FFD166", "#7BD88F", "#56E1FF"}

func init() {
	game.Register("breakout", "Breakout", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "breakout"}}
	})
}

type pt = [2]int

type area struct {
	game.Walker
	padX           int           // paddle left
	bx, by, vx, vy int           // ball
	bricks         map[pt]string // live bricks → colour
	lives          int
	score          int
	over           bool
	won            bool
	stuck          bool // ball rests on the paddle until you nudge it off
	toast          string
	toastUntil     time.Time
}

func (a *area) Name() string      { return "Breakout" }
func (a *area) HideAvatars() bool { return true }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.reset()
	return nil
}

func (a *area) reset() {
	a.lives, a.score = startLife, 0
	a.over, a.won = false, false
	a.layBricks()
	a.padX = boardW/2 - padW/2
	a.serveOnPaddle()
	a.X, a.Y = boardW/2, boardH/2
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

func (a *area) layBricks() {
	a.bricks = map[pt]string{}
	for r, hex := range brickRows {
		y := 2 + r
		for x := 2; x < boardW-2; x++ {
			a.bricks[pt{x, y}] = hex
		}
	}
}

// serveOnPaddle rests the ball on the paddle, launched by the next move.
func (a *area) serveOnPaddle() {
	a.bx, a.by = a.padX+padW/2, padRow-1
	a.vx, a.vy = 1, -1
	a.stuck = true
}

func (a *area) TickInterval() time.Duration { return tickMS * time.Millisecond }

func (a *area) GameTick() game.Area {
	if a.over || a.stuck {
		return a
	}
	// X axis with wall bounce.
	nx := a.bx + a.vx
	if nx <= 1 || nx >= boardW-2 {
		a.vx = -a.vx
		nx = a.bx + a.vx
	}
	// Y axis with top-wall bounce.
	ny := a.by + a.vy
	if ny <= 1 {
		a.vy = 1
		ny = a.by + a.vy
	}
	// Brick hits (check the target cell on both axes).
	if hex, ok := a.bricks[pt{nx, ny}]; ok {
		_ = hex
		delete(a.bricks, pt{nx, ny})
		a.score += 10
		a.vy = -a.vy
		ny = a.by + a.vy
		if len(a.bricks) == 0 {
			a.over, a.won = true, true
			a.setToast("🏆 cleared! · r play again · x leave")
			a.rebuild()
			return a
		}
	}
	// Paddle bounce.
	if ny >= padRow-1 && a.vy > 0 && nx >= a.padX && nx < a.padX+padW {
		a.vy = -1
		// english: steer by where it struck the paddle
		off := nx - (a.padX + padW/2)
		if off < 0 {
			a.vx = -1
		} else if off > 0 {
			a.vx = 1
		}
		ny = padRow - 1
	}
	// Dropped past the paddle.
	if ny >= boardH-1 {
		a.lives--
		if a.lives <= 0 {
			a.over = true
			a.setToast(fmt.Sprintf("game over · score %d · r retry · x leave", a.score))
			a.rebuild()
			return a
		}
		a.serveOnPaddle()
		a.rebuild()
		return a
	}
	a.bx, a.by = nx, ny
	a.rebuild()
	return a
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
	}
	if dx, _, _, ok := game.MoveKey(key.String()); ok && dx != 0 {
		a.padX += dx
		if a.padX < 1 {
			a.padX = 1
		}
		if a.padX+padW > boardW-1 {
			a.padX = boardW - 1 - padW
		}
		if a.stuck { // a move launches the ball
			a.stuck = false
			a.bx = a.padX + padW/2
		}
		a.rebuild()
	}
	return a, nil
}

func (a *area) rebuild() {
	tiles := make([][]game.Tile, boardH)
	for y := 0; y < boardH; y++ {
		row := make([]game.Tile, boardW)
		for x := 0; x < boardW; x++ {
			if y == 0 || x == 0 || x == boardW-1 {
				row[x] = game.Tile{Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#2A3550"}
			} else {
				row[x] = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#121620"}
			}
		}
		tiles[y] = row
	}
	for c, hex := range a.bricks {
		tiles[c[1]][c[0]] = game.Tile{Kind: game.TileDecor, Ch: '▬', Walkable: false,
			Tex: game.TexFloor, Ground: hex, Color: hex}
	}
	for i := 0; i < padW; i++ {
		tiles[padRow][a.padX+i] = game.Tile{Kind: game.TileDecor, Ch: '█', Walkable: false,
			Tex: game.TexFloor, Ground: "#E6E8EE", Color: "#E6E8EE"}
	}
	tiles[a.by][a.bx] = game.Tile{Kind: game.TileDecor, Ch: '●', Walkable: false,
		Tex: game.TexFloor, Ground: "#0F1117", Prop: game.PropGemGlow, PropHex: "#FFE08A"}
	a.Map = &game.TileMap{W: boardW, H: boardH, Tiles: tiles}
}

func (a *area) setToast(s string) { a.toast, a.toastUntil = s, time.Now().Add(5*time.Second) }

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	return fmt.Sprintf("score %d · lives %d · A/D move · r restart · x leave", a.score, a.lives)
}

func (a *area) Prompt() (string, bool) {
	if a.over {
		return "game over · r play again · x leave", true
	}
	if a.stuck {
		return "move (A/D) to launch the ball · x leave", true
	}
	return "A/D move the paddle · clear the bricks · x leave", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("🧱 Breakout"), "",
		th.ChatText.Render("Clear every brick."), "",
		th.Dim.Render("Score   ") + th.Bright.Render(fmt.Sprintf("%d", a.score)),
		th.Dim.Render("Lives   ") + th.Accent.Render(fmt.Sprintf("%d", a.lives)),
		th.Dim.Render("Bricks  ") + th.ChatText.Render(fmt.Sprintf("%d", len(a.bricks))), "",
		th.Dim.Render("A / ←   left"),
		th.Dim.Render("D / →   right"),
		th.Dim.Render("r       restart"),
		th.Dim.Render("x       leave"),
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
	mapView := a.RenderBoard(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}
