// Command wildlifepreview renders the Phase 1–3 wildlife as pictures: the biome
// fauna in a daylit clearing, a tamed companion trotting at heel, and the same
// scene under the night torch. Throwaway art tool, like lootpreview — it pulls
// the real Species table so the sprites/hues match what the game draws.
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"time"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

func grass(x, y int) game.Tile {
	return game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#5EAE63"}
}

// placeCreature stamps a species onto a tile exactly as the Wilds' sample() does
// — keeping the biome ground, overlaying the species glyph/hue/prop.
func placeCreature(tm *game.TileMap, x, y int, kind string) {
	sp, ok := game.SpeciesByKind(kind)
	if !ok {
		panic("unknown species " + kind)
	}
	t := tm.Tiles[y][x]
	t.Ch, t.Color, t.Prop, t.PropHex = sp.Glyph, sp.Hex, sp.Prop, sp.Hex
	tm.Tiles[y][x] = t
}

// clearing is a meadow with a forest edge and a pond, strewn with one of each
// species (the fish on the water) plus a tree line, to show the fauna in situ.
func clearing() *game.TileMap {
	const W, H = 24, 15
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			tiles[y][x] = grass(x, y)
		}
	}
	tm := &game.TileMap{W: W, H: H, Tiles: tiles}
	// A forest edge along the top-left.
	for _, p := range [][2]int{{2, 1}, {3, 1}, {5, 2}, {1, 3}, {4, 3}, {2, 4}} {
		tm.Tiles[p[1]][p[0]] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexForest,
			Ground: "#2E6B40", Prop: game.PropTree, PropHex: "#2E5E34"}
	}
	// A little pond on the right for the fish.
	for y := 9; y <= 12; y++ {
		for x := 18; x <= 22; x++ {
			tm.Tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexWater, Ground: "#2E6BFF"}
		}
	}
	// A scatter of flowers for life.
	for _, p := range [][2]int{{8, 2}, {14, 4}, {6, 11}, {16, 12}} {
		tm.Tiles[p[1]][p[0]] = game.Tile{Kind: game.TileObject, Walkable: true, Tex: game.TexGrass,
			Ground: "#5EAE63", Prop: game.PropFlower, PropHex: "#F2D24A"}
	}

	placeCreature(tm, 4, 5, "deer")
	placeCreature(tm, 9, 7, "rabbit")
	placeCreature(tm, 15, 6, "fox")
	placeCreature(tm, 12, 3, "bird")
	placeCreature(tm, 20, 10, "fish")
	return tm
}

// companionScene is a tight shot of the player with a tamed fox at heel and a
// wild deer a few tiles off — the difference a leash makes.
func companionScene() *game.TileMap {
	const W, H = 13, 9
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			tiles[y][x] = grass(x, y)
		}
	}
	tm := &game.TileMap{W: W, H: H, Tiles: tiles}
	for _, p := range [][2]int{{1, 1}, {11, 1}, {2, 7}, {10, 7}} {
		tm.Tiles[p[1]][p[0]] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexForest,
			Ground: "#2E6B40", Prop: game.PropTree, PropHex: "#2E5E34"}
	}
	placeCreature(tm, 7, 4, "fox")   // the companion, trotting just behind the player at (6,4)
	placeCreature(tm, 10, 2, "deer") // a wild one keeping its distance
	return tm
}

// speciesChart lines every species up large, one per pair of columns, so each
// silhouette is unmistakable at high zoom.
func speciesChart() *game.TileMap {
	kinds := []string{"rabbit", "deer", "fox", "bird", "fish"}
	const H = 3
	W := len(kinds)*2 + 1
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			tiles[y][x] = grass(x, y)
		}
	}
	tm := &game.TileMap{W: W, H: H, Tiles: tiles}
	for i, k := range kinds {
		x := i*2 + 1
		if k == "fish" { // give the fish its water
			tm.Tiles[1][x] = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexWater, Ground: "#2E6BFF"}
		}
		placeCreature(tm, x, 1, k)
	}
	return tm
}

func saveImg(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

func main() {
	style := game.DefaultStyle()
	const frame = 6
	if err := os.MkdirAll("wildlifeshots", 0o755); err != nil {
		panic(err)
	}

	// Midday, so the day-faded torch opens up and the fauna read in full color.
	noon := time.Date(2026, 6, 20, 12, 30, 0, 0, time.UTC)
	ui.Now = func() time.Time { return noon }

	// 1) The clearing in daylight (uniform light).
	cl := clearing()
	pl := []world.Player{{Name: "you", X: 11, Y: 8, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()}}
	img := game.RenderRGBA(nil, cl, pl, "you", frame, game.Camera{W: cl.W, H: cl.H}, game.Light{}, 0, 0, 30, false, style)
	saveImg("wildlifeshots/wilds-fauna-day.png", img)

	// 2) The companion at heel (zoomed in).
	cs := companionScene()
	cp := []world.Player{{Name: "you", X: 6, Y: 4, Color: "#FFC861", Facing: world.DirE, LastMoved: time.Now()}}
	cimg := game.RenderRGBA(nil, cs, cp, "you", frame, game.Camera{W: cs.W, H: cs.H}, game.Light{}, 0, 0, 44, false, style)
	saveImg("wildlifeshots/companion.png", cimg)

	// 3) The same clearing under the night torch — fauna inside the circle read,
	//    the rest sinks into dusk (the discovery light, exactly as in-game).
	night := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	ui.Now = func() time.Time { return night }
	torch := game.DayFadedLight(game.Light{X: 11 + game.PlayerW/2, Y: 8 + game.PlayerH/2, Radius: 7})
	nimg := game.RenderRGBA(nil, cl, pl, "you", frame, game.Camera{W: cl.W, H: cl.H}, torch, 0, 0, 30, false, style)
	saveImg("wildlifeshots/wilds-fauna-night.png", nimg)

	// 4) A zoomed lineup of every species (no player), back in daylight.
	ui.Now = func() time.Time { return noon }
	ch := speciesChart()
	chimg := game.RenderRGBA(nil, ch, nil, "", frame, game.Camera{W: ch.W, H: ch.H}, game.Light{}, 0, 0, 60, false, style)
	saveImg("wildlifeshots/species-chart.png", chimg)

	fmt.Println("wrote wilds-fauna-day, companion, wilds-fauna-night, species-chart")
}
