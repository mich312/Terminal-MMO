// Package presentation is the Presentation Wing: a concourse of stages that
// grows as players add presentations. Anyone can walk to the ＋ booth and author
// a markdown deck in world; it becomes a new stage with a big screen. Everyone
// standing in a stage sees the same slide, and the deck's owner drives it from
// the lectern. The wing's layout is rebuilt from the live list of decks, so the
// structure is dynamic.
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

const (
	bayW       = 18 // width of one stage bay
	stageH     = 10 // stage chamber height
	concourseH = 4  // walkway height along the bottom
)

// legend drives both renderers: Ch/Color/Anim are the glyph look; Tex/Ground/
// Prop are the HD pixel look.
var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#23262E"},
	'#': {Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#3A3550"},
	// Carpet aisle (walkable, deep violet).
	'r': {Kind: game.TileFloor, Ch: '▒', Walkable: true, Color: "#3A2E5A", Tex: game.TexFloor, Ground: "#3A2E5A"},
	// Big stage screen across the back wall (HD: glowing display).
	'S': {Kind: game.TileDecor, Ch: '▀', Tex: game.TexBrick, Ground: "#10141B", Prop: game.PropScreen, PropHex: "#2E8BFF", Anim: &game.TileAnim{
		ColorA: "#2E6BFF", ColorB: "#7DF0FF", Speed: 3}},
	// Lectern: the presenter's podium (walkable object).
	'L': {Kind: game.TileObject, Ch: '▟', Walkable: true, Object: "lectern", Tex: game.TexFloor, Ground: "#23262E", Prop: game.PropCrate, PropHex: "#A8854C"},
	// ＋ booth: stand here to create a new presentation (walkable object).
	'+': {Kind: game.TileObject, Ch: '✚', Walkable: true, Object: "create", Tex: game.TexFloor, Ground: "#1C3A2E", Prop: game.PropPlinth, PropHex: "#7BD88F"},
	// Audience seating.
	'c': {Kind: game.TileDecor, Ch: '▪', Color: "#5A536E", Tex: game.TexFloor, Ground: "#23262E", Prop: game.PropCrate, PropHex: "#5A536E"},
}

// stage is one bay: a deck stage or the create booth.
type stage struct {
	deckID         string
	x0, y0, x1, y1 int // chamber interior bounds (inclusive)
	lx, ly         int // lectern / booth tile (left cell of a 2-wide spot)
	booth          bool
}

// buildWing lays out the concourse and one bay per deck, plus a trailing create
// booth. Bays are append-only so existing positions stay valid across rebuilds.
func buildWing(decks []world.Deck) ([]string, []game.MapText, []stage) {
	nbays := len(decks) + 1
	W := nbays*bayW + 1
	H := stageH + concourseH + 3
	g := make([][]rune, H)
	for y := range g {
		g[y] = make([]rune, W)
		for x := range g[y] {
			g[y][x] = '#'
		}
	}
	set := func(x, y int, r rune) {
		if x >= 0 && x < W && y >= 0 && y < H {
			g[y][x] = r
		}
	}
	cy0 := stageH + 2
	for y := cy0; y < cy0+concourseH; y++ {
		for x := 1; x < W-1; x++ {
			set(x, y, '.')
		}
	}
	set(0, cy0+1, '0') // lobby portal

	var stages []stage
	var texts []game.MapText
	for i := 0; i < nbays; i++ {
		x0 := i*bayW + 1
		x1 := x0 + bayW - 2
		for y := 1; y <= stageH; y++ {
			for x := x0; x <= x1; x++ {
				set(x, y, '.')
			}
		}
		dcx := (x0 + x1) / 2
		set(dcx, stageH+1, '.') // 2-wide door into the concourse
		set(dcx+1, stageH+1, '.')
		for y := 2; y <= stageH; y++ { // carpet aisle
			set(dcx, y, 'r')
			set(dcx+1, y, 'r')
		}
		ly := stageH - 2
		if i == len(decks) { // the create booth
			set(dcx, ly, '+')
			set(dcx+1, ly, '+')
			plate := "＋ New presentation"
			if len(decks) >= world.MaxDecks {
				plate = fmt.Sprintf("Wing full %d/%d", len(decks), world.MaxDecks)
			}
			texts = append(texts, centered(plate, x0, x1, stageH))
			stages = append(stages, stage{booth: true, x0: x0, y0: 1, x1: x1, y1: stageH, lx: dcx, ly: ly})
			continue
		}
		for x := x0 + 2; x <= x1-2; x++ { // screen across the back wall
			set(x, 1, 'S')
		}
		set(dcx, ly, 'L')
		set(dcx+1, ly, 'L')
		for _, sy := range []int{4, 6} { // seating flanking the aisle
			for x := x0 + 2; x < dcx-1; x += 2 {
				set(x, sy, 'c')
			}
			for x := dcx + 3; x <= x1-2; x += 2 {
				set(x, sy, 'c')
			}
		}
		d := decks[i]
		texts = append(texts, centered("[ "+d.Title+" ]", x0, x1, stageH))
		stages = append(stages, stage{deckID: d.ID, x0: x0, y0: 1, x1: x1, y1: stageH, lx: dcx, ly: ly})
	}
	rows := make([]string, H)
	for y := range g {
		rows[y] = string(g[y])
	}
	return rows, texts, stages
}

