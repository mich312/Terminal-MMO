# Trade Plan — the Durst Group Concession

> **Status:** ✅ Phases 1–3 shipped — the atomic `world.MutatePlacement`, the
> `StallState` schema + trade logic, the Concession placeable, and the buyer
> panel in both clients, with `/sell` and `/collect` as the authoring path. A
> stocked stall trades to others while the owner is away. ⬜ Phase 4 (a full
> keyboard owner-authoring panel) is the remaining polish.

> How player-to-player trade lands on the cozy-frontier foundation
> ([`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md),
> [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md)). Grounded in the real
> packages. Voice is corporate × medieval.

## The shape: an asynchronous vending stall

A **Concession** is a *placement* (like a machine) that any player can stock and
anyone can buy from — **without both being online at once.** The owner posts
offers and stocks goods; buyers walk up and accept whenever; the owner collects
the takings later. This is the only model that fits the pillars: presence is
eventually consistent, so live "both players trading face-to-face" is the wrong
shape — a **stall that trades on your behalf while you're away** is the right one,
and it reuses everything machines already proved.

A stall holds, in its placement `State`:

```go
type Offer struct {
    GiveItem string; GiveN int   // what a buyer receives
    AskItem  string; AskN  int   // what a buyer pays
    Stock    int                 // give-units the owner has deposited
    Sold     int                 // lifetime counter (flavor)
}
type StallState struct {
    Offers []Offer
    Till   map[string]int        // payments waiting for the owner to collect
}
```

Everything is an item id from `inventory.go`; no new currency. An offer trades
real goods for real goods ("10 Planks for 6 Cut Stone").

## The one new architectural piece: an atomic stall transaction

Machines mutate their own state from a single owner, so a client-side
read-settle-write was safe. **Trade is not:** two buyers can hit the same offer
at once, and `UpdatePlacementState` is last-writer-wins — it would oversell stock
or drop a payment. So we need the mutation to be **atomic under the world mutex**.

Add one general primitive to `internal/world` (the world stays schema-agnostic —
it never decodes `StallState`):

```go
// MutatePlacement runs fn against the placement at (x,y) under the world mutex,
// for an atomic read-modify-write of its opaque State. fn gets the current State
// and returns the new State plus whether it changed anything. Persists +
// broadcasts when it did. The safe way to settle a race like two buyers on one
// stall. false if nothing is placed there or fn made no change.
func (w *World) MutatePlacement(area string, x, y int,
    fn func(state string) (newState string, changed bool)) bool
```

Then the trade logic lives in `internal/game` (which *can* decode the schema),
and the whole check-decrement-credit happens inside the locked callback:

```go
func AcceptOffer(ctx *Ctx, x, y, idx int) (Offer, bool) {
    var got Offer; var done bool
    // pre-check the buyer can pay (buyer inventory is single-session, so no race)
    snap, ok := StallSnapshot(ctx, x, y); ...
    if ctx.Inventory[o.AskItem] < o.AskN { return Offer{}, false }

    ctx.World.MutatePlacement("wilds", x, y, func(s string) (string, bool) {
        st := decodeStall(s)
        o := st.Offers[idx]
        if o.Stock < o.GiveN { return s, false }   // sold out — lost the race
        o.Stock -= o.GiveN
        st.Till[o.AskItem] += o.AskN
        st.Offers[idx] = o
        got, done = o, true
        return st.encode(), true
    })
    if !done { return Offer{}, false }
    // buyer pays + receives locally (can't fail; pre-checked)
    spend got.AskN × got.AskItem; add got.GiveN × got.GiveItem
    return got, true
}
```

Reserve-at-stall-then-pay-locally means no refund path: the atomic step either
secures the goods or reports sold-out, and the local payment never fails.
`MutatePlacement` is also a tidy retrofit for machines (collect/refuel become
atomic too), so it pays for itself twice.

## Reused wholesale (almost nothing is new)

| Need | Already shipped |
| --- | --- |
| A stall in the world | the placements layer (`world.Place`, `EventPlaced`) |
| Per-stall mutable state + persistence | placement `State` JSON + the store column |
| Build the stall | a new `Placeable` (Concession, `PropStall`) with a cost |
| Open it by standing beside it | `Ctx.UseMachine`-style signal (generalize to `UseStation`) |
| Buyer/owner panels | the `DrawCraftPanel` / `DrawMachinePanel` idiom, both clients |
| Spend/receive goods | `Ctx.Inventory` + `store.AddItem/SpendItem` |

The only genuinely new code is `MutatePlacement`, the `StallState` schema, the
two panels, and the owner-authoring flow.

## UI — two roles, one placement

When you press `e` beside a Concession, the panel you get depends on whether
you're the owner (`placement.Owner == ctx.Name`):

**Buyer panel** (the easy half — it's the craft panel with money).
- A list of offers: `GiveN Give  ⇄  AskN Ask`, with stock remaining and a
  green/amber affordability mark.
- `e` accepts the selected offer (→ `AcceptOffer`); a toast confirms, the row's
  stock ticks down, sold-out rows grey out.

**Owner panel** (when it's your stall).
- The same offer list, plus **Till** (collected payments) and each offer's
  **Stock**.
- `c` collect the till into your pack; `f` deposit a batch of the selected
  offer's give-item from your pack into its stock (mirrors machine refuel).
- Authoring offers: `n` adds an offer and drops into an edit row with
  ←→-adjustable fields (give item, give qty, ask item, ask qty) exactly like the
  character panel's field cycling; `x` removes the selected offer.

## Phasing

1. **`MutatePlacement`** + the `StallState` schema + `AcceptOffer`/`StallSnapshot`
   /`CollectTill`/`DepositStock`/`AddOffer`/`RemoveOffer` (pure-ish game logic on
   top of the world primitive). **This is the testable core** — land and test it
   before any UI.
2. **Concession placeable** + `UseStation` open signal in the Wilds (generalize
   the machine path so `e` opens either a machine or a stall).
3. **Buyer panel** in both clients — the smaller, higher-value half (a stall is
   useless to others until someone can buy).
4. **Owner panel** — collect/deposit first (simple), then offer authoring.
5. **Trade** chat command alias `/sell <n> <item> for <m> <item>` as a fast
   authoring path next to the panel (optional; the command framework makes it
   cheap and it sidesteps keyboard-form fiddliness).

Ship 1–3 as the first usable slice: a stocked stall others can buy from. 4–5
complete the owner experience.

## Tests (the core carries the risk)

- **Atomicity:** fire N concurrent `AcceptOffer`s at a stall stocked for K sales;
  assert exactly K succeed, stock floors at 0, and the till credits exactly K
  payments — never oversold (this is *the* test that justifies `MutatePlacement`).
- `AcceptOffer`: spends the ask, grants the give, credits the till; sold-out and
  can't-afford are no-ops.
- Owner ops: deposit moves pack→stock, collect moves till→pack, add/remove offer.
- Persistence: a stall's offers/stock/till round-trip through the store
  (`State` already persists; just assert the schema survives).
- Determinism untouched: trade only ever mutates the stored placements layer.

## Pillar check

| Pillar | Trade? |
| --- | --- |
| Works from any terminal | ✅ panels render in both clients |
| SSH username is identity | ✅ stall `Owner`; buyer/owner views key off it |
| Shared, real-time, eventually consistent | ✅ `MutatePlacement` makes the one racy op atomic; `EventPlaced` nudges redraws |
| Persist between visits, not during | ✅ **async trade is built on exactly this** — sell while logged off |
| Deterministic & offline | ✅ terrain untouched; trade only mutates the stored placement layer, no RNG |

## Open questions (decide before building)

- **Theft / grief:** can anyone collect a stall's till, or only the owner? (Owner
  only — keyed on `ctx.Name == placement.Owner`.) Can a non-owner demolish a
  stall? (No — restrict `Unplace` to the owner; a small, sensible follow-up that
  also protects machines/fences.)
- **Discovery:** how do buyers find stalls? Out of scope here; a later `/stalls`
  list or minimap markers. For now they're found by walking the Wilds.
- **Stall cap per player:** probably 1–3, to keep the world legible. Trivial to
  enforce at build time once decided.
