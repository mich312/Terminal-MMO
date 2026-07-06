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
	"fmt"
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
	"#.............H..............#",
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
	// The Hall of Fame plinth: press e beside it to read the leaderboards.
	'H': {Kind: game.TileObject, Ch: '≡', Object: "scores", Label: "High Scores",
		Color: "#FFD166", Tex: game.TexFloor, Ground: "#241F30", Prop: game.PropPlinth, PropHex: "#FFD166"},
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
	boardOpen bool // the Hall of Fame panel is up
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

// CapturesInput keeps global keys away while the Hall of Fame is up.
func (a *area) CapturesInput() bool { return a.boardOpen }

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	if a.boardOpen {
		if _, ok := msg.(tea.KeyMsg); ok {
			a.boardOpen = false // any key closes the board
		}
		return a, nil
	}
	if portal, handled := a.HandleCommon(msg); handled && portal != "" {
		return game.Transition{To: portal}, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "e" &&
		a.Map.NearObject(a.X, a.Y, "scores") {
		a.boardOpen = true
	}
	return a, nil
}

func (a *area) Hint() string {
	if h := a.PortalHint(); h != "" {
		return h
	}
	if a.Map.NearObject(a.X, a.Y, "scores") {
		return "e — the Hall of Fame"
	}
	return "walk into a cabinet to play · ◈ door leaves"
}

// scoreLines renders the leaderboards as rows of (header, entries) — the
// shared source both the glyph panel and the HD slide format from.
func (a *area) scoreLines() []struct {
	Title   string
	Entries []string
} {
	out := make([]struct {
		Title   string
		Entries []string
	}, 0, len(game.ScoreGames))
	for _, g := range game.ScoreGames {
		rows := a.Ctx.Store.TopScores(g.ID, 3)
		entries := make([]string, 0, len(rows))
		for i, hs := range rows {
			entries = append(entries, fmt.Sprintf("%d. %s — %d %s", i+1, hs.Name, hs.Score, g.Unit))
		}
		out = append(out, struct {
			Title   string
			Entries []string
		}{game.DisplayName(g.ID), entries})
	}
	return out
}

// scoresPanel is the glyph client's Hall of Fame overlay.
func (a *area) scoresPanel() string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	var b strings.Builder
	b.WriteString(th.PanelTitle.Render("🏆 Hall of Fame") + "\n")
	for _, g := range a.scoreLines() {
		b.WriteString("\n" + th.Accent.Render(g.Title) + "\n")
		if len(g.Entries) == 0 {
			b.WriteString(th.Dim.Render("  no scores yet — step in!") + "\n")
			continue
		}
		for i, e := range g.Entries {
			if i == 0 {
				b.WriteString("  " + th.Bright.Render(e) + "\n")
			} else {
				b.WriteString("  " + th.ChatText.Render(e) + "\n")
			}
		}
	}
	b.WriteString("\n" + th.Dim.Render("any key to close"))
	return th.Panel.Render(b.String())
}

// HDSlide implements game.HDOverlayer: the Hall of Fame rendered as a markdown
// panel over the HD frame, exactly like the Wilds' notice board.
func (a *area) HDSlide() (string, string, bool) {
	if !a.boardOpen {
		return "", "", false
	}
	var b strings.Builder
	b.WriteString("# Hall of Fame\n")
	for _, g := range a.scoreLines() {
		b.WriteString("\n## " + g.Title + "\n\n")
		if len(g.Entries) == 0 {
			b.WriteString("_no scores yet — step in!_\n")
			continue
		}
		for _, e := range g.Entries {
			b.WriteString(e + "\n\n")
		}
	}
	return b.String(), "any key to close", true
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
	view := lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
	if a.boardOpen {
		board := a.scoresPanel()
		view = ui.Overlay(view, board, (width-lipgloss.Width(board))/2, 1)
	}
	return view
}
