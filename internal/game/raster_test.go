package game

import (
	"testing"
	"time"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/durst-group/durstworld/internal/world"
)

// smokeMap is a tiny 3×2 map exercising textured ground, a prop and a portal.
func smokeMap() *TileMap {
	tiles := [][]Tile{
		{
			{Kind: TileFloor, Walkable: true, Tex: TexGrass, Ground: "#3A7D44", Color: "#3A7D44"},
			{Kind: TileFloor, Walkable: true, Tex: TexGrass, Ground: "#3A7D44", Prop: PropTree, PropHex: "#2E5E34"},
			{Kind: TileFloor, Walkable: true, Tex: TexWater, Ground: "#2E6BFF", Color: "#2E6BFF"},
		},
		{
			{Kind: TilePortal, Walkable: true, Prop: PropPortal, PropHex: "#7DF0FF", Portal: "lobby"},
			{Kind: TileFloor, Walkable: true, Tex: TexSand, Ground: "#D9C58B"},
			{Kind: TileWall},
		},
	}
	return &TileMap{W: 3, H: 2, Tiles: tiles}
}

// RenderRGBA must produce a W*scale × H*scale image for every art style, both
// flat and smooth, without panicking — and tolerate a nil style.
func TestRenderRGBASmoke(t *testing.T) {
	tm := smokeMap()
	players := []world.Player{{Name: "you", X: 1, Y: 0, Color: "#FFC861", LastMoved: time.Now()}}
	cam := Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	const scale = 8

	for _, name := range []string{"default", "gameboy", "neon"} {
		st := StyleByName(name)
		for _, smooth := range []bool{false, true} {
			img := RenderRGBA(nil, tm, players, "you", 0, cam, Light{}, 0, 0, scale, smooth, st)
			if got := img.Bounds().Dx(); got != tm.W*scale {
				t.Errorf("style %s smooth=%v: width %d, want %d", name, smooth, got, tm.W*scale)
			}
			if got := img.Bounds().Dy(); got != tm.H*scale {
				t.Errorf("style %s smooth=%v: height %d, want %d", name, smooth, got, tm.H*scale)
			}
		}
	}

	// A nil style falls back to DefaultStyle rather than panicking.
	if img := RenderRGBA(nil, tm, players, "you", 0, cam, Light{}, 0, 0, scale, false, nil); img == nil {
		t.Fatal("nil style returned a nil image")
	}
}

// The gameboy palette maps every pixel onto its 4-tone green ramp.
func TestGameboyMapStaysOnRamp(t *testing.T) {
	for _, hex := range []string{"#FF0000", "#00FF00", "#1234AB", "#FFFFFF", "#000000"} {
		out := gameboyMap(mustHex(hex))
		onRamp := false
		for _, shade := range gbShades {
			if near(out, shade) {
				onRamp = true
				break
			}
		}
		if !onRamp {
			t.Errorf("gameboyMap(%s) = %v is not on the DMG ramp", hex, out)
		}
	}
}

// A rendered gameboy frame must actually use both shade sets — terrain on the
// middle shades and the portal/avatar sprites on the reserved dark/light shades —
// proving the salience mask is threaded end-to-end through the draw paths, not
// just present in the map function.
func TestGameboyRenderSeparatesSpritesFromTerrain(t *testing.T) {
	tm := smokeMap() // has a portal (salient) and textured ground (terrain)
	players := []world.Player{{Name: "you", X: 1, Y: 0, Color: "#FFC861", LastMoved: time.Now()}}
	cam := Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	img := RenderRGBA(nil, tm, players, "you", 0, cam, Light{}, 0, 0, 16, false, StyleByName("gameboy"))

	sprite := []colorful.Color{gbShades[0], gbShades[3]} // reserved for salient
	terrain := []colorful.Color{gbShades[1], gbShades[2]}
	hasSprite, hasTerrain := false, false
	for i := 0; i+3 < len(img.Pix); i += 4 {
		c := colorful.Color{R: float64(img.Pix[i]) / 255, G: float64(img.Pix[i+1]) / 255, B: float64(img.Pix[i+2]) / 255}
		for _, s := range sprite {
			if near(c, s) {
				hasSprite = true
			}
		}
		for _, s := range terrain {
			if near(c, s) {
				hasTerrain = true
			}
		}
	}
	if !hasSprite {
		t.Error("no reserved sprite shade in the gameboy frame — salient elements not separated")
	}
	if !hasTerrain {
		t.Error("no terrain shade in the gameboy frame — recolor not applied")
	}
}

// gameboyMapSalient must keep terrain and gameplay sprites on disjoint shades, so
// an item can never share a shade with same-luminance terrain and disappear.
func TestGameboyMapSalientSeparatesSprites(t *testing.T) {
	bg := map[colorful.Color]bool{} // shades terrain may use
	fg := map[colorful.Color]bool{} // shades sprites may use
	for _, hex := range []string{"#FF0000", "#00FF00", "#1234AB", "#FFFFFF", "#000000", "#7DF0FF", "#FFC861"} {
		c := mustHex(hex)
		bg[gameboyMapSalient(c, false)] = true
		fg[gameboyMapSalient(c, true)] = true
	}
	for _, set := range []map[colorful.Color]bool{bg, fg} {
		for shade := range set {
			onRamp := false
			for _, s := range gbShades {
				if near(shade, s) {
					onRamp = true
					break
				}
			}
			if !onRamp {
				t.Errorf("gameboyMapSalient produced %v off the DMG ramp", shade)
			}
		}
	}
	for s := range bg {
		if fg[s] {
			t.Errorf("shade %v used for both terrain and sprites — items can vanish", s)
		}
	}
}
