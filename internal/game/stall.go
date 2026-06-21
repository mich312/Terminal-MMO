package game

import (
	"encoding/json"

	"github.com/durst-group/durstworld/internal/world"
)

// Trade: the Durst Group Concession — an asynchronous vending stall built on the
// placements layer (docs/TRADE_PLAN.md). An owner stocks offers; anyone buys
// whenever, online or not; the owner collects the takings later. The one racy
// operation (two buyers on one offer) is settled atomically through
// world.MutatePlacement. Voice is corporate × medieval.

// stallKind is the placeable id a Concession is built from.
const stallKind = "stall"

// IsStall reports whether a placeable id is a trade stall.
func IsStall(kind string) bool { return kind == stallKind }

// Offer is one posted swap: GiveN of GiveItem for AskN of AskItem, backed by
// Stock give-units the owner has deposited.
type Offer struct {
	GiveItem string `json:"g"`
	GiveN    int    `json:"gn"`
	AskItem  string `json:"a"`
	AskN     int    `json:"an"`
	Stock    int    `json:"s"` // give-units available (counts in GiveItem units)
	Sold     int    `json:"sold"`
}

// StallState is a Concession's persisted contents.
type StallState struct {
	Offers []Offer        `json:"offers"`
	Till   map[string]int `json:"till"` // payments awaiting the owner
}

func decodeStall(s string) StallState {
	var st StallState
	if s != "" {
		_ = json.Unmarshal([]byte(s), &st)
	}
	if st.Till == nil {
		st.Till = map[string]int{}
	}
	return st
}

func (st StallState) encode() string {
	b, _ := json.Marshal(st)
	return string(b)
}

// StallSnapshot returns a stall's current contents for display. ok is false if
// (x,y) is not a stall.
func StallSnapshot(ctx *Ctx, x, y int) (StallState, bool) {
	pl, ok := ctx.World.PlacementAt(x, y)
	if !ok || !IsStall(pl.Kind) {
		return StallState{}, false
	}
	return decodeStall(pl.State), true
}

// StallOwner returns whether ctx's player owns the stall at (x,y).
func StallOwner(ctx *Ctx, x, y int) bool {
	pl, ok := ctx.World.PlacementAt(x, y)
	return ok && pl.Owner == ctx.Name
}

// CanAcceptOffer reports whether the pack can pay an offer's ask and it's still
// in stock.
func CanAcceptOffer(o Offer, inv map[string]int) bool {
	return o.Stock >= o.GiveN && inv[o.AskItem] >= o.AskN
}

// AcceptOffer buys offer idx at the stall: it atomically reserves the goods and
// credits the till (under the world mutex, so concurrent buyers never oversell),
// then pays the ask and receives the give from the buyer's own pack. Returns the
// completed offer and true, or false if sold out / unaffordable / not a stall.
func AcceptOffer(ctx *Ctx, x, y, idx int) (Offer, bool) {
	if ctx.Inventory == nil {
		ctx.Inventory = map[string]int{}
	}
	// Pre-check affordability against a snapshot (buyer inventory is single-
	// session, so it can't race); the stock check is redone atomically below.
	snap, ok := StallSnapshot(ctx, x, y)
	if !ok || idx < 0 || idx >= len(snap.Offers) {
		return Offer{}, false
	}
	if ctx.Inventory[snap.Offers[idx].AskItem] < snap.Offers[idx].AskN {
		return Offer{}, false
	}

	var got Offer
	var done bool
	ctx.World.MutatePlacement("wilds", x, y, func(s string) (string, bool) {
		st := decodeStall(s)
		if idx >= len(st.Offers) {
			return s, false
		}
		o := st.Offers[idx]
		if o.Stock < o.GiveN { // sold out — lost the race
			return s, false
		}
		o.Stock -= o.GiveN
		o.Sold++
		st.Till[o.AskItem] += o.AskN
		st.Offers[idx] = o
		got, done = o, true
		return st.encode(), true
	})
	if !done {
		return Offer{}, false
	}
	// Pay and receive locally (can't fail; pre-checked, and the goods are reserved).
	for i := 0; i < got.AskN; i++ {
		ctx.Inventory[got.AskItem]--
		ctx.Store.SpendItem(ctx.Name, got.AskItem)
	}
	if ctx.Inventory[got.AskItem] <= 0 {
		delete(ctx.Inventory, got.AskItem)
	}
	for i := 0; i < got.GiveN; i++ {
		ctx.Inventory[got.GiveItem]++
		ctx.Store.AddItem(ctx.Name, got.GiveItem)
	}
	return got, true
}

// AddOffer (owner) posts a new offer and stocks it by moving every unit of the
// give-item the owner is carrying into the offer. Returns the units stocked, or
// 0 if the terms are invalid or the pack holds none of the give-item.
func AddOffer(ctx *Ctx, x, y int, give string, giveN int, ask string, askN int) int {
	if !StallOwner(ctx, x, y) || giveN <= 0 || askN <= 0 || give == ask {
		return 0
	}
	if _, ok := ItemByID(give); !ok {
		return 0
	}
	if _, ok := ItemByID(ask); !ok {
		return 0
	}
	stock := 0
	if ctx.Inventory != nil {
		stock = ctx.Inventory[give]
	}
	if stock < giveN { // need at least one sale's worth to post
		return 0
	}
	ctx.World.MutatePlacement("wilds", x, y, func(s string) (string, bool) {
		st := decodeStall(s)
		st.Offers = append(st.Offers, Offer{GiveItem: give, GiveN: giveN,
			AskItem: ask, AskN: askN, Stock: stock})
		return st.encode(), true
	})
	for i := 0; i < stock; i++ {
		ctx.Inventory[give]--
		ctx.Store.SpendItem(ctx.Name, give)
	}
	if ctx.Inventory[give] <= 0 {
		delete(ctx.Inventory, give)
	}
	return stock
}

// CollectTill (owner) sweeps the stall's takings into the owner's pack. Returns
// the total units collected.
func CollectTill(ctx *Ctx, x, y int) int {
	if !StallOwner(ctx, x, y) {
		return 0
	}
	if ctx.Inventory == nil {
		ctx.Inventory = map[string]int{}
	}
	taken := map[string]int{}
	ctx.World.MutatePlacement("wilds", x, y, func(s string) (string, bool) {
		st := decodeStall(s)
		if len(st.Till) == 0 {
			return s, false
		}
		for item, n := range st.Till {
			taken[item] = n
		}
		st.Till = map[string]int{}
		return st.encode(), true
	})
	total := 0
	for item, n := range taken {
		for i := 0; i < n; i++ {
			ctx.Inventory[item]++
			ctx.Store.AddItem(ctx.Name, item)
		}
		total += n
	}
	return total
}

// StationNear finds an interactable placement (a machine or a stall) on the ring
// around the player's body, using live world position. pred filters which kinds
// qualify. Shared by the chat commands (which have no area handle).
func StationNear(ctx *Ctx, pred func(world.Placement) bool) (int, int, bool) {
	self, ok := ctx.World.Self(ctx.Name)
	if !ok {
		return 0, 0, false
	}
	for y := self.Y - 1; y <= self.Y+PlayerH; y++ {
		for x := self.X - 1; x <= self.X+PlayerW; x++ {
			if x >= self.X && x < self.X+PlayerW && y >= self.Y && y < self.Y+PlayerH {
				continue
			}
			if pl, ok := ctx.World.PlacementAt(x, y); ok && pred(pl) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}
