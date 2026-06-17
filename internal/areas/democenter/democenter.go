// Package democenter is the Durst showroom: a grand gallery of exhibit wings
// around a central fountain, with plinths and sparkling pieces under spotlights,
// demo screens and plotters, and a portal back to the lobby. Built as a
// game.FlavorArea (map data plus one RegisterFlavor call); the legend drives
// both the glyph and HD pixel renderers.
package democenter

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"########......####......####......####......########",
	"########......####......####......####......########",
	"########.i..i.####.i..i.####.i..i.####.i..i.########",
	"#######s.d..d.ssss.d..d.rrr..d..d.ssss.d..d.########",
	"######..................rrr..................#######",
	"#####.g..o..o......o..o.rrr..o..o......o..o.g.######",
	"####....................rrr....................#####",
	"####....................rrr....................#####",
	"####....................rrr....................#####",
	"####...........................................#####",
	"####.rrrrrrrrrrrrrrrrrr......rrrrrrrrrrrrrrrrr.....#",
	"####.rrrrrrrrrrrrrrrrrr..FF..rrrrrrrrrrrrrrrrr.....0",
	"####.rrrrrrrrrrrrrrrrrr..FF..rrrrrrrrrrrrrrrrr.....#",
	"####...........................................#####",
	"####....................rrr....................#####",
	"####....................rrr.............PPPPP..#####",
	"####....................rrr.............PPPPP..#####",
	"#####.g.................rrr.............PPPPP.######",
	"######..................rrr..................#######",
	"#######..i..i......i..i.rrr..i..i......i..i.########",
	"########.d..d.####.d..d.####.d..d.####.d..d.########",
	"########......####......####......####......########",
	"########......####......####......####......########",
	"####################################################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	// Gallery floor (a touch warmer than the default).
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#2E3138"},
	// Carpet runners (walkable, warm red).
	'r': {Kind: game.TileFloor, Ch: '▒', Walkable: true, Color: "#7A3340", Tex: game.TexFloor, Ground: "#7A3340"},
	// Demo screens mounted on the back wall, cycling through their reels.
	's': {Kind: game.TileDecor, Ch: '▣', Tex: game.TexBrick, Ground: "#20242B", Prop: game.PropScreen, PropHex: "#2E8BFF", Anim: &game.TileAnim{
		ColorA: "#2E6BFF", ColorB: "#7DF0FF", Speed: 3}},
	// Plotters at work: trays shuffle, steel→paper-white.
	'P': {Kind: game.TileDecor, Ch: '▤', Tex: game.TexFloor, Ground: "#2A2F37", Prop: game.PropMachine, PropHex: "#9AA3AD", Anim: &game.TileAnim{
		Frames: []rune{'▤', '▦', '▤', '▥'}, ColorA: "#6B7480", ColorB: "#C2CBD6", Speed: 5}},
	// Exhibit plinths.
	'd': {Kind: game.TileDecor, Ch: '▆', Color: "#8A93A0", Tex: game.TexFloor, Ground: "#2E3138", Prop: game.PropPlinth, PropHex: "#9AA0A8"},
	// Showcased pieces glinting atop each plinth.
	'i': {Kind: game.TileDecor, Ch: '◈', Tex: game.TexFloor, Ground: "#2E3138", Prop: game.PropGem, PropHex: "#FFD36B", Anim: &game.TileAnim{
		Frames: []rune{'◈', '◇', '◆', '◇'}, ColorA: "#FFC861", ColorB: "#7DF0FF", Speed: 3}},
	// Spotlights: bright, near-white.
	'o': {Kind: game.TileObject, Ch: '◉', Tex: game.TexFloor, Ground: "#2E3138", Prop: game.PropLamp, PropHex: "#FFE3A0", Anim: &game.TileAnim{
		ColorA: "#FFE3A0", ColorB: "#FFFFFF", Speed: 4}},
	// Plants for a little warmth.
	'g': {Kind: game.TileDecor, Ch: '♣', Tex: game.TexGrass, Ground: "#3F8A5A", Prop: game.PropBush, PropHex: "#7BD88F", Anim: &game.TileAnim{
		ColorA: "#4FD6BE", ColorB: "#7BD88F", Speed: 3}},
	// Central fountain centerpiece: glowing water.
	'F': {Kind: game.TileDecor, Ch: '◉', Tex: game.TexWater, Ground: "#2E6BFF", Prop: game.PropFountain, PropHex: "#7DF0FF", Anim: &game.TileAnim{
		Frames: []rune{'◉', '◍', '◉', '◌'}, ColorA: "#2E6BFF", ColorB: "#9BE8FF", Speed: 3}},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "democenter", Display: "Demo Center",
		Rows: rows, Legend: legend,
		SpawnX: 47, SpawnY: 11, Jitter: 0,
		Title:     "🖨 Demo Center",
		Body:      "Showroom of Durst output.\n\nWander the wings — each plinth holds a piece.",
		PanelLeft: true,
	})
}
