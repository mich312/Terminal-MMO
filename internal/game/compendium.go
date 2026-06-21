package game

import (
	"fmt"
	"strings"
)

// The compendium is the in-game codex of every collectible and wearable: what
// each find is, where it turns up, and what it does. Both clients (the glyph
// info panel and the HD pixel panel) render from the builders here, so the two
// never drift and the catalog stays the one source of truth.

// CompendiumEntry is one item card: a catalog item, how many the player holds
// (0 = not yet found), and the one-line note on what it does.
type CompendiumEntry struct {
	Item  Item
	Owned int
	Note  string // what it can do — wearable power and/or special use; "" for a plain material
}

// CompendiumGroup is a titled run of entries sharing a Source.
type CompendiumGroup struct {
	Title   string
	Entries []CompendiumEntry
}

var compendiumGroups = []struct {
	src   Source
	title string
}{
	{Forage, "Forage"},
	{Worksite, "Worksite harvests"},
	{CaveFind, "Cave finds"},
}

// Compendium builds the full item catalog grouped by source, annotated with the
// player's counts. inv may be nil (every item then reads as not-yet-found).
func Compendium(inv map[string]int) []CompendiumGroup {
	var groups []CompendiumGroup
	for _, g := range compendiumGroups {
		grp := CompendiumGroup{Title: g.title}
		for _, it := range Items {
			if it.Source != g.src {
				continue
			}
			grp.Entries = append(grp.Entries, CompendiumEntry{
				Item:  it,
				Owned: inv[it.ID],
				Note:  ItemNote(it),
			})
		}
		if len(grp.Entries) > 0 {
			groups = append(groups, grp)
		}
	}
	return groups
}

// ItemNote is the one-line "what it can do" for an item: the wearable power it
// unlocks (derived from the accessory catalog) and any special use, joined with
// a dot. Empty for a find that's just a raw material.
func ItemNote(it Item) string {
	var parts []string
	if acc, power, ok := it.WearPower(); ok {
		if power != "" {
			parts = append(parts, fmt.Sprintf("worn as the %s — %s", acc, power))
		} else {
			parts = append(parts, fmt.Sprintf("worn as the %s", acc))
		}
	}
	if it.Use != "" {
		parts = append(parts, it.Use)
	}
	return strings.Join(parts, " · ")
}

// WearableEntry is one accessory row in the compendium's wearables section.
type WearableEntry struct {
	Index  int
	Name   string
	Power  string // "" for a plain cosmetic
	Source string // how you come by it
	Owned  bool
	Worn   bool
}

// itemForWearable maps an accessory name to the item that unlocks it, so the
// compendium can say "forage a Mushroom" instead of a vague hint.
func itemForWearable(name string) (Item, bool) {
	for _, it := range Items {
		if it.Wear == name {
			return it, true
		}
	}
	return Item{}, false
}

// Wearables lists every accessory beyond the bare "none": its power, how it's
// unlocked, and whether this player owns or wears it. ctx may be nil.
func Wearables(ctx *Ctx) []WearableEntry {
	owned := map[int]bool{}
	worn := -1
	if ctx != nil {
		for _, idx := range OwnedHats(ctx) {
			owned[idx] = true
		}
		if cur, ok := ctx.World.Self(ctx.Name); ok {
			worn = cur.Accessory
		}
	}
	out := make([]WearableEntry, 0, NumAccessories())
	for i := 1; i < NumAccessories(); i++ {
		name := AccessoryName(i)
		src := "Found out in the Wilds"
		if it, ok := itemForWearable(name); ok {
			src = "Forage a " + it.Name
		}
		out = append(out, WearableEntry{
			Index:  i,
			Name:   name,
			Power:  AccessoryPower(i),
			Source: src,
			Owned:  owned[i],
			Worn:   i == worn,
		})
	}
	return out
}
