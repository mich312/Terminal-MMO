// Package game holds the per-session root bubbletea model, the Area
// interface every scene implements, and the shared tilemap machinery.
package game

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Area is one walkable scene (or full-screen panel). The root model swaps
// areas when Update returns a different one — typically a Transition.
type Area interface {
	Name() string
	Init(player *world.Player) tea.Cmd
	Update(msg tea.Msg) (Area, tea.Cmd) // may return a different Area to transition
	View(width, height int) string
}

// Hinter lets an area surface a contextual status-bar hint
// ("↪ Presentation Wing — walk in to enter", "e — sign guestbook", …).
type Hinter interface {
	Hint() string
}

// Prompter lets an area expose the single contextual action available right
// where the player stands ("e — wear the Crown", "step in to enter Durst HQ").
// ok is false when nothing is actionable, so the HD client keeps the bottom of
// the screen clear. Areas that don't implement it fall back to a non-empty
// Hint() being treated as the prompt.
type Prompter interface {
	Prompt() (text string, ok bool)
}

// InputCapturer lets an area grab all key input (e.g. while the guestbook
// panel is open) so the root model's global keys don't interfere.
type InputCapturer interface {
	CapturesInput() bool
}

// HDViewer is an area the HD pixel renderer can draw: it returns a vw×vh tile
// window centered on the local player, plus the window's absolute top-left
// origin (players carry absolute world coordinates, so the renderer maps them
// onto the window with this origin). Walker-based areas and the Wilds implement
// it; panel-only areas (the Arcade stub) don't, so HD mode skips them.
type HDViewer interface {
	HDView(vw, vh int) (window *TileMap, originX, originY int)
}

// HDLighter lets an HD area supply a radial light — the Wilds uses it for the
// discovery circle around the player. Areas that don't implement it render at
// full brightness.
type HDLighter interface {
	HDLight() Light
}

// Toaster is an area that surfaces a transient one-line message (e.g. an item
// pickup). Both renderers poll it: the glyph View overlays it and the HD loop
// draws it onto the frame.
type Toaster interface {
	Toast() (text string, show bool)
}

// HDOverlayer lets an area draw a text panel over the HD pixel frame. The
// Presentation Wing uses it to show the current slide on screen in HD (there
// are no terminal cells in HD, so the markdown is rendered into the image). It
// returns the slide's markdown source, a footer, and whether to show it.
type HDOverlayer interface {
	HDSlide() (src string, footer string, show bool)
}

// Ctx is everything an area needs: shared world, persistence, and who the
// local player is. From is the area id the player came from ("" on a fresh
// connect) so areas can spawn players next to the right portal.
type Ctx struct {
	World *world.World
	Store store.Store
	Name  string
	From  string
	// Theme is the player's per-session, auto-detecting style set. Nil-safe:
	// rendering falls back to ui.Default when unset (e.g. in tests).
	Theme *ui.Theme
	// Inventory is the session's collected items (id → count), shared between
	// the Wilds pickup logic and the /inventory command. Loaded at join.
	Inventory map[string]int
	// Hats is the set of accessory indices the player has unlocked (found in the
	// world). Index 0 ("none") is always available. Loaded at join.
	Hats map[int]bool
	// FixedGates is the set of personal gate ids this player has repaired.
	// Loaded at join. Co-op gate state lives in the shared World instead.
	FixedGates map[string]bool
}

// Accessory is the index of the accessory the player is currently wearing (0 =
// none), read from live world state — so an area can give a worn thing a power
// (a light, surer footing, a keener eye) the same way the renderer reads it.
func (c *Ctx) Accessory() int {
	if c.World == nil {
		return 0
	}
	if p, ok := c.World.Self(c.Name); ok {
		return p.Accessory
	}
	return 0
}

// Wearing reports whether the player currently wears the named accessory.
func (c *Ctx) Wearing(name string) bool {
	idx, ok := AccessoryIndex(name)
	return ok && c.Accessory() == idx
}

// ForagerBoon reports whether the player wears a gatherer's wearable — the
// meadow flower or the lantern-cap — which yields a richer haul from each node.
func (c *Ctx) ForagerBoon() bool {
	return c.Wearing("flower") || c.Wearing("glowcap")
}

// Transition is a sentinel Area: returning it from Update tells the root
// model to construct the destination area from the registry and swap to it.
type Transition struct{ To string }

func (Transition) Name() string                     { return "" }
func (Transition) Init(*world.Player) tea.Cmd       { return nil }
func (t Transition) Update(tea.Msg) (Area, tea.Cmd) { return t, nil }
func (Transition) View(int, int) string             { return "" }

// Factory builds an area for one session.
type Factory func(ctx *Ctx) Area

var (
	registry     = map[string]Factory{}
	displayNames = map[string]string{}
)

// Register adds an area factory under an id with its display name. Areas
// self-register in init(); main imports them for the side effect.
func Register(id, display string, f Factory) {
	registry[id] = f
	displayNames[id] = display
}

// AreaRegistered reports whether an area id has a factory.
func AreaRegistered(id string) bool {
	_, ok := registry[id]
	return ok
}

// RegisteredAreas returns all registered area ids, sorted, for /goto and help.
func RegisteredAreas() []string {
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// DisplayName resolves an area id to its human name.
func DisplayName(id string) string {
	if d, ok := displayNames[id]; ok {
		return d
	}
	return id
}

// NewArea instantiates a registered area; unknown ids fall back to the
// lobby so a bad portal can never strand a player.
func NewArea(id string, ctx *Ctx) Area {
	if f, ok := registry[id]; ok {
		return f(ctx)
	}
	return registry["lobby"](ctx)
}

// WorldEventMsg wraps a world.Event as a tea.Msg. The root model receives
// it from the subscription command and forwards it to the active area.
type WorldEventMsg world.Event

// EventsClosedMsg means the session's event channel closed (player removed).
type EventsClosedMsg struct{}

// WaitForEvent is the subscription-command pattern: block on the session's
// event channel and hand the next event to Update, which re-issues it.
func WaitForEvent(ch <-chan world.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return EventsClosedMsg{}
		}
		return WorldEventMsg(ev)
	}
}
