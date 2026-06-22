package game

import (
	"encoding/json"
	"time"

	"github.com/durst-group/durstworld/internal/world"
)

// Machines: the offline-production layer (Milestone 1, step 3). A machine is a
// placement whose opaque State carries input/output buffers and a wall-clock;
// Settle fast-forwards elapsed real time into product. It is pure and has no
// per-tick RNG, so a machine costs nothing while idle and "runs while you're
// logged off" — the headline of persist-between-visits. Voice: corporate ×
// medieval.

// nowUnix is the machine clock, swappable in tests.
var nowUnix = func() int64 { return time.Now().Unix() }

// MachineKind is a machine type: what it converts, how fast, and how much it can
// hold. Keyed by the placeable id it's built from.
type MachineKind struct {
	Placeable string
	Name      string
	In, Out   string        // item ids consumed / produced
	InPer     int           // input consumed per cycle
	OutPer    int           // output produced per cycle
	Period    time.Duration // wall-clock per cycle
	Cap       int           // output buffer ceiling
}

var machineKinds = map[string]MachineKind{
	"sawmill": {"sawmill", "Sawmill", "wood", "plank", 1, 1, 20 * time.Second, 16},
	"mill":    {"mill", "Mill", "grain", "flour", 1, 1, 20 * time.Second, 16},
	"furnace": {"furnace", "Ingot Synergy Furnace", "nugget", "ingot", 2, 1, 45 * time.Second, 12},
}

// MachineKindFor returns the machine type a placeable id builds, if it's a machine.
func MachineKindFor(placeable string) (MachineKind, bool) {
	k, ok := machineKinds[placeable]
	return k, ok
}

// IsMachine reports whether a placeable id is a machine.
func IsMachine(placeable string) bool {
	_, ok := machineKinds[placeable]
	return ok
}

// MachineState is the per-machine mutable buffer, JSON-encoded into a
// placement's State.
type MachineState struct {
	In   int   `json:"in"`
	Out  int   `json:"out"`
	Last int64 `json:"last"` // unix seconds of the last settle
}

func decodeMachine(s string) MachineState {
	var m MachineState
	if s != "" {
		_ = json.Unmarshal([]byte(s), &m)
	}
	return m
}

func (m MachineState) encode() string {
	b, _ := json.Marshal(m)
	return string(b)
}

// Settle advances a machine from its last tick to now, running as many whole
// cycles as elapsed time, available input and output headroom allow — consuming
// input and producing output. Pure & deterministic (no RNG): the same state and
// now always yield the same result, so a machine fast-forwards correctly on
// load no matter how long it slept. Returns the new state, output produced and
// input consumed.
func Settle(k MachineKind, s MachineState, now int64) (MachineState, int, int) {
	if s.Last == 0 || s.Last > now {
		s.Last = now // first touch (or a clock that went backwards): start the clock
		return s, 0, 0
	}
	period := int64(k.Period / time.Second)
	if period <= 0 {
		period = 1
	}
	timeCycles := int((now - s.Last) / period)
	if timeCycles <= 0 {
		return s, 0, 0 // less than a period elapsed; the remainder carries
	}
	cycles := timeCycles
	if c := s.In / k.InPer; c < cycles { // starved
		cycles = c
	}
	if c := (k.Cap - s.Out) / k.OutPer; c < cycles { // output full
		cycles = c
	}
	if cycles < 0 {
		cycles = 0
	}
	out, in := cycles*k.OutPer, cycles*k.InPer
	s.In -= in
	s.Out += out
	if cycles == timeCycles {
		s.Last += int64(cycles) * period // time-limited: carry the sub-period remainder
	} else {
		s.Last = now // stalled (starved or full): the idle time is simply gone
	}
	return s, out, in
}

// MachineView is a settled snapshot for the panels: the kind, current buffers,
// whether it's actively producing and seconds until the next unit.
type MachineView struct {
	Kind    MachineKind
	State   MachineState
	Running bool
	NextSec int
}

