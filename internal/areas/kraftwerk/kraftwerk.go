// Package kraftwerk is a placeholder area: a small machine hall with
// flavor text and a portal back to the lobby.
package kraftwerk

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

var rows = []string{
	"##############################",
	"#..o....................o....#",
	"#..mmm....mmm....mmm.........#",
	"#..mmm....mmm....mmm.........#",
	"#............................#",
	"0...........~~~~~~~~.........#",
	"#............................#",
	"#.......mmmmm....mmmmm.......#",
	"#.......mmmmm....mmmmm.......#",
	"#..o....................o....#",
	"#............................#",
	"##############################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	// Machines hum: glyph pulses through block shades, color cycles cold→hot.
	'm': {Kind: game.TileDecor, Ch: '▓', Anim: &game.TileAnim{
		Frames: []rune{'▓', '▒', '░', '▒'}, ColorA: "#3A4654", ColorB: "#7DF0FF", Speed: 2}},
	// Coolant channel: flowing water glyphs in blues.
	'~': {Kind: game.TileDecor, Ch: '~', Anim: &game.TileAnim{
		Frames: []rune{'~', '≈', '~', '≋'}, ColorA: "#2E6BFF", ColorB: "#56E1FF", Speed: 3}},
	// Lamps: warm flicker, and the only real light in the hall.
	'o': {Kind: game.TileObject, Ch: '◉', Anim: &game.TileAnim{
		ColorA: "#FF8A4C", ColorB: "#FFC861", Speed: 2}},
}

// lightRadius is how far the player's lamp reaches; the rest sits in shadow.
const lightRadius = 9

func init() {
	game.Register("kraftwerk", "Kraftwerk", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{
			Ctx:    ctx,
			Map:    game.ParseMap(rows, legend, nil),
			AreaID: "kraftwerk",
		}}
	})
}

type area struct {
	game.Walker
}

func (a *area) Name() string { return "Kraftwerk" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.Enter(2, 5, 1)
	return nil
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if portal, handled := a.HandleCommon(msg); handled && portal != "" {
		return game.Transition{To: portal}, nil
	}
	return a, nil
}

func (a *area) Hint() string { return a.PortalHint() }

func (a *area) View(width, height int) string {
	panel := ui.PanelStyle.Width(34).Render(
		ui.PanelTitleStyle.Render("⚡ Durst Kraftwerk") + "\n\n" +
			ui.ChatTextStyle.Render("Home of spin-offs and experiments.\n\nThe machines are warming up — mind the coolant."))
	mapView := a.RenderLit(a.Map.W, a.Map.H, lightRadius)
	return lipgloss.JoinHorizontal(lipgloss.Center, mapView, "   ", panel)
}
