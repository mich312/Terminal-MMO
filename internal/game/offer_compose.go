package game

// In-panel offer authoring for the Durst Group Concession: a small composer the
// stall owner edits to post an offer without leaving the panel — the UI twin of
// the `/sell` command (commands.go). Both renderers drive it through the shared
// helpers here, the way the character panel shares CycleAvatarField, so HD (which
// has no command line and so couldn't post at all before) and the glyph client
// behave identically. The composer only stages terms; PostDraft hands them to the
// existing AddOffer, which stocks the offer from the owner's pack.

// OfferDraft is an in-progress offer being authored at a Concession. It only
// holds the editable terms; the stock comes from whatever the owner carries when
// they post (AddOffer moves every give-unit in the pack onto the offer).
type OfferDraft struct {
	Field    int    // which term is selected (the OfferField* constants)
	GiveItem string // catalog id of what the owner puts up
	GiveN    int    // give-units per sale
	AskItem  string // catalog id wanted in return
	AskN     int    // ask-units per sale
}

// The composer's editable fields, in display order.
const (
	OfferFieldGive  = iota // which item to sell
	OfferFieldGiveN        // how many per sale
	OfferFieldAsk          // which item to ask for
	OfferFieldAskN         // how many to ask per sale
	OfferFields            // count, for wrap-around
)

// packItemIDs lists the catalog ids the owner is carrying, in catalog order, so
// the give-item picker only offers items there's stock to post.
func packItemIDs(ctx *Ctx) []string {
	var ids []string
	for _, r := range packRows(invOf(ctx)) {
		ids = append(ids, r.Item.ID)
	}
	return ids
}

// NewOfferDraft seeds a composer from the owner's pack. ok is false when the pack
// is empty (nothing to sell), so the caller can decline to open the composer.
func NewOfferDraft(ctx *Ctx) (OfferDraft, bool) {
	give := packItemIDs(ctx)
	if len(give) == 0 {
		return OfferDraft{}, false
	}
	d := OfferDraft{GiveItem: give[0], GiveN: 1, AskN: 1}
	// Default the ask to the first catalog item that isn't the give item.
	for _, it := range Items {
		if it.ID != d.GiveItem {
			d.AskItem = it.ID
			break
		}
	}
	return d, true
}

// CycleOfferField changes the selected field of the draft by delta (-1/+1),
// keeping the terms valid: item pickers wrap and never let give == ask, and the
// counts clamp (give to what the owner holds, ask to a sane ceiling).
func CycleOfferField(ctx *Ctx, d *OfferDraft, delta int) {
	switch d.Field {
	case OfferFieldGive:
		ids := packItemIDs(ctx)
		d.GiveItem = stepID(ids, d.GiveItem, delta, "")
		if d.AskItem == d.GiveItem { // keep the two sides distinct
			d.AskItem = stepID(catalogIDs(), d.AskItem, +1, d.GiveItem)
		}
		if held := invOf(ctx)[d.GiveItem]; d.GiveN > held && held > 0 {
			d.GiveN = held
		}
	case OfferFieldGiveN:
		held := invOf(ctx)[d.GiveItem]
		if held < 1 {
			held = 1
		}
		d.GiveN = clampInt(d.GiveN+delta, 1, held)
	case OfferFieldAsk:
		d.AskItem = stepID(catalogIDs(), d.AskItem, delta, d.GiveItem)
	case OfferFieldAskN:
		d.AskN = clampInt(d.AskN+delta, 1, 99)
	}
}

// DraftStock is how many give-units the owner would stock the offer with if they
// posted now (every unit of the give-item in the pack), and how many whole sales
// that funds. It feeds the composer's "stocks N (M sales)" hint.
func DraftStock(ctx *Ctx, d OfferDraft) (units, sales int) {
	units = invOf(ctx)[d.GiveItem]
	if d.GiveN > 0 {
		sales = units / d.GiveN
	}
	return units, sales
}

// DraftValid reports whether posting the draft now would succeed: distinct items
// and at least one sale's worth of the give-item in the pack.
func DraftValid(ctx *Ctx, d OfferDraft) bool {
	return d.GiveItem != "" && d.AskItem != "" && d.GiveItem != d.AskItem &&
		d.GiveN > 0 && d.AskN > 0 && invOf(ctx)[d.GiveItem] >= d.GiveN
}

// PostDraft posts the composed offer at the stall via the existing AddOffer (which
// validates ownership and stocks from the pack). Returns the units stocked, 0 if
// the terms don't hold.
func PostDraft(ctx *Ctx, x, y int, d OfferDraft) int {
	return AddOffer(ctx, x, y, d.GiveItem, d.GiveN, d.AskItem, d.AskN)
}

// catalogIDs is every item id in display order.
func catalogIDs() []string {
	ids := make([]string, len(Items))
	for i, it := range Items {
		ids[i] = it.ID
	}
	return ids
}

// stepID returns the neighbour of cur in ids by delta (wrapping), skipping skip.
// If cur isn't found it returns the first allowed id.
func stepID(ids []string, cur string, delta int, skip string) string {
	if len(ids) == 0 {
		return cur
	}
	idx := -1
	for i, id := range ids {
		if id == cur {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
		delta = 0
	}
	n := len(ids)
	for i := 0; i < n; i++ {
		idx = ((idx+delta)%n + n) % n
		if ids[idx] != skip {
			return ids[idx]
		}
		if delta == 0 {
			delta = 1 // a found-but-skipped start still needs to move off it
		}
	}
	return cur
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