func centered(s string, x0, x1, y int) game.MapText {
	x := x0 + (x1-x0+1-len([]rune(s)))/2
	if x < x0 {
		x = x0
	}
	return game.MapText{X: x, Y: y, S: s}
}

func init() {
	game.Register("presentation", "Presentation Wing", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "presentation"}}
	})
}

type editMode int

const (
	modeWalk editMode = iota
	modeTitle
	modeEdit
)

type area struct {
	game.Walker
	stages []stage

	mode         editMode
	title        ui.TextInput
	editor       ui.Editor
	editID       string // deck being edited; "" when creating
	pendingTitle string
	retireArmed  bool // a retire keypress is awaiting confirmation
}

func (a *area) Name() string { return "Presentation Wing" }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.title = ui.NewTextInput("title: ", 48)
	a.rebuild()
	a.Enter(3, stageH+3, 0) // concourse, by the lobby portal
	return nil
}

// rebuild regenerates the map from the live deck list. Positions are stable
// (bays only ever append), so the player stays put.
func (a *area) rebuild() {
	rows, texts, stages := buildWing(a.Ctx.World.Decks())
	a.Map = game.ParseMap(rows, legend, texts)
	a.stages = stages
}

func (a *area) stageAt(x, y int) (stage, bool) {
	for _, s := range a.stages {
		if x >= s.x0 && x <= s.x1 && y >= s.y0 && y <= s.y1 {
			return s, true
		}
	}
	return stage{}, false
}

func (a *area) onLectern() (stage, bool) {
	switch a.Map.At(a.X, a.Y).Object {
	case "lectern", "create":
		return a.stageAt(a.X, a.Y)
	}
	return stage{}, false
}

// CapturesInput grabs every key while a modal (title prompt or editor) is open.
func (a *area) CapturesInput() bool { return a.mode != modeWalk }

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	switch a.mode {
	case modeTitle:
		if key, ok := msg.(tea.KeyMsg); ok {
			a.titleKey(key)
		}
		return a, nil
	case modeEdit:
		if key, ok := msg.(tea.KeyMsg); ok {
			a.editKey(key)
		}
		return a, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() != "x" { // any other key cancels a pending retire
			a.retireArmed = false
		}
		if st, ok := a.onLectern(); ok {
			if st.booth {
				if s := key.String(); s == "e" || s == "enter" {
					if a.Ctx.World.DeckCount() >= world.MaxDecks {
						return a, nil // wing full — the Hint explains
					}
					a.openTitle()
					return a, nil
				}
			} else if d, ok := a.Ctx.World.GetDeck(st.deckID); ok && d.Owner == a.Ctx.Name {
				switch key.String() {
				case "e":
					a.openEditor(d)
					return a, nil
				case "n":
					a.Ctx.World.AdvanceDeck(st.deckID, a.Ctx.Name, +1)
					return a, nil
				case "p":
					a.Ctx.World.AdvanceDeck(st.deckID, a.Ctx.Name, -1)
					return a, nil
				case "x":
					if a.retireArmed {
						a.Ctx.World.RemoveDeck(st.deckID, a.Ctx.Name)
						a.retireArmed = false
						a.rebuildSafe()
					} else {
						a.retireArmed = true
					}
					return a, nil
				}
			}
		}
	}

	if wm, ok := msg.(game.WorldEventMsg); ok && world.Event(wm).Type == world.EventDeck {
		a.rebuildSafe()
	}
	if portal, handled := a.HandleCommon(msg); handled {
		if portal != "" {
			return game.Transition{To: portal}, nil
		}
		return a, nil
	}
	return a, nil
}

