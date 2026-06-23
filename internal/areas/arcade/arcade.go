// Package arcade is the Durst Arcade: a neon hall of cabinets, each a portal
// into a minigame, with a door back out to the Wilds. The games dock here. A
// portal in the overworld (and one in the lobby) leads to it.
//
// It is a Walker-based room: the shared base gives it movement, wall collision,
// the HD pixel renderer and portal triggering, so the package is little more
// than a hand-drawn map. Each cabinet is a TilePortal carrying a screen sprite;
// walking into one enters that game. To dock a new game, point one of the spare
// 'c' cabinets at it (and add the matching cabinet→spawn entry below).
package arcade

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

var rows = []string{
	"##############################",
	"#............................#",
	"#..S....M....N....T....P.....#",
	"#............................#",
	"#............................#",
	"#..B....Z....G....C....O.....#",
	"#............................#",
	"#..o......................o..#",
	"#............................#",
	"#............................#",
	"#............................#",
	"#............................#",
	"#............................#",
	"#............................#",
	"#............................#",
	"##############X###############",
}

// cabinet builds a portal tile wearing a glowing screen in the given colour.
func cabinet(dest, label, hex, animA string) game.LegendEntry {
	return game.LegendEntry{Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: dest, Label: label,
		Color: hex, Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropScreen, PropHex: hex,
		Anim: &game.TileAnim{ColorA: animA, ColorB: hex, Speed: 3}}
}

var legend = map[rune]game.LegendEntry{
	'S': cabinet("sokoban", "Sokoban", "#7BD88F", "#4FD6BE"),
	'M': cabinet("maze", "Maze", "#C792EA", "#9A6FD6"),
	'N': cabinet("snake", "Snake", "#7BD88F", "#3FB65B"),
	'T': cabinet("tetris", "Tetris", "#56E1FF", "#2E8BFF"),
	'P': cabinet("pong", "Pong", "#FFD166", "#FF8A4C"),
	'B': cabinet("breakout", "Breakout", "#FF7AD5", "#7A4CFF"),
	'Z': cabinet("bomberman", "Bomberman", "#FF4040", "#FFD166"),
	'G': cabinet("2048", "2048", "#FFD166", "#FF8A4C"),
	'C': cabinet("chess", "Chess", "#EAE0C8", "#9AA0B0"),
	'O': cabinet("doom", "Doom", "#C24A3A", "#FF8A4C"),
	// The door back to the overworld.
	'X': {Kind: game.TilePortal, Ch: '◈', Walkable: true, Portal: "wilds", Label: "The Wilds", Color: "#56E1FF"},
	// Dormant cabinets — room for the next games to dock.
	'c': {Kind: game.TileDecor, Ch: '▦', Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropMachine, PropHex: "#5A5470"},
	// Floor-standing lamps that wash the hall in neon.
	'o': {Kind: game.TileDecor, Ch: '◉', Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropLamp, PropHex: "#FF7AD5", Anim: &game.TileAnim{
		ColorA: "#7A4CFF", ColorB: "#FF7AD5", Speed: 2}},
	'#': {Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#2A2440"},
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#1E1B28"},
}

// spawnBy maps the game you came back from to a spot just below its cabinet, so
// you reappear at the machine you were playing.
var spawnBy = map[string][2]int{
	"sokoban":   {3, 3},
	"maze":      {8, 3},
	"snake":     {13, 3},
	"tetris":    {18, 3},
	"pong":      {23, 3},
	"breakout":  {3, 6},
	"bomberman": {8, 6},
	"2048":      {13, 6},
	"chess":     {18, 6},
	"doom":      {23, 6},
}

var texts = []game.MapText{
	{X: 10, Y: 1, S: "[ ARCADE ]"},
}

func init() {
	game.Register("arcade", "Arcade", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{
			Ctx:    ctx,
			Map:    game.ParseMap(rows, legend, texts),
			AreaID: "arcade",
		}}
	})
}

type area struct {
	game.Walker
}

func (a *area) Name() string { return "Arcade" }

func (a *area) Init(p *world.Player) tea.Cmd {
	if s, ok := spawnBy[a.Ctx.From]; ok {
		a.Enter(s[0], s[1], 0)
	} else {
		a.Enter(14, 14, 0) // by the entrance door
	}
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
	return "walk into a cabinet to play · ◈ door leaves"
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	line := func(name, blurb string) string {
		return th.Accent.Render("◊ "+name) + th.Dim.Render("  "+blurb)
	}
	panel := th.Panel.Width(28).Render(strings.Join([]string{
		th.PanelTitle.Render("🎮 Durst Arcade"), "",
		th.ChatText.Render("Step into a cabinet to"),
		th.ChatText.Render("play. ◈ door → the Wilds."), "",
		line("Sokoban", "crate puzzle"),
		line("Maze", "find the exit"),
		line("Snake", "eat & grow"),
		line("Tetris", "stack & clear"),
		line("Pong", "beat the house"),
		line("Breakout", "bust bricks"),
		line("Bomberman", "blast foes"),
		line("2048", "merge tiles"),
		line("Chess", "vs the house"),
		line("Doom", "raycaster maze"),
	}, "\n"))

	const gap = 3
	mapW := width - lipgloss.Width(panel) - gap
	if mapW < 24 {
		mapW = 24
	}
	mapView := a.RenderViewport(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}
