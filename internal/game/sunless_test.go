package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// TestSunlessIgnoresSurfaceClock is the cave's contract: a Sunless scene (an
// underground world) renders the same dark, glow-lit frame whatever the hour is
// on the surface — so a cavern never floods with daylight at noon and its
// bioluminescence always blooms. A non-sunless light, by contrast, must differ
// between noon and midnight (it tracks the sky).
func TestSunlessIgnoresSurfaceClock(t *testing.T) {
	orig := ui.Now
	defer func() { ui.Now = orig }()

	floor := Tile{Kind: TileFloor, Color: "#A39AA9", Tex: TexDirt, Ground: "#746C7C"}
	shroom := Tile{Kind: TileObject, Color: "#7CF2C4", Tex: TexDirt, Ground: "#746C7C", Prop: PropCaveShroom, PropHex: "#7CF2C4"}
	const vw, vh, scale = 12, 9, 16
	tm := &TileMap{W: vw, H: vh, Tiles: make([][]Tile, vh)}
	for y := 0; y < vh; y++ {
		tm.Tiles[y] = make([]Tile, vw)
		for x := 0; x < vw; x++ {
			tm.Tiles[y][x] = floor
		}
	}
	tm.Tiles[3][3] = shroom
	players := []world.Player{{Name: "me", X: 6, Y: 4, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now().Add(-time.Hour)}}
	style := DefaultStyle()

	noon := time.Date(2026, 6, 16, 10, 30, 0, 0, time.UTC)    // surface midday
	midnight := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC) // surface deep night
	render := func(at time.Time, light Light) []byte {
		ui.Now = func() time.Time { return at }
		return RenderRGBA(nil, tm, players, "me", 7, Camera{W: vw, H: vh}, light, 0, 0, scale, false, style).Pix
	}

	sun := Light{X: 6, Y: 4, Radius: 9, Warm: true, Sunless: true}
	if !bytes.Equal(render(noon, sun), render(midnight, sun)) {
		t.Error("Sunless scene differs between surface noon and midnight — it should be clock-independent")
	}

	// Sanity: drop Sunless and the same scene must track the surface clock.
	tracks := Light{X: 6, Y: 4, Radius: 9, Warm: true}
	if bytes.Equal(render(noon, tracks), render(midnight, tracks)) {
		t.Error("non-sunless scene identical at noon and midnight — expected it to follow the day/night cycle")
	}
}
