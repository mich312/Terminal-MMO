// Command lootpreview renders two review scenes that exercise the recent
// rendering fixes: loot/forage props under the Wilds' day-faded torch at deep
// night (they should now fade into the dark with the ground instead of glowing),
// and a stretch of cavern under the lantern (walls vs. floor contrast). Throwaway
// art tool, like nightpreview — it lives here only to make the fixes reviewable
// as pictures.
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

// wildsLoot lays a small clearing strewn with non-glowing loot/forage (gems,
// stone, logs, fish) plus one genuinely luminous crystal, so the difference
// between "the torch finds it" and "it shines on its own" is visible at night.
func wildsLoot() *game.TileMap {
	const W, H = 15, 11
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			tiles[y][x] = game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#3A7D44"}
		}
	}
	set := func(x, y int, prop game.TileProp, hex, ground string) {
		tiles[y][x] = game.Tile{Kind: game.TileObject, Walkable: true, Tex: game.TexGrass,
			Ground: ground, Prop: prop, PropHex: hex}
	}
	// A spread of mundane loot at varying distances from the player at (7,5).
	set(3, 2, game.PropGem, "#E7C84B", "#3A7D44")   // gold gem (the classic "always bright" offender)
	set(11, 2, game.PropGem, "#5BD8E0", "#3A7D44")  // teal gem
	set(2, 8, game.PropStone, "#B7B1A6", "#6B6B63") // cut stone
	set(12, 8, game.PropLog, "#9C6B3F", "#3A7D44")  // log pile
	set(5, 9, game.PropFish, "#A9C7D6", "#6E5A40")  // a catch
	set(9, 9, game.PropGem, "#E0604D", "#3A7D44")   // ruby gem
	// One luminous crystal — this one SHOULD still glow at night, by design.
	set(7, 2, game.PropGemGlow, "#7DF0FF", "#3A7D44")
	return &game.TileMap{W: W, H: H, Tiles: tiles}
}

// cavern lays a chamber of rock wall around an open floor, with a glowing
// mushroom and a non-glowing stalagmite, to judge wall-vs-floor contrast under
// the lantern.
func cavern() *game.TileMap {
	const W, H = 15, 11
	rockWall := game.Tile{Kind: game.TileWall, Ch: '▓', Walkable: false, Color: "#4E4656", Tex: game.TexRock, Ground: "#2E2935"}
	caveFloor := game.Tile{Kind: game.TileFloor, Ch: '·', Walkable: true, Color: "#A39AA9", Tex: game.TexDirt, Ground: "#746C7C"}
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			tiles[y][x] = caveFloor
		}
	}
	// Surround with wall, plus a couple of internal spurs so edges read.
	for x := 0; x < W; x++ {
		tiles[0][x], tiles[H-1][x] = rockWall, rockWall
	}
	for y := 0; y < H; y++ {
		tiles[y][0], tiles[y][W-1] = rockWall, rockWall
	}
	tiles[3][4], tiles[3][5], tiles[4][4] = rockWall, rockWall, rockWall
	tiles[7][10], tiles[8][10] = rockWall, rockWall
	// A little glowing life so the lantern isn't the only light.
	tiles[8][3] = game.Tile{Kind: game.TileObject, Ch: 'ψ', Walkable: true, Color: "#7CF2C4",
		Tex: game.TexDirt, Ground: "#746C7C", Prop: game.PropCaveShroom, PropHex: "#7CF2C4"}
	// A non-glowing stalagmite — should fade with distance like the wall.
	tiles[3][11] = game.Tile{Kind: game.TileFloor, Ch: '▲', Walkable: true, Color: "#B9B0BE",
		Tex: game.TexRock, Ground: "#746C7C", Prop: game.PropStalagmite, PropHex: "#9A92A0"}
	return &game.TileMap{W: W, H: H, Tiles: tiles}
}

// wideWilds is a large explored map: forest/grass with scattered glowing forage
// (mushrooms, crystals) far beyond the player's small torch — to show how
// luminous loot reads across the whole night map.
func wideWilds() *game.TileMap {
	const W, H = 40, 26
	tiles := make([][]game.Tile, H)
	for y := 0; y < H; y++ {
		tiles[y] = make([]game.Tile, W)
		for x := 0; x < W; x++ {
			t := game.Tile{Kind: game.TileFloor, Walkable: true, Tex: game.TexGrass, Ground: "#3A7D44"}
			if (x*7+y*13+x*y)%11 < 4 {
				t = game.Tile{Kind: game.TileFloor, Walkable: false, Tex: game.TexForest,
					Ground: "#2C5E33", Prop: game.PropTree, PropHex: "#2E5E34"}
			}
			tiles[y][x] = t
		}
	}
	for y := 1; y < H-1; y += 3 {
		for x := 1; x < W-1; x += 3 {
			if (x*y)%2 == 0 {
				hex := "#C792EA" // mushroom purple
				if (x+y)%3 == 0 {
					hex = "#7DF0FF" // crystal teal
				}
				tiles[y][x] = game.Tile{Kind: game.TileObject, Walkable: true, Tex: game.TexGrass,
					Ground: "#3A7D44", Prop: game.PropGemGlow, PropHex: hex}
			} else {
				tiles[y][x] = game.Tile{Kind: game.TileObject, Walkable: true, Tex: game.TexGrass,
					Ground: "#3A7D44", Prop: game.PropGem, PropHex: "#E0604D"}
			}
		}
	}
	return &game.TileMap{W: W, H: H, Tiles: tiles}
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
	const scale = 28
	const frame = 7
	if err := os.MkdirAll("lootshots", 0o755); err != nil {
		panic(err)
	}

	// Deep night for the Wilds shot, so the torch is at full vignette.
	night := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	ui.Now = func() time.Time { return night }

	wl := wildsLoot()
	wplayers := []world.Player{{Name: "you", X: 7, Y: 5, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()}}
	torch := game.DayFadedLight(game.Light{X: 7 + game.PlayerW/2, Y: 5 + game.PlayerH/2, Radius: 7})
	wimg := game.RenderRGBA(nil, wl, wplayers, "you", frame, game.Camera{W: wl.W, H: wl.H}, torch, 0, 0, scale, false, style)
	saveImg("lootshots/wilds-night-loot.png", wimg)

	cv := cavern()
	cplayers := []world.Player{{Name: "you", X: 7, Y: 5, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()}}
	lantern := game.Light{X: 7 + game.PlayerW/2, Y: 5 + game.PlayerH/2, Radius: 11, Warm: true}
	cimg := game.RenderRGBA(nil, cv, cplayers, "you", frame, game.Camera{W: cv.W, H: cv.H}, lantern, 0, 0, scale, false, style)
	saveImg("lootshots/cave-lantern.png", cimg)

	// Wide night map: glowing forage scattered far past the small torch.
	ww := wideWilds()
	wwp := []world.Player{{Name: "you", X: 20, Y: 13, Color: "#FFC861", Facing: world.DirS, LastMoved: time.Now()}}
	wtorch := game.DayFadedLight(game.Light{X: 20 + game.PlayerW/2, Y: 13 + game.PlayerH/2, Radius: 7})
	wwimg := game.RenderRGBA(nil, ww, wwp, "you", frame, game.Camera{W: ww.W, H: ww.H}, wtorch, 0, 0, 18, false, style)
	saveImg("lootshots/wide-wilds-night.png", wwimg)

	fmt.Println("wrote wilds-night-loot, cave-lantern, wide-wilds-night")
}
