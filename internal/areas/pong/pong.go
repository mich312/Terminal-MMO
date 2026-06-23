// Package pong is the arcade's bat-and-ball duel: you (left paddle) versus the
// house AI (right paddle). First to 7 wins. It's a game.Ticker — the ball
// advances on the wall clock — and a board game, so it frames the table with the
// camera and draws no walk-around avatar (HideAvatars). 'r' restarts, 'x' leaves.
package pong

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
	boardW   = 31 // tilemap width (walls top/bottom; left/right are goals)
	boardH   = 19
	padH     = 5 // paddle height in tiles
	winScore = 7
	tickMS   = 90 // ball step cadence
)

func init() {
	game.Register("pong", "Pong", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "pong"}}
	})
}

type area struct {
	game.Walker
	playerY, aiY      int // paddle tops (left = player, right = AI)
	bx, by, vx, vy    int // ball position and velocity
	youScore, aiScore int
	over              bool
	win               bool
	toast             string
	toastUntil        time.Time
}

func (a *area) Name() string      { return "Pong" }
func (a *area) HideAvatars() bool { return true }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.reset()
	return nil
}

// reset re-centres the paddles and serves a fresh match.
func (a *area) reset() {
	a.playerY = boardH/2 - padH/2
	a.aiY = boardH/2 - padH/2
	a.youScore, a.aiScore = 0, 0
	a.over, a.win = false, false
	a.serve(1)
	a.X, a.Y = boardW/2, boardH/2 // frame the table
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

// serve places the ball at centre heading towards dir (-1 left, +1 right).
func (a *area) serve(dir int) {
	a.bx, a.by = boardW/2, boardH/2
	a.vx, a.vy = dir, 1
}

func (a *area) TickInterval() time.Duration { return tickMS * time.Millisecond }

func (a *area) GameTick() game.Area {
	if a.over {
		return a
	}
	// AI tracks the ball, one step per tick, but lazily (only when the ball
	// approaches) so it's beatable.
	if a.vx > 0 {
		mid := a.aiY + padH/2
		if a.by < mid && a.aiY > 1 {
			a.aiY--
		} else if a.by > mid && a.aiY+padH < boardH-1 {
			a.aiY++
		}
	}

	nx, ny := a.bx+a.vx, a.by+a.vy
	// Top / bottom walls.
	if ny <= 1 {
		ny, a.vy = 1, 1
	} else if ny >= boardH-2 {
		ny, a.vy = boardH-2, -1
	}
	// Player paddle (left, column 2 face).
	if nx <= 2 {
		if ny >= a.playerY && ny < a.playerY+padH {
			nx, a.vx = 2, 1
		} else if nx <= 0 { // missed → AI scores
			a.aiScore++
			a.score(-1)
			return a
		}
	}
	// AI paddle (right, column boardW-3 face).
	if nx >= boardW-3 {
		if ny >= a.aiY && ny < a.aiY+padH {
			nx, a.vx = boardW-3, -1
		} else if nx >= boardW-1 { // missed → you score
			a.youScore++
			a.score(1)
			return a
		}
	}
	a.bx, a.by = nx, ny
	a.rebuild()
	return a
}

// score records a point and either ends the match or serves towards the loser.
func (a *area) score(loser int) {
	if a.youScore >= winScore || a.aiScore >= winScore {
		a.over = true
		a.win = a.youScore > a.aiScore
		if a.win {
			a.setToast("🏆 you win! · r rematch · x leave")
		} else {
			a.setToast("the house wins · r rematch · x leave")
		}
		a.rebuild()
		return
	}
	a.serve(loser) // serve towards whoever was just scored on
	a.rebuild()
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
	if _, dy, _, ok := game.MoveKey(key.String()); ok && dy != 0 {
		a.playerY += dy
		if a.playerY < 1 {
			a.playerY = 1
		}
		if a.playerY+padH > boardH-1 {
			a.playerY = boardH - 1 - padH
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
			switch {
			case y == 0 || y == boardH-1:
				row[x] = game.Tile{Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#2A3550"}
			case x == boardW/2:
				row[x] = game.Tile{Kind: game.TileFloor, Ch: '┊', Walkable: true, Tex: game.TexFloor, Ground: "#1B2030", Color: "#39405A"}
			default:
				row[x] = game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#14171F"}
			}
		}
		tiles[y] = row
	}
	for i := 0; i < padH; i++ {
		tiles[a.playerY+i][1] = paddle("#56E1FF")
		tiles[a.aiY+i][boardW-2] = paddle("#FF8A4C")
	}
	tiles[a.by][a.bx] = game.Tile{Kind: game.TileDecor, Ch: '●', Walkable: false,
		Tex: game.TexFloor, Ground: "#0F1117", Prop: game.PropGemGlow, PropHex: "#FFE08A"}
	a.Map = &game.TileMap{W: boardW, H: boardH, Tiles: tiles}
}

func paddle(hex string) game.Tile {
	return game.Tile{Kind: game.TileDecor, Ch: '█', Walkable: false, Tex: game.TexFloor, Ground: hex, Color: hex}
}

func (a *area) setToast(s string) {
	a.toast, a.toastUntil = s, time.Now().Add(5*time.Second)
}

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		return a.toast, true
	}
	return "", false
}

func (a *area) Hint() string {
	return fmt.Sprintf("you %d — %d house · W/S move · r restart · x leave", a.youScore, a.aiScore)
}

func (a *area) Prompt() (string, bool) {
	if a.over {
		return "match over · r rematch · x leave", true
	}
	return "W/S move your paddle · first to 7 · x leave", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("🏓 Pong"), "",
		th.ChatText.Render("Beat the house to 7."), "",
		th.Dim.Render("You     ") + th.Fg(lipgloss.Color("#56E1FF")).Render(fmt.Sprintf("%d", a.youScore)),
		th.Dim.Render("House   ") + th.Fg(lipgloss.Color("#FF8A4C")).Render(fmt.Sprintf("%d", a.aiScore)), "",
		th.Dim.Render("W / ↑   up"),
		th.Dim.Render("S / ↓   down"),
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