func viewOf(k MachineKind, s MachineState, now int64) MachineView {
	period := int64(k.Period / time.Second)
	if period <= 0 {
		period = 1
	}
	running := s.In >= k.InPer && s.Out+k.OutPer <= k.Cap
	next := 0
	if running && s.Last > 0 && s.Last <= now {
		next = int(period - (now-s.Last)%period)
	} else if running {
		next = int(period)
	}
	return MachineView{Kind: k, State: s, Running: running, NextSec: next}
}

func machineAt(ctx *Ctx, x, y int) (world.Placement, MachineKind, bool) {
	pl, ok := ctx.World.PlacementAt(x, y)
	if !ok {
		return pl, MachineKind{}, false
	}
	k, ok := MachineKindFor(pl.Kind)
	return pl, k, ok
}

// OpenMachine settles a machine to now and persists it, returning a view plus
// the output gained / input consumed since it last ticked — the "while you were
// away" delta. ok is false if (x,y) is not a machine.
func OpenMachine(ctx *Ctx, x, y int) (view MachineView, gainedOut, consumedIn int, ok bool) {
	pl, k, isM := machineAt(ctx, x, y)
	if !isM {
		return MachineView{}, 0, 0, false
	}
	now := nowUnix()
	st := decodeMachine(pl.State)
	st, gainedOut, consumedIn = Settle(k, st, now)
	ctx.World.UpdatePlacementState("wilds", x, y, st.encode())
	return viewOf(k, st, now), gainedOut, consumedIn, true
}

// MachineSnapshot returns a settled view for display, without persisting (so a
// redraw never writes). ok is false if (x,y) is not a machine.
func MachineSnapshot(ctx *Ctx, x, y int) (MachineView, bool) {
	pl, k, isM := machineAt(ctx, x, y)
	if !isM {
		return MachineView{}, false
	}
	now := nowUnix()
	st, _, _ := Settle(k, decodeMachine(pl.State), now)
	return viewOf(k, st, now), true
}

// CollectMachine settles then empties the output buffer into the player's pack,
// persisting the machine. Returns how many units were collected.
func CollectMachine(ctx *Ctx, x, y int) int {
	pl, k, isM := machineAt(ctx, x, y)
	if !isM {
		return 0
	}
	st, _, _ := Settle(k, decodeMachine(pl.State), nowUnix())
	got := st.Out
	if got > 0 {
		if ctx.Inventory == nil {
			ctx.Inventory = map[string]int{}
		}
		for i := 0; i < got; i++ {
			ctx.Inventory[k.Out]++
			ctx.Store.AddItem(ctx.Name, k.Out)
		}
		st.Out = 0
	}
	ctx.World.UpdatePlacementState("wilds", x, y, st.encode())
	return got
}

// machineRefuelBatch caps how much input one refuel press loads.
const machineRefuelBatch = 20

// RefuelMachine settles then moves up to a batch of the machine's input item
// from the pack into its hopper, persisting. Returns how many units were loaded.
func RefuelMachine(ctx *Ctx, x, y int) int {
	pl, k, isM := machineAt(ctx, x, y)
	if !isM {
		return 0
	}
	st, _, _ := Settle(k, decodeMachine(pl.State), nowUnix())
	have := 0
	if ctx.Inventory != nil {
		have = ctx.Inventory[k.In]
	}
	move := have
	if move > machineRefuelBatch {
		move = machineRefuelBatch
	}
	for i := 0; i < move; i++ {
		ctx.Inventory[k.In]--
		ctx.Store.SpendItem(ctx.Name, k.In)
	}
	if move > 0 && ctx.Inventory[k.In] <= 0 {
		delete(ctx.Inventory, k.In)
	}
	st.In += move
	if st.Last == 0 {
		st.Last = nowUnix()
	}
	ctx.World.UpdatePlacementState("wilds", x, y, st.encode())
	return move
}
