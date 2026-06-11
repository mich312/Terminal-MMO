// Package democenter is a placeholder area: a small showroom with
// decorative printers and a portal back to the lobby.
package democenter

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
	"#...pppppp........pppppp.....#",
	"#...p....p........p....p.....#",
	"#...pppppp........pppppp.....#",
	"#............................0",
	"#............................#",
	"#.........pppppppp...........#",
	"#.........p......p...........#",
	"#.........pppppppp...........#",
	"#............................#",
	"##############################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	'p': {Kind: game.TileDecor, Ch: '▚'},
}

func init() {
	game.Register("democenter", "Demo Center", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{
			Ctx:    ctx,
			Map:    game.ParseMap(rows, legend, nil),
			AreaID: "democenter",
		}}
	})
}

type area struct {
	game.Walker
}

func (a *area) Name() string { return "Demo Center" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.Enter(27, 5, 1)
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
		ui.PanelTitleStyle.Render("🖨 Demo Center") + "\n\n" +
			ui.ChatTextStyle.Render("The printers are warming up.\n\nCome back for the full tour."))
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", a.Render())
}
