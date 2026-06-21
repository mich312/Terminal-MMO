package game

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// key makes a rune keypress for the modal trade handler.
func tradeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// pump feeds every queued world event into a model, as the real event loop does.
func pump(m *Model, ch <-chan world.Event) {
	for {
		select {
		case ev := <-ch:
			m.handleWorldEvent(ev)
		default:
			return
		}
	}
}

// Two glyph clients sharing one world complete a full trade: request, accept,
// stage offers from each pack, both ready — and the items swap in both
// inventories.
func TestTradeEndToEndGlyph(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	mk := func(user string, inv map[string]int) (*Model, <-chan world.Event) {
		name, ev := w.Join(user)
		ctx := &Ctx{World: w, Store: store.Open(t.TempDir() + "/" + user + ".db"),
			Name: name, Theme: ui.Default, Inventory: inv, Hats: map[int]bool{}}
		m := NewModel(ctx, ev, store.VisitInfo{})
		m.areaID = "lobby"
		m.Init()
		m.width, m.height = 100, 30
		return m, ev
	}
	a, ach := mk("a", map[string]int{"crystal": 2})
	b, bch := mk("b", map[string]int{"nugget": 3})
	w.EnterArea("a", "lobby", 0, 0, "")
	w.EnterArea("b", "lobby", 1, 0, "")
	pump(a, ach)
	pump(b, bch)

	a.runChatLine("/trade b")
	pump(b, bch)
	if b.tradeReq != "a" {
		t.Fatalf("b should have a pending request from a, got %q", b.tradeReq)
	}
	b.runChatLine("/accept")
	pump(a, ach)
	pump(b, bch)
	if !a.showTrade || !b.showTrade {
		t.Fatalf("both tables should be open (a=%v b=%v)", a.showTrade, b.showTrade)
	}

	// a stages 2 crystal, b stages 3 nugget (each has one pack slot, already sel 0).
	a.handleTradeKey(tradeKey("+"))
	a.handleTradeKey(tradeKey("+"))
	b.handleTradeKey(tradeKey("+"))
	b.handleTradeKey(tradeKey("+"))
	b.handleTradeKey(tradeKey("+"))
	pump(a, ach)
	pump(b, bch)

	snap, _ := w.TradeSnapshot("a")
	if snap.YourOffer["crystal"] != 2 || snap.TheirOffer["nugget"] != 3 {
		t.Fatalf("offers not staged: %+v", snap)
	}

	// Both ready → swap commits.
	a.handleTradeKey(tradeKey("r"))
	b.handleTradeKey(tradeKey("r"))
	pump(a, ach)
	pump(b, bch)

	if a.showTrade || b.showTrade {
		t.Fatal("tables should close after the swap")
	}
	if a.ctx.Inventory["nugget"] != 3 || a.ctx.Inventory["crystal"] != 0 {
		t.Fatalf("a's inventory after trade wrong: %+v", a.ctx.Inventory)
	}
	if b.ctx.Inventory["crystal"] != 2 || b.ctx.Inventory["nugget"] != 0 {
		t.Fatalf("b's inventory after trade wrong: %+v", b.ctx.Inventory)
	}
	// Persisted too.
	if store.Open(t.TempDir() + "/x.db"); a.ctx.Store.LoadInventory("a")["nugget"] != 3 {
		t.Errorf("a's nugget not persisted: %v", a.ctx.Store.LoadInventory("a"))
	}
}

// Changing an offer after going ready clears the ready flag, so a swap can't
// sneak through.
func TestTradeReadyResetGlyph(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	mk := func(user string, inv map[string]int) (*Model, <-chan world.Event) {
		name, ev := w.Join(user)
		ctx := &Ctx{World: w, Store: store.Open(t.TempDir() + "/" + user + ".db"),
			Name: name, Theme: ui.Default, Inventory: inv, Hats: map[int]bool{}}
		m := NewModel(ctx, ev, store.VisitInfo{})
		m.areaID = "lobby"
		m.Init()
		return m, ev
	}
	a, ach := mk("a", map[string]int{"crystal": 2})
	b, bch := mk("b", map[string]int{"nugget": 3})
	w.EnterArea("a", "lobby", 0, 0, "")
	w.EnterArea("b", "lobby", 1, 0, "")
	pump(a, ach)
	pump(b, bch)
	a.runChatLine("/trade b")
	pump(b, bch)
	b.runChatLine("/accept")
	pump(a, ach)
	pump(b, bch)

	a.handleTradeKey(tradeKey("+")) // a offers 1 crystal
	a.handleTradeKey(tradeKey("r")) // a ready
	if snap, _ := w.TradeSnapshot("b"); !snap.ThemReady {
		t.Fatal("a should be ready")
	}
	b.handleTradeKey(tradeKey("+")) // b changes its offer → resets both ready
	if snap, _ := w.TradeSnapshot("a"); snap.YouReady {
		t.Fatal("a's ready should have reset when b changed the offer")
	}
}