func (a *area) openTitle() {
	a.editID = ""
	a.pendingTitle = ""
	a.title.Focus()
	a.mode = modeTitle
}

func (a *area) openEditor(d world.Deck) {
	a.editID = d.ID
	a.pendingTitle = d.Title
	a.editor = ui.NewEditor(d.Source)
	a.editor.Focus()
	a.mode = modeEdit
}

func (a *area) titleKey(key tea.KeyMsg) {
	switch key.Type {
	case tea.KeyEnter:
		t := strings.TrimSpace(a.title.Value)
		if t == "" {
			return
		}
		a.pendingTitle = t
		a.editor = ui.NewEditor(deckTemplate(t))
		a.editor.Focus()
		a.title.Blur()
		a.mode = modeEdit
	case tea.KeyEsc:
		a.title.Blur()
		a.mode = modeWalk
	default:
		a.title.HandleKey(key)
	}
}

func (a *area) editKey(key tea.KeyMsg) {
	switch key.Type {
	case tea.KeyCtrlS:
		src := a.editor.Value()
		if a.editID == "" {
			if a.Ctx.World.CreateDeck(a.Ctx.Name, a.pendingTitle, src) == "" {
				return // the wing filled while editing — keep the editor open so the work isn't lost
			}
		} else {
			a.Ctx.World.UpdateDeck(a.editID, a.Ctx.Name, a.pendingTitle, src)
		}
		a.editor.Blur()
		a.mode = modeWalk
		a.rebuildSafe()
	case tea.KeyEsc:
		a.editor.Blur()
		a.mode = modeWalk
	default:
		a.editor.HandleKey(key)
	}
}

// rebuildSafe regenerates the map and keeps the local player sensibly placed.
// Retiring an earlier deck shifts every later bay left, so a player can end up
// on a wall (handled) or — worse — silently standing in a *different* stage than
// the one they were watching. In either case we step them down to the concourse
// near their current column rather than mis-attribute them to someone else's
// talk.
func (a *area) rebuildSafe() {
	prev, wasInStage := a.stageAt(a.X, a.Y)
	a.rebuild()
	now, inStage := a.stageAt(a.X, a.Y)
	shifted := wasInStage && inStage && now.deckID != prev.deckID
	if a.fits(a.X, a.Y) && !shifted {
		return
	}
	cy := stageH + 3 // a concourse row
	for d := 0; d < a.Map.W; d++ {
		for _, x := range [2]int{a.X - d, a.X + d} {
			if a.fits(x, cy) {
				a.X, a.Y = x, cy
				a.Ctx.World.Move(a.Ctx.Name, x, cy)
				return
			}
		}
	}
}

func (a *area) fits(x, y int) bool {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			if !a.Map.Walkable(x+dx, y+dy) {
				return false
			}
		}
	}
	return true
}

