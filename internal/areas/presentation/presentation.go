// Package presentation is the Presentation Wing: a corridor with four
// meeting rooms, each with a wall screen whose slide index is shared world
// state — everyone in the room sees the same slide.
package presentation

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

var rows = []string{
	"############################################################",
	"#..sssssssss..#..sssssssss...#..sssssssss...#..sssssssss...#",
	"#......t......#......t.......#......t.......#......t.......#",
	"#.............#..............#..............#..............#",
	"#.............#..............#..............#..............#",
	"#.............#..............#..............#..............#",
	"#.............#..............#..............#..............#",
	"#.............#..............#..............#..............#",
	"#.............#..............#..............#..............#",
	"#.............#..............#..............#..............#",
	"#######.###############.###############.###############.####",
	"#..........................................................#",
	"0..........................................................#",
	"#..........................................................#",
	"#..........................................................#",
	"############################################################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	's': {Kind: game.TileDecor, Ch: '▀'},
	't': {Kind: game.TileObject, Ch: '▣', Walkable: true, Object: "presenter"},
}

// room describes one meeting room: its bounds and its shared-slide key.
type room struct {
	name           string
	x0, y0, x1, y1 int // interior bounds, inclusive
}

var rooms = []room{
	{"AURORA", 1, 1, 13, 9},
	{"DOLOMIT", 15, 1, 28, 9},
	{"PLOSE", 30, 1, 43, 9},
	{"KRAFTRAUM", 45, 1, 58, 9},
}

const slideCount = 3

// slides returns the hardcoded deck for a room.
func slides(roomName string) [][]string {
	return [][]string{
		{
			roomName,
			"",
			"Q3 All-Hands",
			"— Durst Group —",
		},
		{
			"Pixel to Output",
			"",
			"· ink   · paper",
			"· pixels · physics",
		},
		{
			"Questions?",
			"",
			"danke · grazie",
			"thank you",
		},
	}
}

func texts() []game.MapText {
	var out []game.MapText
	for _, r := range rooms {
		plate := "[ " + r.name + " ]"
		x := r.x0 + (r.x1-r.x0+1-len(plate))/2
		out = append(out, game.MapText{X: x, Y: 9, S: plate})
	}
	return out
}

func init() {
	game.Register("presentation", "Presentation Wing", func(ctx *game.Ctx) game.Area {
		return &area{
			Walker: game.Walker{
				Ctx:    ctx,
				Map:    game.ParseMap(rows, legend, texts()),
				AreaID: "presentation",
			},
		}
	})
}

type area struct {
	game.Walker
}

func (a *area) Name() string { return "Presentation Wing" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.Enter(2, 12, 1) // corridor, next to the lobby portal
	return nil
}

// roomAt returns the meeting room containing x,y, if any.
func roomAt(x, y int) (room, bool) {
	for _, r := range rooms {
		if x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1 {
			return r, true
		}
	}
	return room{}, false
}

func (a *area) onPresenterTile() (room, bool) {
	if a.Map.At(a.X, a.Y).Object != "presenter" {
		return room{}, false
	}
	return roomAt(a.X, a.Y)
}

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if portal, handled := a.HandleCommon(msg); handled {
		if portal != "" {
			return game.Transition{To: portal}, nil
		}
		return a, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if r, ok := a.onPresenterTile(); ok {
			switch key.String() {
			case "n":
				a.Ctx.World.ChangeSlide("presentation", r.name, +1, slideCount, a.Ctx.Name)
			case "p":
				a.Ctx.World.ChangeSlide("presentation", r.name, -1, slideCount, a.Ctx.Name)
			}
		}
	}
	return a, nil
}

func (a *area) Hint() string {
	if _, ok := a.onPresenterTile(); ok {
		return "n/p — next/previous slide"
	}
	if h := a.PortalHint(); h != "" {
		return h
	}
	if _, ok := roomAt(a.X, a.Y); ok {
		return "stand on ▣ to present"
	}
	return ""
}

func (a *area) View(width, height int) string {
	view := a.Render()
	if r, ok := roomAt(a.X, a.Y); ok {
		panel := a.slidePanel(r)
		pw := lipgloss.Width(panel)
		view = ui.Overlay(view, panel, (a.Map.W-pw)/2, 1)
	}
	return view
}

// slidePanel renders the room's 20×6 wall screen with the current slide.
func (a *area) slidePanel(r room) string {
	idx := a.Ctx.World.Slide(r.name)
	deck := slides(r.name)
	if idx >= len(deck) {
		idx = len(deck) - 1
	}

	const w, h = 20, 6
	body := make([]string, 0, h)
	for _, line := range deck[idx] {
		body = append(body, lipgloss.PlaceHorizontal(w, lipgloss.Center, line))
	}
	for len(body) < h-1 {
		body = append(body, strings.Repeat(" ", w))
	}
	footer := fmt.Sprintf("%s · Slide %d/%d", r.name, idx+1, len(deck))
	body = append(body, ui.DimStyle.Render(lipgloss.PlaceHorizontal(w, lipgloss.Center, footer)))

	screen := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorAccent).
		Padding(0, 1).
		Render(strings.Join(body[:h], "\n"))
	return screen
}
