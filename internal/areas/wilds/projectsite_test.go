package wilds

import (
	"strings"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// Standing beside a community build shows its live progress, and a contribution
// to the shared world moves the tally the hint reads — the catalog, the world's
// project state, and the wilds hint all wired together.
func TestProjectSiteHint(t *testing.T) {
	w := world.New()
	t.Cleanup(w.Close)
	name, _ := w.Join("ada")
	ctx := &game.Ctx{World: w, Store: store.Open(t.TempDir() + "/p.db"), Name: name, Theme: ui.Default,
		Inventory: map[string]int{}, Hats: map[int]bool{}, FixedGates: map[string]bool{}}
	a := game.NewArea("wilds", ctx).(*area)
	self, _ := w.Self(name)
	a.Init(&self)

	s := worldgen.ProjectSites[0]
	a.wx, a.wy = s.X+1, s.Y // stand the body just east of the anchor
	if _, ok := a.projectSiteAdjacent(); !ok {
		t.Fatal("expected to stand beside the project site")
	}

	hint := a.Hint()
	if !strings.Contains(hint, s.Name) || !strings.Contains(hint, "0/30") {
		t.Fatalf("hint = %q, want the build name and an empty Foundation tally", hint)
	}

	// Contribute to the shared world; the hint reflects the new tally.
	def, _ := game.ProjectByID(s.ID)
	w.ContributeToProject(s.ID, name, 0, "stone", 5, def.PhaseReq(0), len(def.Phases))
	if hint := a.Hint(); !strings.Contains(hint, "5/30") {
		t.Fatalf("after contributing 5 stone, hint = %q, want 5/30", hint)
	}
}