func (a *area) Hint() string {
	if a.mode != modeWalk {
		return ""
	}
	if st, ok := a.onLectern(); ok {
		if st.booth {
			if a.Ctx.World.DeckCount() >= world.MaxDecks {
				return fmt.Sprintf("wing full (%d/%d) — a presenter must retire a talk", world.MaxDecks, world.MaxDecks)
			}
			return "e — author a presentation"
		}
		if d, ok := a.Ctx.World.GetDeck(st.deckID); ok {
			if d.Owner == a.Ctx.Name {
				if a.retireArmed {
					return "press x again to retire “" + d.Title + "” · move to cancel"
				}
				return "n/p — slides · e — edit · x — retire"
			}
			return "presented by " + d.Owner
		}
	}
	if st, ok := a.stageAt(a.X, a.Y); ok && !st.booth {
		if d, ok := a.Ctx.World.GetDeck(st.deckID); ok {
			return "▸ " + d.Title + " — stand on the ▟ lectern to present"
		}
	}
	if h := a.PortalHint(); h != "" {
		return h
	}
	return "walk into a stage to watch · ＋ booth adds one"
}

// Prompt implements game.Prompter: reuse Hint, but suppress the idle "walk into
// a stage" fallback so the HD bottom stays clear until you're actually on a
// lectern, booth, stage or portal.
func (a *area) Prompt() (string, bool) {
	h := a.Hint()
	if h == "" || h == "walk into a stage to watch · ＋ booth adds one" {
		return "", false
	}
	return h, true
}

// HDSlide implements game.HDOverlayer: when the player stands in a deck stage,
// the current slide's markdown is drawn on the big screen in HD pixel mode.
func (a *area) HDSlide() (string, string, bool) {
	st, ok := a.stageAt(a.X, a.Y)
	if !ok || st.booth {
		return "", "", false
	}
	d, ok := a.Ctx.World.GetDeck(st.deckID)
	if !ok || len(d.Slides) == 0 {
		return "", "", false
	}
	cur := d.Current
	if cur < 0 {
		cur = 0
	}
	if cur > len(d.Slides)-1 {
		cur = len(d.Slides) - 1
	}
	footer := fmt.Sprintf("%s  ·  slide %d/%d  ·  %s", d.Title, cur+1, len(d.Slides), d.Owner)
	return d.Slides[cur], footer, true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	base := a.RenderViewport(width, height)

	switch a.mode {
	case modeEdit:
		w := width - 6
		if w > 82 {
			w = 82
		}
		return overlay(base, a.editor.View(th, "✎ "+a.pendingTitle, w, height-1), width, height)
	case modeTitle:
		panel := th.Panel.Render(
			th.PanelTitle.Render("New presentation") + "\n\n" +
				a.title.View() + "\n\n" +
				th.Dim.Render("Enter to write slides · Esc cancel"))
		return overlay(base, panel, width, height)
	}

	if st, ok := a.stageAt(a.X, a.Y); ok && !st.booth {
		if d, ok := a.Ctx.World.GetDeck(st.deckID); ok {
			panel := a.screenPanel(th, d, width, height)
			pw := lipgloss.Width(panel)
			return ui.Overlay(base, panel, (width-pw)/2, 1)
		}
	}
	return base
}

// screenPanel renders the deck's current slide as the stage's big screen.
func (a *area) screenPanel(th *ui.Theme, d world.Deck, width, height int) string {
	cur := d.Current
	if cur < 0 {
		cur = 0
	}
	if cur > len(d.Slides)-1 {
		cur = len(d.Slides) - 1
	}
	sw := width - 16
	if sw > 60 {
		sw = 60
	}
	if sw < 28 {
		sw = 28
	}
	sh := height/2 + 1
	if sh < 7 {
		sh = 7
	}
	if sh > 15 {
		sh = 15
	}
	body := th.RenderSlide(d.Slides[cur], sw, sh)
	footer := th.Dim.Render(lipgloss.PlaceHorizontal(sw, lipgloss.Center,
		fmt.Sprintf("%s — slide %d/%d · ▸ %s", d.Title, cur+1, len(d.Slides), d.Owner)))
	return th.Screen().Render(body + "\n" + footer)
}

func overlay(base, panel string, width, height int) string {
	pw, ph := lipgloss.Width(panel), lipgloss.Height(panel)
	return ui.Overlay(base, panel, (width-pw)/2, (height-ph)/2)
}

func deckTemplate(title string) string {
	return "# " + title + "\n\nyour opening line\n\n---\n\n## Agenda\n\n- first point\n- second point\n\n---\n\n## Thanks\n\nQuestions?\n"
}
