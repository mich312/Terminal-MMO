package wilds

import (
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// A snow-covered cell renders as snow in HD — white ground, snow texture (so
// the weather layer's fronts fall frozen on it) — while its props keep their
// own colors and everything structural stays put.
func TestCellTileSeasonalSnow(t *testing.T) {
	bare := worldgen.Cell{Biome: worldgen.Grass, Glyph: '·', Color: "#5EAE63", Walkable: true}
	dusted := bare
	dusted.Snowy, dusted.Color = true, "#E8EEF5"

	if tt := CellTile(bare); tt.Tex != game.TexGrass {
		t.Fatalf("bare grass tex = %v, want TexGrass", tt.Tex)
	}
	wt := CellTile(dusted)
	if wt.Tex != game.TexSnow {
		t.Fatalf("dusted tex = %v, want TexSnow", wt.Tex)
	}
	if !wt.Walkable {
		t.Fatal("snow cover must not change walkability")
	}

	tree := worldgen.Cell{Biome: worldgen.Forest, Glyph: '♣', Color: "#2E7A44", Snowy: true}
	tr := CellTile(tree)
	if tr.Tex != game.TexSnow || tr.Ground != groundColor(worldgen.Snow) {
		t.Fatalf("winter forest floor = tex %v ground %q, want snow", tr.Tex, tr.Ground)
	}
	if tr.Prop != game.PropTree || tr.PropHex != "#2E7A44" {
		t.Fatal("the tree itself keeps its species and color over the snow")
	}
}
