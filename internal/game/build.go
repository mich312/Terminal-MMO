package game

// Build-side polish: demolishing your own structures (returning whatever they
// hold) and the workbench-as-station tie-in. Owner-gated, so nobody can tear
// down or empty someone else's Workspace.

// IsWorkbench reports whether a placeable id is a crafting bench.
func IsWorkbench(kind string) bool { return kind == "workbench" }

// AddToPack credits n of an item to the player's pack (live + store) — exported
// for the clearing yield in the wilds area.
func AddToPack(ctx *Ctx, item string, n int) { addToPack(ctx, item, n) }

// addToPack credits n of an item to the player's pack (live + store).
func addToPack(ctx *Ctx, item string, n int) {
	if n <= 0 || item == "" {
		return
	}
	if ctx.Inventory == nil {
		ctx.Inventory = map[string]int{}
	}
	for i := 0; i < n; i++ {
		ctx.Inventory[item]++
		ctx.Store.AddItem(ctx.Name, item)
	}
}

// Demolish removes the player's own structure at (x,y), returning any goods it
// holds to their pack — a stall's stock and till, a machine's input/output
// buffers — so nothing is ever lost to a teardown. Returns false if the cell is
// empty or the structure isn't theirs.
func Demolish(ctx *Ctx, x, y int) bool {
	pl, ok := ctx.World.PlacementAt(x, y)
	if !ok || pl.Owner != ctx.Name {
		return false
	}
	switch {
	case IsStall(pl.Kind):
		st := decodeStall(pl.State)
		for _, o := range st.Offers {
			addToPack(ctx, o.GiveItem, o.Stock)
		}
		for item, n := range st.Till {
			addToPack(ctx, item, n)
		}
	default:
		if k, ok := MachineKindFor(pl.Kind); ok {
			ms := decodeMachine(pl.State)
			addToPack(ctx, k.In, ms.In)
			addToPack(ctx, k.Out, ms.Out)
		}
	}
	_, removed := ctx.World.Unplace("wilds", x, y)
	return removed
}
