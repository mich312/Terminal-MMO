// Command nightpreview renders a single representative Wilds-style scene at
// several points around the day/night cycle and writes them out as PNGs, so the
// night look can be reviewed as real frames rather than read off the code. It is
// a throwaway art tool — not part of the server — and lives here only to make
// "show me how night looks" answerable with pictures.
package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// previewNoise is a cheap smoothed value-noise in [0,1] for laying out an
// organic tree stand — just enough coherence to clump trees rather than scatter
// them as static. (Local to the art tool; the engine has its own.)
func previewNoise(x, y int) float64 {
	hash := func(a, b int) float64 {
		h := uint32(a*374761393+b*668265263) ^ 0x9E3779B1
		h = (h ^ (h >> 13)) * 1274126177
		return float64((h^(h>>16))&0xffff) / 65536
	}
	fx, fy := float64(x)/3, float64(y)/3
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := fx-float64(x0), fy-float64(y0)
	tx, ty = tx*tx*(3-2*tx), ty*ty*(3-2*ty)
	top := hash(x0, y0) + (hash(x0+1, y0)-hash(x0, y0))*tx
	bot := hash(x0, y0+1) + (hash(x0+1, y0+1)-hash(x0, y0+1))*tx
	return top + (bot-top)*ty
}

// scene hand-builds a small forest clearing: grass and a pond, a ring of trees,
// a traveler's campfire, a lamp post, an active portal and a couple of glowing
// crystals — i.e. one of every night-time light source the engine knows how to
// bloom, so a single frame exercises the whole night palette.
func scene() *game.TileMap {
	const W, H = 16, 11
	grass := func() game.Tile {
		return game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#3A7D44"}
	}
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			tiles[y][x] = grass()
		}
	}
	tree := func(x, y int) {
		tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexForest,
			Ground: "#2C5E33", Prop: game.PropTree, PropHex: "#2E5E34"}
	}
	// A loose ring of trees around the clearing.
	for x := 0; x < W; x++ {
		tree(x, 0)
		tree(x, H-1)
	}
	for y := 0; y < H; y++ {
		tree(0, y)
		tree(W-1, y)
	}
	tree(3, 2)
	tree(12, 2)
	tree(2, 8)
	tree(13, 8)

	// A pond in the lower-left to catch the moon-glitter and breathe mist.
	for y := 6; y <= 8; y++ {
		for x := 3; x <= 6; x++ {
			tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexWater, Ground: "#2E6BFF"}
		}
	}
	// A swampy margin along the pond's edge — also a mist source at night.
	for x := 7; x <= 9; x++ {
		tiles[8][x] = game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexSwamp, Ground: "#4A5A3A"}
	}

	// A little cottage on the right, so its windows light up after dark.
	tiles[7][12] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexGrass,
		Ground: "#3A7D44", Prop: game.PropHouse, PropHex: "#B07A4A"}

	set := func(x, y int, prop game.TileProp, hex string) {
		t := tiles[y][x]
		t.Prop, t.PropHex = prop, hex
		tiles[y][x] = t
	}
	// Light sources scattered through the clearing.
	set(8, 5, game.PropCampfire, "#FF7A2C")  // warm, flickering fire
	set(11, 6, game.PropLamp, "#FFD27A")     // steady amber lamp
	set(5, 3, game.PropGemGlow, "#7DF0FF")   // luminous crystal
	set(10, 8, game.PropGemGlow, "#C792EA")  // luminous mushroom
	tiles[4][13] = game.Tile{Kind: game.TilePortal, Walkable: true, Tex: game.TexGrass,
		Ground: "#3A7D44", Prop: game.PropPortal, PropHex: "#7DF0FF", Portal: "lobby"}

	return &game.TileMap{W: W, H: H, Tiles: tiles}
}

