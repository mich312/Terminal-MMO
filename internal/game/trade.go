package game

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/durst-group/durstworld/internal/world"
)

// Client-side trade glue shared by both renderers: it turns the world's live
// trade snapshot into a TradeView the panels draw, stages offers against the
// player's own pack, and applies a finished swap to the inventory. The
// negotiation itself lives in the world; this is the per-session half.

// errNotTrading is returned when a trade action is taken with no open table.
var errNotTrading = errors.New("you're not in a trade")

// TradeViewFor builds the panel view for the player's current trade, with the
// given pack slot selected. ok is false when there's no open table.
func TradeViewFor(ctx *Ctx, sel int) (TradeView, bool) {
	snap, ok := ctx.World.TradeSnapshot(ctx.Name)
	if !ok {
		return TradeView{}, false
	}
	you, _ := ctx.World.Self(ctx.Name)
	them, _ := ctx.World.Self(snap.With)
	them.Name = snap.With // in case they've gone (rendered until the cancel lands)
	return TradeView{
		You:  tradeParty(you, snap.YourOffer, snap.YouReady),
		Them: tradeParty(them, snap.TheirOffer, snap.ThemReady),
		Pack: packRows(ctx.Inventory),
		Sel:  sel,
	}, true
}

func tradeParty(p world.Player, offer map[string]int, ready bool) TradeParty {
	return TradeParty{
		Name:      p.Name,
		Style:     p.Style,
		Accessory: p.Accessory,
		Color:     p.Color,
		Offer:     offerRows(offer),
		Ready:     ready,
	}
}

// offerRows lists an offer in catalog order so the table is stable as it changes.
func offerRows(offer map[string]int) []TradeRow {
	var rows []TradeRow
	for _, it := range Items {
		if n := offer[it.ID]; n > 0 {
			rows = append(rows, TradeRow{Item: it, N: n})
		}
	}
	return rows
}

// packRows lists the player's holdings in catalog order for the offer picker.
func packRows(inv map[string]int) []TradeRow {
	var rows []TradeRow
	for _, it := range Items {
		if n := inv[it.ID]; n > 0 {
			rows = append(rows, TradeRow{Item: it, N: n})
		}
	}
	return rows
}

// AdjustOffer moves `delta` of an item on or off the player's side of the table,
// clamped to what they actually hold. It rewrites the whole offer (which resets
// both ready flags in the world).
func AdjustOffer(ctx *Ctx, item string, delta int) error {
	snap, ok := ctx.World.TradeSnapshot(ctx.Name)
	if !ok {
		return errNotTrading
	}
	offer := snap.YourOffer
	if offer == nil {
		offer = map[string]int{}
	}
	n := offer[item] + delta
	if n < 0 {
		n = 0
	}
	if have := ctx.Inventory[item]; n > have {
		n = have
	}
	if n == 0 {
		delete(offer, item)
	} else {
		offer[item] = n
	}
	return ctx.World.SetOffer(ctx.Name, offer)
}

// ApplyCompletedTrade drains a just-finished swap and applies it to the player's
// inventory (and the store), returning a human summary. ok is false if there's
// nothing to apply.
func ApplyCompletedTrade(ctx *Ctx) (string, bool) {
	c, ok := ctx.World.TakeCompletedTrade(ctx.Name)
	if !ok {
		return "", false
	}
	if ctx.Inventory == nil {
		ctx.Inventory = map[string]int{}
	}
	for id, n := range c.Gave {
		for i := 0; i < n; i++ {
			ctx.Store.SpendItem(ctx.Name, id)
		}
		if ctx.Inventory[id] -= n; ctx.Inventory[id] <= 0 {
			delete(ctx.Inventory, id)
		}
	}
	for id, n := range c.Got {
		for i := 0; i < n; i++ {
			ctx.Store.AddItem(ctx.Name, id)
		}
		ctx.Inventory[id] += n
	}
	return tradeSummary(c), true
}

// tradeSummary renders a one-line recap of a finished swap for the chat log.
func tradeSummary(c world.CompletedTrade) string {
	gave, got := countList(c.Gave), countList(c.Got)
	if gave == "" {
		gave = "nothing"
	}
	if got == "" {
		got = "nothing"
	}
	return fmt.Sprintf("traded with %s: gave %s, got %s", c.With, gave, got)
}

// countList renders an item-count map as "2 Ice Crystal, 1 Mushroom" in catalog
// order.
func countList(m map[string]int) string {
	var parts []string
	for _, it := range Items {
		if n := m[it.ID]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, it.Name))
		}
	}
	// Any unknown ids (shouldn't happen) appended for safety, sorted.
	if len(parts) == 0 && len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%d %s", m[k], k))
		}
	}
	return strings.Join(parts, ", ")
}
