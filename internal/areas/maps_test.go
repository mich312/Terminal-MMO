// Package areas_test sanity-checks the hand-crafted maps: rectangular rows,
// reachable portals, walkable spawn points.
package areas_test

import (
	"path/filepath"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"

	_ "github.com/durst-group/durstworld/internal/areas/arcade"
	_ "github.com/durst-group/durstworld/internal/areas/democenter"
	_ "github.com/durst-group/durstworld/internal/areas/kraftwerk"
	_ "github.com/durst-group/durstworld/internal/areas/lobby"
	_ "github.com/durst-group/durstworld/internal/areas/maze"
	_ "github.com/durst-group/durstworld/internal/areas/presentation"
	_ "github.com/durst-group/durstworld/internal/areas/sokoban"
)

// TestAreasConstructAndSpawn instantiates every registered area and runs
// its Init, which places the player on a spawn tile.
func TestAreasConstructAndSpawn(t *testing.T) {
	for _, id := range []string{"lobby", "presentation", "kraftwerk", "democenter", "arcade", "sokoban", "maze"} {
		t.Run(id, func(t *testing.T) {
			w := world.New()
			defer w.Close()
			name, _ := w.Join("tester")
			st := store.Open(filepath.Join(t.TempDir(), "test.db"))
			defer st.Close()
			ctx := &game.Ctx{World: w, Store: st, Name: name}

			a := game.NewArea(id, ctx)
			if a == nil {
				t.Fatal("area not registered")
			}
			self, _ := w.Self(name)
			a.Init(&self)

			self, ok := w.Self(name)
			if !ok || self.Area != id {
				t.Fatalf("after Init player should be in %q, got %q", id, self.Area)
			}
			if v := a.View(80, 17); v == "" {
				t.Error("empty view")
			}
		})
	}
}
