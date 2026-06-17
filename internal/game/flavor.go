package game

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// FlavorArea is a small fixed walkable room: a hand-drawn map with a portal
// back out and a descriptive side panel, nothing else. It implements Area in
// full, so such a room is just map data plus one RegisterFlavor call — the
// simplest way to add a scene. Kraftwerk and the Demo Center are built this way.
type FlavorArea struct {
	Walker
	title, body            string
	spawnX, spawnY, jitter int
	panelWidth             int
	panelLeft              bool // panel sits left of the map (else right)
	light                  int  // radial light radius; 0 = fully lit
}

// FlavorConfig describes one flavor room. Zero values are sensible: PanelWidth
// defaults to 34, Light 0 means no shadow, PanelLeft false puts the map first.
type FlavorConfig struct {
	ID, Display            string
	Rows                   []string
	Legend                 map[rune]LegendEntry
	SpawnX, SpawnY, Jitter int
	Title, Body            string
	PanelWidth             int
	PanelLeft              bool
	Light                  int
}

// RegisterFlavor registers a flavor room with the area registry. Call it from
// the area package's init().
func RegisterFlavor(c FlavorConfig) {
	Register(c.ID, c.Display, func(ctx *Ctx) Area { return newFlavorArea(ctx, c) })
}

func newFlavorArea(ctx *Ctx, c FlavorConfig) *FlavorArea {
	pw := c.PanelWidth
	if pw == 0 {
		pw = 34
	}
	return &FlavorArea{
		Walker:     Walker{Ctx: ctx, Map: ParseMap(c.Rows, c.Legend, nil), AreaID: c.ID},
		title:      c.Title,
		body:       c.Body,
		spawnX:     c.SpawnX,
		spawnY:     c.SpawnY,
		jitter:     c.Jitter,
		panelWidth: pw,
		panelLeft:  c.PanelLeft,
		light:      c.Light,
	}
}

func (a *FlavorArea) Name() string { return DisplayName(a.AreaID) }

func (a *FlavorArea) Init(p *world.Player) tea.Cmd {
	a.Enter(a.spawnX, a.spawnY, a.jitter)
	return nil
}

func (a *FlavorArea) Update(msg tea.Msg) (Area, tea.Cmd) {
	if portal, handled := a.HandleCommon(msg); handled && portal != "" {
		return Transition{To: portal}, nil
	}
	return a, nil
}

func (a *FlavorArea) Hint() string { return a.PortalHint() }

func (a *FlavorArea) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	panel := th.Panel.Width(a.panelWidth).Render(
		th.PanelTitle.Render(a.title) + "\n\n" + th.ChatText.Render(a.body))

	mapView := a.Render()
	if a.light > 0 {
		mapView = a.RenderLit(a.Map.W, a.Map.H, a.light)
	}

	if a.panelLeft {
		return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, mapView, "   ", panel)
}
