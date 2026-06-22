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
	_ "github.com/durst-group/durstworld/internal/areas/snake"
	_ "github.com/durst-group/durstworld/internal/areas/sokoban"
	_ "github.com/durst-group/durstworld/internal/areas/wilds"
)

// Every walkable area must be HD-renderable: implement game.HDViewer, hand back
// a correctly-sized tile window, and feed a non-empty RGBA frame. This is what
// lets HD mode work in all worlds, not just the Wilds.
func TestAreasHDRenderable(t *testing.T) {
	const vw, vh, scale = 40, 24, 8
	for _, id := range []string{"wilds", "lobby", "kraftwerk", "democenter", "presentation", "arcade", "sokoban", "maze", "snake"} {
		t.Run(id, func(t *testing.T) {
			w := world.New()
			defer w.Close()
			name, _ := w.Join("you")
			st := store.Open(filepath.Join(t.TempDir(), "x.db"))
			defer st.Close()
			ctx := &game.Ctx{World: w, Store: st, Name: name}

			a := game.NewArea(id, ctx)
			hv, ok := a.(game.HDViewer)
			if !ok {
				t.Fatalf("%s does not implement game.HDViewer", id)
			}
			self, _ := w.Self(name)
			a.Init(&self)

			tm, ox, oy := hv.HDView(vw, vh)
			if tm.W != vw || tm.H != vh {
				t.Fatalf("HDView window %dx%d, want %dx%d", tm.W, tm.H, vw, vh)
			}
			img := game.RenderRGBA(nil, tm, w.PlayersInArea(id), name, 3,
				game.Camera{W: vw, H: vh}, game.Light{}, ox, oy, scale, false, game.DefaultStyle())
			if img.Bounds().Dx() != vw*scale || img.Bounds().Dy() != vh*scale {
				t.Fatalf("HD frame %dx%d, want %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), vw*scale, vh*scale)
			}
		})
	}
}