// forestScene is an organic woodland — a dense, naturally-clumped stand of trees
// with a small clearing, a stream and a campfire — rather than a hand-drawn ring,
// so the "forest at night" look can be judged as it actually generates in play.
func forestScene() *game.TileMap {
	const W, H = 22, 14
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			// Coherent noise picks tree density; a clearing is carved in the middle.
			n := previewNoise(x, y)
			cx, cy := float64(x-11)/6, float64(y-7)/4
			clearing := cx*cx+cy*cy < 1.1
			t := game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#3A7D44"}
			if n > 0.52 && !clearing {
				t = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexForest,
					Ground: "#2C5E33", Prop: game.PropTree, PropHex: "#2E5E34"}
			}
			tiles[y][x] = t
		}
	}
	// A stream snaking through, and a campfire in the clearing.
	for y := 0; y < H; y++ {
		x := 4 + y/3
		tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexWater, Ground: "#2E6BFF"}
	}
	tiles[7][11] = game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass,
		Ground: "#3A7D44", Prop: game.PropCampfire, PropHex: "#FF7A2C"}
	return &game.TileMap{W: W, H: H, Tiles: tiles}
}

// shots are the cycle points we render, labelled for the filename. The cycle is
// compressed into one real hour (see ui.CyclePeriod), so the minute-of-hour
// drives the ring: minute 0 = midnight, 15 = dawn, 30 = noon, 45 = dusk.
var shots = []struct {
	name string
	min  int
}{
	{"1-noon", 30},
	{"2-golden-dusk", 44},
	{"3-twilight", 47},
	{"4-deep-night", 0},
}

func main() {
	tm := scene()
	players := []world.Player{
		{Name: "you", X: 7, Y: 6, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()},
	}
	style := game.DefaultStyle()
	cam := game.Camera{X: 0, Y: 0, W: tm.W, H: tm.H}
	const scale = 28
	const frame = 7 // a fixed frame so flames/fireflies are mid-animation

	if err := os.MkdirAll("nightshots", 0o755); err != nil {
		panic(err)
	}
	for _, s := range shots {
		// Pin the renderer's clock to this point in the cycle.
		at := time.Date(2026, 6, 20, 10, s.min, 0, 0, time.UTC)
		ui.Now = func() time.Time { return at }

		img := game.RenderRGBA(nil, tm, players, "you", frame, cam, game.Light{}, 0, 0, scale, false, style)
		path := fmt.Sprintf("nightshots/%s.png", s.name)
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		hex, str := ui.Ambient(at)
		fmt.Printf("wrote %s  (ambient tint %s @ %.0f%%)\n", path, hex, str*100)
	}

	// Organic forest at deep night under two lunar phases, to show the moonlight
	// pegging: scan the synodic month around today for its darkest (new) and
	// brightest (full) nights and render the forest under each.
	fst := forestScene()
	fplayers := []world.Player{{Name: "you", X: 11, Y: 7, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()}}
	base := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	newMoon, fullMoon := base, base
	lo, hi := 2.0, -1.0
	for d := 0; d < 30; d++ {
		day := base.AddDate(0, 0, d)
		if il := ui.MoonIllumination(day); il < lo {
			lo, newMoon = il, day
		} else if il > hi {
			hi, fullMoon = il, day
		}
	}
	for _, m := range []struct {
		name string
		when time.Time
	}{
		{"5-forest-newmoon", newMoon},
		{"6-forest-fullmoon", fullMoon},
	} {
		ui.Now = func() time.Time { return m.when } // date sets the phase; 00:00 = deep night
		img := game.RenderRGBA(nil, fst, fplayers, "you", frame, game.Camera{W: fst.W, H: fst.H}, game.Light{}, 0, 0, scale, false, style)
		path := fmt.Sprintf("nightshots/%s.png", m.name)
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		fmt.Printf("wrote %s  (%s, moon %.0f%% lit)\n", path, m.when.Format("2006-01-02"), ui.MoonIllumination(m.when)*100)
	}
}
