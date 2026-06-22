// Package arcade is the Durst Arcade: a neon hall of cabinets, each a portal
// into a minigame, with a door back out to the Wilds. It replaces the old
// "coming soon" stub — the games dock here. A portal in the overworld (and one
// in the lobby) leads to it.
//
// It is a Walker-based room: the shared base gives it movement, wall collision,
// the HD pixel renderer and portal triggering, so the package is little more
// than a hand-drawn map. Each cabinet is a TilePortal carrying a screen sprite;
// walking into one enters that game.
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
	"######################",
	"#....................#",
	"#..S.....M.....c.....#",
	"#....................#",
	"#....................#",
	"#..o..............o..#",
	"#....................#",
	"#....................#",
	"#....................#",
	"#....................#",
	"#....................#",
	"##########X###########",
}

var legend = map[rune]game.LegendEntry{
	// Cabinets: portals that wear a glowing screen rather than the default gate.
	'S': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "sokoban", Label: "Sokoban",
		Color: "#7BD88F", Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropScreen, PropHex: "#7BD88F",
		Anim: &game.TileAnim{ColorA: "#4FD6BE", ColorB: "#7BD88F", Speed: 3}},
	'M': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "maze", Label: "Maze",
		Color: "#C792EA", Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropScreen, PropHex: "#C792EA",
		Anim: &game.TileAnim{ColorA: "#9A6FD6", ColorB: "#C792EA", Speed: 3}},
	// The door back to the overworld.
	'X': {Kind: game.TilePortal, Ch: '◈', Walkable: true, Portal: "wilds", Label: "The Wilds", Color: "#56E1FF"},
	// A dormant cabinet — room for the next game to dock.
	'c': {Kind: game.TileDecor, Ch: '▦', Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropMachine, PropHex: "#5A5470"},
	// Floor-standing lamps that wash the hall in neon.
	'o': {Kind: game.TileDecor, Ch: '◉', Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropLamp, PropHex: "#FF7AD5", Anim: &game.TileAnim{
		ColorA: "#7A4CFF", ColorB: "#FF7AD5", Speed: 2}},
	'#': {Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#2A2440"},
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#1E1B28"},
}

var texts = []game.MapText{
	{X: 6, Y: 1, S: "[ ARCADE ]"},
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
	// Spawn beside the cabinet we came back from, else by the entrance door.
	switch a.Ctx.From {
	case "sokoban":
		a.Enter(3, 3, 0)
	case "maze":
		a.Enter(9, 3, 0)
	default:
		a.Enter(10, 10, 0)
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
	panel := th.Panel.Width(28).Render(strings.Join([]string{
		th.PanelTitle.Render("🎮 Durst Arcade"), "",
		th.ChatText.Render("Step into a cabinet to"),
		th.ChatText.Render("play. Walk out the ◈ door"),
		th.ChatText.Render("to the Wilds."), "",
		th.Accent.Render("◊ Sokoban") + th.Dim.Render("  crate puzzle"),
		th.Accent.Render("◊ Maze") + th.Dim.Render("     find the exit"),
		th.Dim.Render("▦ (more docking soon)"),
	}, "\n"))

	const gap = 3
	mapW := width - lipgloss.Width(panel) - gap
	if mapW < 24 {
		mapW = 24
	}
	mapView := a.RenderViewport(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}
