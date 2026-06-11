// Package lobby is the Durst HQ entrance hall: spawn point, reception desk,
// guestbook, and the four portals to everywhere else.
package lobby

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

const guestbookMax = 80

var rows = []string{
	"#############################1##############################",
	"#..........................................................#",
	"#....------------..........................................#",
	"#....============.g........................................#",
	"#..........................................................#",
	"#..........*..............................*................#",
	"#..........................................................#",
	"2..........................................................#",
	"#..........................................................3",
	"#....................*..................*..................#",
	"#..........................................................#",
	"#..........................................................#",
	"#..........................................................#",
	"#..........................................................#",
	"#..........................................................#",
	"######################################################4#####",
}

var legend = map[rune]game.LegendEntry{
	'1': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "presentation", Label: "Presentation Wing"},
	'2': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "kraftwerk", Label: "Kraftwerk"},
	'3': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "democenter", Label: "Demo Center"},
	'4': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "arcade", Label: "Arcade"},
	'g': {Kind: game.TileObject, Ch: '≡', Object: "guestbook"},
	'-': {Kind: game.TileDecor, Ch: '▀'},
	'=': {Kind: game.TileDecor, Ch: '▄'},
	'*': {Kind: game.TileDecor, Ch: '♣'},
}

var texts = []game.MapText{
	{X: 5, Y: 2, S: "[ DURST HQ ]"},
}

func init() {
	game.Register("lobby", "Lobby", func(ctx *game.Ctx) game.Area {
		return &area{
			Walker: game.Walker{
				Ctx:    ctx,
				Map:    game.ParseMap(rows, legend, texts),
				AreaID: "lobby",
			},
			input: ui.NewTextInput("sign: ", guestbookMax),
		}
	})
}

type area struct {
	game.Walker
	bookOpen bool
	entries  []store.GuestbookEntry
	input    ui.TextInput
	signed   bool
}

func (a *area) Name() string { return "Lobby" }

func (a *area) Init(p *world.Player) tea.Cmd {
	// spawn next to the portal we came through; fresh joins enter at the
	// bottom, slightly jittered so players don't stack
	switch a.Ctx.From {
	case "presentation":
		a.Enter(29, 1, 1)
	case "kraftwerk":
		a.Enter(2, 7, 1)
	case "democenter":
		a.Enter(57, 8, 1)
	case "arcade":
		a.Enter(54, 14, 1)
	default:
		a.Enter(29, 13, 3)
	}
	return nil
}

func (a *area) CapturesInput() bool { return a.bookOpen }

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if a.bookOpen {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.Type {
			case tea.KeyEsc:
				a.bookOpen = false
				a.input.Blur()
			case tea.KeyEnter:
				text := strings.TrimSpace(a.input.Value)
				if text != "" {
					if err := a.Ctx.Store.SignGuestbook(a.Ctx.Name, text); err == nil {
						a.signed = true
					}
					a.entries = a.Ctx.Store.GuestbookEntries(5)
					a.input.Focus() // clear for another line
				}
			default:
				a.input.HandleKey(key)
			}
			return a, nil
		}
	}

	if portal, handled := a.HandleCommon(msg); handled {
		if portal != "" {
			return game.Transition{To: portal}, nil
		}
		return a, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "e" {
		if a.Map.NearObject(a.X, a.Y, "guestbook") {
			a.bookOpen = true
			a.signed = false
			a.entries = a.Ctx.Store.GuestbookEntries(5)
			a.input.Focus()
		}
	}
	return a, nil
}

func (a *area) Hint() string {
	if h := a.PortalHint(); h != "" {
		return h
	}
	if a.Map.NearObject(a.X, a.Y, "guestbook") {
		return "e — sign guestbook"
	}
	return ""
}

func (a *area) View(width, height int) string {
	view := a.Render()
	if a.bookOpen {
		panel := a.guestbookPanel()
		pw := lipgloss.Width(panel)
		view = ui.Overlay(view, panel, (a.Map.W-pw)/2, 2)
	}
	return view
}

func (a *area) guestbookPanel() string {
	var b strings.Builder
	b.WriteString(ui.PanelTitleStyle.Render("≡ Guestbook") + "\n\n")
	if len(a.entries) == 0 {
		b.WriteString(ui.DimStyle.Render("No entries yet — be the first.") + "\n")
	}
	for _, e := range a.entries {
		name := ui.ChatNameStyle.Foreground(ui.AvatarColor(e.Name)).Render(e.Name)
		b.WriteString(fmt.Sprintf("%s %s\n  %s\n",
			name,
			ui.DimStyle.Render(e.CreatedAt.Format("2006-01-02")),
			ui.ChatTextStyle.Render(truncate(e.Message, 44))))
	}
	b.WriteString("\n" + a.input.View() + "\n")
	footer := "Enter sign · Esc close"
	if a.signed {
		footer = "signed! · " + footer
	}
	b.WriteString(ui.DimStyle.Render(footer))
	return ui.PanelStyle.Render(b.String())
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
