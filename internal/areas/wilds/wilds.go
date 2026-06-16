// Package wilds is the Wilds: Durst World's procedurally generated, infinite
// overworld. Unlike the hand-authored areas it has no fixed map — the player
// carries absolute world coordinates and every frame a window of terrain is
// sampled from worldgen around them and handed to the normal tile renderer.
// Because generation is a pure function of the seed, every session sees the
// same world and stands on the same ground.
package wilds

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// worldSeed is fixed so the Wilds are identical for everyone. (A later admin
// /regen command can swap it.)
const worldSeed uint64 = 0xD0117_C0FFEE_5742

func init() {
	game.Register("wilds", "The Wilds", func(ctx *game.Ctx) game.Area {
		return &area{ctx: ctx, gen: worldgen.New(worldSeed)}
	})
}

type area struct {
	ctx    *game.Ctx
	gen    *worldgen.Generator
	wx, wy int // absolute world position of the local player
	frame  int
}

func (a *area) Name() string { return "The Wilds" }

func (a *area) Init(*world.Player) tea.Cmd {
	// Spawn just south of the home gate, inside the guaranteed clearing.
	a.wx, a.wy = worldgen.GateX, worldgen.GateY+1
	a.ctx.World.EnterArea(a.ctx.Name, "wilds", a.wx, a.wy, "The Wilds")
	return nil
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	switch msg := msg.(type) {
	case game.WorldEventMsg:
		if ev := world.Event(msg); ev.Type == world.EventTick {
			a.frame = int(ev.Frame)
		}
		return a, nil

	case tea.KeyMsg:
		dx, dy := 0, 0
		switch msg.String() {
		case "up", "w":
			dy = -1
		case "down", "s":
			dy = 1
		case "left", "a":
			dx = -1
		case "right", "d":
			dx = 1
		default:
			return a, nil
		}
		nx, ny := a.wx+dx, a.wy+dy
		cell := a.gen.At(nx, ny)
		if !cell.Walkable {
			return a, nil
		}
		a.wx, a.wy = nx, ny
		a.ctx.World.Move(a.ctx.Name, nx, ny)
		if cell.Portal != "" {
			return game.Transition{To: cell.Portal}, nil
		}
	}
	return a, nil
}

// Hint shows a compass back to the home gate so players are never stranded.
func (a *area) Hint() string {
	dx := worldgen.GateX - a.wx
	dy := worldgen.GateY - a.wy
	if cheb(dx, dy) <= 1 {
		return "⌂ Durst Gate — step in to return to the Lobby"
	}
	return "⌂ Durst Gate " + bearing(dx, dy)
}

func (a *area) View(width, height int) string {
	cx, cy := width/2, height/2
	tiles := make([][]game.Tile, height)
	for ly := 0; ly < height; ly++ {
		row := make([]game.Tile, width)
		for lx := 0; lx < width; lx++ {
			wx := a.wx + lx - cx
			wy := a.wy + ly - cy
			row[lx] = cellToTile(a.gen.At(wx, wy))
		}
		tiles[ly] = row
	}
	tm := &game.TileMap{W: width, H: height, Tiles: tiles}

	// Shift everyone in the area into window-local coordinates so the shared
	// renderer can stamp them on the sampled window.
	var local []world.Player
	for _, p := range a.ctx.World.PlayersInArea("wilds") {
		lp := p
		lp.X = p.X - a.wx + cx
		lp.Y = p.Y - a.wy + cy
		local = append(local, lp)
	}
	return game.RenderMap(a.ctx.Theme, tm, local, a.ctx.Name, a.frame)
}

func cellToTile(c worldgen.Cell) game.Tile {
	kind := game.TileFloor
	switch {
	case c.Portal != "":
		kind = game.TilePortal
	case c.Object:
		kind = game.TileObject
	case !c.Walkable:
		kind = game.TileDecor
	}
	t := game.Tile{
		Kind:     kind,
		Ch:       c.Glyph,
		Walkable: c.Walkable,
		Color:    c.Color,
		Portal:   c.Portal,
	}
	if c.AnimA != "" && c.AnimB != "" {
		t.Anim = &game.TileAnim{Frames: c.Frames, ColorA: c.AnimA, ColorB: c.AnimB, Speed: 3}
	}
	return t
}

func bearing(dx, dy int) string {
	s := ""
	switch {
	case dy < 0:
		s += fmt.Sprintf("%d↑ ", -dy)
	case dy > 0:
		s += fmt.Sprintf("%d↓ ", dy)
	}
	switch {
	case dx < 0:
		s += fmt.Sprintf("%d←", -dx)
	case dx > 0:
		s += fmt.Sprintf("%d→", dx)
	}
	return s
}

func cheb(dx, dy int) int {
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}
