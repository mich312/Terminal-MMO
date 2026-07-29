package web

import (
	"strings"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
)

// Every prop the game defines should have geometry. A missing entry is not
// fatal — the client draws a marker box — but it means something in the world
// renders as a placeholder, which is worth failing a build over rather than
// discovering in a screenshot.
func TestEveryPropHasAShape(t *testing.T) {
	var missing []string
	for prop := game.PropNone + 1; prop <= game.PropChessKing; prop++ {
		sh, ok := propShapes[prop]
		if !ok {
			missing = append(missing, propLabel(prop))
			continue
		}
		// PropBldBody is deliberately empty: it's a footprint tile covered by
		// the building anchored beside it, and drawing anything there would
		// double up the walls.
		if sh.Build == "" && prop != game.PropBldBody {
			missing = append(missing, propLabel(prop)+" (empty build)")
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d props have no 3D shape: %s", len(missing), strings.Join(missing, ", "))
	}
}

// Shapes must be buildable by a builder the client actually implements, and
// have real dimensions — a zero-height prop is invisible.
func TestShapesAreWellFormed(t *testing.T) {
	known := map[string]bool{
		BuildTree: true, BuildClump: true, BuildRock: true, BuildBox: true,
		BuildBuilding: true, BuildFence: true, BuildFlat: true, BuildGlow: true,
		BuildPortal: true, BuildItem: true, BuildCreature: true, BuildChess: true,
	}
	for prop, sh := range propShapes {
		if sh.Build == "" {
			continue
		}
		if !known[sh.Build] {
			t.Errorf("%s uses unknown builder %q", propLabel(prop), sh.Build)
		}
		if sh.W <= 0 || sh.D <= 0 || sh.H <= 0 {
			t.Errorf("%s has a degenerate size %gx%gx%g", propLabel(prop), sh.W, sh.D, sh.H)
		}
		if sh.Glow < 0 || sh.Glow > 1 {
			t.Errorf("%s has glow %g outside 0..1", propLabel(prop), sh.Glow)
		}
	}
}

// The hello message's two tables have to agree: every prop id must name a shape
// key that exists, or the client looks up geometry it was never given.
func TestShapeVocabularyIsConsistent(t *testing.T) {
	ids, shapes := shapeNames()
	if len(ids) == 0 || len(shapes) == 0 {
		t.Fatal("the shape vocabulary is empty")
	}
	for propID, key := range ids {
		if _, ok := shapes[key]; !ok {
			t.Errorf("prop %d maps to shape key %q, which isn't in the shapes table", propID, key)
		}
	}
}

// Every ground texture the tilemap defines needs a surface name, or the client
// falls back to a flat finish for a biome that should ripple or sparkle.
func TestEveryTextureIsNamed(t *testing.T) {
	for tex := game.TexFlat; tex <= game.TexField; tex++ {
		if _, ok := texNames[tex]; !ok {
			t.Errorf("texture %d has no surface name", int(tex))
		}
	}
}

// Live creatures resolve their body through the same table as tile props, so a
// species added to the bestiary needs no browser change.
func TestEverySpeciesResolvesToAShape(t *testing.T) {
	for _, sp := range game.SpeciesList() {
		key, ok := propShapeKey(sp.Prop)
		if !ok {
			t.Errorf("species %q (prop %d) has no shape", sp.Kind, int(sp.Prop))
			continue
		}
		if !strings.HasPrefix(key, BuildCreature+":") {
			t.Errorf("species %q resolved to %q, which isn't a creature body", sp.Kind, key)
		}
	}
}

func propLabel(p game.TileProp) string {
	return "prop#" + itoa(int(p))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
