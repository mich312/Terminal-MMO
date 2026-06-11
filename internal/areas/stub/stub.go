// Package stub is the generic "coming soon" area — the template for
// plugging in future mini-games: register an id, implement Area, point a
// portal at it.
package stub

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

func init() {
	game.Register("arcade", "Arcade", func(ctx *game.Ctx) game.Area {
		return &area{ctx: ctx, id: "arcade", title: "🎮 Arcade"}
	})
}

type area struct {
	ctx   *game.Ctx
	id    string
	title string
}

func (a *area) Name() string { return game.DisplayName(a.id) }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.ctx.World.EnterArea(a.ctx.Name, a.id, 0, 0, game.DisplayName(a.id))
	return nil
}

// CapturesInput: any key returns to the lobby, so grab them all.
func (a *area) CapturesInput() bool { return true }

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return game.Transition{To: "lobby"}, nil
	}
	return a, nil
}

func (a *area) View(width, height int) string {
	panel := ui.PanelStyle.Render(
		ui.PanelTitleStyle.Render(a.title+" — coming soon") + "\n\n" +
			ui.ChatTextStyle.Render("Future mini-games dock here.") + "\n\n" +
			ui.DimStyle.Render("press any key to return to the Lobby"))
	return panel
}
