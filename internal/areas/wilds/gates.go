package wilds

import (
	"strings"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// Sealed gates are optional, riddle/offering-gated doors out past the hub.
// There are two kinds:
//
//   - personal: each player repairs it themselves — say the riddle's answer in
//     chat while standing at it, or press e to offer the required item. State is
//     per-player (Ctx.FixedGates).
//   - co-op: a shared community effort — anyone presses e to contribute the
//     required item into a pool; when the pool fills, the gate opens for
//     everyone. State is global (the shared World).
type gateKind int

const (
	gatePersonal gateKind = iota
	gateCoop
)

type gate struct {
	worldgen.Landmark        // position, destination (Portal), name, glyph, color
	kind              gateKind
	riddle            string // shown at the gate; answer it in chat (personal)
	answer            string
	item              string // item id that repairs/contributes
	need              int    // co-op: contributions required to open
}

// gates is keyed by destination id (worldgen.Gates' Portal field).
var gates = buildGates()

func buildGates() map[string]gate {
	meta := map[string]struct {
		kind           gateKind
		riddle, answer string
		item           string
		need           int
	}{
		"grove": {gatePersonal,
			"I have rivers but no water, forests but no trees, and roads but no cars. What am I?",
			"map", "crystal", 1},
		"vault": {gateCoop, "", "", "nugget", 3},
	}
	out := map[string]gate{}
	for _, lm := range worldgen.Gates {
		m := meta[lm.Portal]
		out[lm.Portal] = gate{Landmark: lm, kind: m.kind, riddle: m.riddle,
			answer: m.answer, item: m.item, need: m.need}
	}
	return out
}

// gateAtCell returns the gate whose marker sits exactly on (x,y).
func gateAtCell(x, y int) (gate, bool) {
	for _, g := range gates {
		if g.X == x && g.Y == y {
			return g, true
		}
	}
	return gate{}, false
}

func normalizeAnswer(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func itemName(id string) string {
	if it, ok := game.ItemByID(id); ok {
		return it.Name
	}
	return id
}
