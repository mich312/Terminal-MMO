package game

import (
	"strings"
	"testing"
)

// TestCatalogEntriesComplete guards that every collectible carries the prose the
// compendium renders — a name, flavor, and where it's found — and that any find
// flagged as a wearable resolves to a real accessory. A blank card would render
// as an empty row.
func TestCatalogEntriesComplete(t *testing.T) {
	for _, it := range Items {
		if it.Name == "" || it.About == "" || it.Found == "" {
			t.Errorf("%s: missing Name/About/Found (%q/%q/%q)", it.ID, it.Name, it.About, it.Found)
		}
		if it.Wear != "" {
			if _, ok := AccessoryIndex(it.Wear); !ok {
				t.Errorf("%s unlocks unknown accessory %q", it.ID, it.Wear)
			}
		}
	}
}

// TestCompendiumCoversCatalog: every catalog item appears in exactly one group,
// so nothing silently drops out of the codex when a new Source is added.
func TestCompendiumCoversCatalog(t *testing.T) {
	seen := map[string]int{}
	for _, g := range Compendium(nil) {
		if g.Title == "" {
			t.Error("group with empty title")
		}
		for _, e := range g.Entries {
			seen[e.Item.ID]++
		}
	}
	for _, it := range Items {
		if seen[it.ID] != 1 {
			t.Errorf("%s appears in %d compendium groups, want 1", it.ID, seen[it.ID])
		}
	}
}

// TestItemNoteDerivesWearPower: a wearable find's note names the accessory and
// its power (pulled from the accessory catalog, not duplicated), so the "what it
// does" line can't drift from what the wearable actually grants.
func TestItemNoteDerivesWearPower(t *testing.T) {
	it, ok := ItemByID("spore")
	if !ok {
		t.Fatal("missing spore")
	}
	note := ItemNote(it)
	acc, power, _ := it.WearPower()
	if !strings.Contains(note, acc) || !strings.Contains(note, power) {
		t.Errorf("spore note %q should mention %q and %q", note, acc, power)
	}
	// A plain material (no Wear, no Use) has nothing to say.
	if grain, _ := ItemByID("grain"); ItemNote(grain) != "" {
		t.Errorf("grain is a plain material; note should be empty, got %q", ItemNote(grain))
	}
}

// TestWearablesUnlockHints: a wearable unlocked by foraging an item points back
// at that item by name, rather than the vague "out in the Wilds" fallback.
func TestWearablesUnlockHints(t *testing.T) {
	got := map[string]string{}
	for _, w := range Wearables(nil) {
		got[w.Name] = w.Source
	}
	if src := got["shroom"]; !strings.Contains(src, "Mushroom") {
		t.Errorf("shroom source = %q, want it to name the Mushroom", src)
	}
}
