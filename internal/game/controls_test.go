package game

import (
	"image"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// CommandReference must mirror the registered commands exactly — it's the source
// the HD client and the "?" overlay render from, so a drift would document keys
// that don't exist (or hide ones that do).
func TestCommandReferenceMirrorsCommands(t *testing.T) {
	ref := CommandReference()
	if len(ref) != len(commands) {
		t.Fatalf("CommandReference has %d entries, want %d", len(ref), len(commands))
	}
	for i, c := range commands {
		if ref[i][0] != c.usage || ref[i][1] != c.summary {
			t.Errorf("entry %d = %q/%q, want %q/%q", i, ref[i][0], ref[i][1], c.usage, c.summary)
		}
	}
}

// Controls must cover the keys players actually need to discover — movement, the
// interact key, and the "?" overlay itself — so the help is self-referential.
func TestControlsCoverCoreKeys(t *testing.T) {
	var keys []string
	for _, g := range Controls() {
		if g.Title == "" {
			t.Error("control group with empty title")
		}
		for _, c := range g.Items {
			keys = append(keys, c.Keys)
		}
	}
	joined := strings.Join(keys, " ")
	for _, want := range []string{"WASD", "e", "Enter", "?", "q"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Controls() is missing the %q key", want)
		}
	}
}

// The HD menu hub's selection handler (openMenuSel in cmd/durstworld) maps a row
// index to a panel, so the entry list's length and order are load-bearing.
func TestMenuEntriesShape(t *testing.T) {
	e := MenuEntries()
	if len(e) != 5 {
		t.Fatalf("MenuEntries has %d rows; the HD menu switch expects 5", len(e))
	}
	want := []string{"Compendium", "Crafting", "Character", "Who's online", "Controls & Help"}
	for i, w := range want {
		if e[i].Label != w {
			t.Errorf("row %d = %q, want %q (order is load-bearing)", i, e[i].Label, w)
		}
	}
}

// "?" opens the glyph help overlay, and it must list both the control reference
// and the chat commands.
func TestQuestionKeyOpensHelp(t *testing.T) {
	m := playingModel(t)
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.showInfo || m.infoTitle != "Help" {
		t.Fatalf("? should open the Help overlay; showInfo=%v title=%q", m.showInfo, m.infoTitle)
	}
	body := strings.Join(m.infoLines, "\n")
	for _, want := range []string{"Move", "WASD", "Chat commands", "/who"} {
		if !strings.Contains(body, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
}

// The HD help and who panels draw straight onto an RGBA frame; they must lay out
// without panicking and actually touch pixels (so the player sees something).
func TestHDHelpAndWhoPanelsRender(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	w.Join("bob")
	ctx := &Ctx{World: w, Store: store.Open(t.TempDir() + "/c.db"), Name: name,
		Inventory: map[string]int{}, Hats: map[int]bool{}}

	for _, c := range []struct {
		name string
		draw func(*image.RGBA)
	}{
		{"help", func(img *image.RGBA) { DrawHelpPanel(img, ctx) }},
		{"who", func(img *image.RGBA) { DrawWhoPanel(img, ctx) }},
		{"craft-empty", func(img *image.RGBA) { DrawCraftPanel(img, ctx, 0) }},
		{"craft-stocked", func(img *image.RGBA) {
			ctx.Inventory = map[string]int{"wood": 7, "herb": 1, "mushroom": 1}
			DrawCraftPanel(img, ctx, len(Recipes)-1)
			ctx.Inventory = map[string]int{}
		}},
		{"machine", func(img *image.RGBA) {
			w.EnterArea(name, "wilds", 0, 0, "")
			w.Place("wilds", world.Placement{X: 4, Y: 4, Kind: "furnace", Owner: name})
			ctx.Inventory = map[string]int{"nugget": 6}
			RefuelMachine(ctx, 4, 4)
			DrawMachinePanel(img, ctx, 4, 4, 3, 6)
			ctx.Inventory = map[string]int{}
		}},
		{"stall", func(img *image.RGBA) {
			w.EnterArea(name, "wilds", 0, 0, "")
			w.Place("wilds", world.Placement{X: 6, Y: 6, Kind: "stall", Owner: name})
			ctx.Inventory = map[string]int{"plank": 20, "stone": 4}
			AddOffer(ctx, 6, 6, "plank", 5, "stone", 3)
			DrawStallPanel(img, ctx, 6, 6, 0)
			ctx.Inventory = map[string]int{}
		}},
	} {
		img := image.NewRGBA(image.Rect(0, 0, 900, 560))
		c.draw(img)
		drawn := false
		for _, b := range img.Pix {
			if b != 0 {
				drawn = true
				break
			}
		}
		if !drawn {
			t.Errorf("%s panel drew nothing", c.name)
		}
	}
}
