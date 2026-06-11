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
	"#............................#",
	"#..mmm....mmm....mmm.........#",
	"#..mmm....mmm....mmm.........#",
	"#............................#",
	"0............................#",
	"#............................#",
	"#.......mmmmm....mmmmm.......#",
	"#.......mmmmm....mmmmm.......#",
	"#............................#",
	"#............................#",
	"##############################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	'm': {Kind: game.TileDecor, Ch: '▓'},
}

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
			ui.ChatTextStyle.Render("Home of spin-offs and experiments.\n\nNothing is plugged in yet."))
	return lipgloss.JoinHorizontal(lipgloss.Center, a.Render(), "   ", panel)
}
