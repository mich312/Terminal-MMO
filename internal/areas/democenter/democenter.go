// Package democenter is a placeholder area: the Durst showroom — a carpeted
// hall of exhibit plinths under spotlights, with demo screens and plotters
// along the back wall and a portal back to the lobby. Built as a
// game.FlavorArea (map data plus one RegisterFlavor call).
package democenter

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"####################################",
	"#...sssssssssss......sssssssssss...#",
	"#.g.PPPPPP................PPPPPP.g.#",
	"#...PPPPPP................PPPPPP...#",
	"#.....o.........o.........o........#",
	"#.....i....i....i....i....i....i...#",
	"#.....d....d....d....d....d....d...#",
	"#rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr#",
	"#rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr0",
	"#..................................#",
	"#.....i....i....i....i....i....i...#",
	"#.....d....d....d....d....d....d...#",
	"#..........o.........o.........o...#",
	"#.g..............................g.#",
	"#..................................#",
	"####################################",
}

// The legend drives both renderers: Ch/Color/Anim are the glyph look; Tex,
// Ground and Prop are the HD pixel look (carpet, plinths, screens, plotters).
var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	// Carpet runner down the central aisle (walkable, warm red).
	'r': {Kind: game.TileFloor, Ch: '▒', Walkable: true, Color: "#7A3340", Tex: game.TexFloor, Ground: "#7A3340"},
	// Demo screens mounted on the back wall, cycling through their reels.
	's': {Kind: game.TileDecor, Ch: '▣', Tex: game.TexBrick, Ground: "#20242B", Prop: game.PropScreen, PropHex: "#2E8BFF", Anim: &game.TileAnim{
		ColorA: "#2E6BFF", ColorB: "#7DF0FF", Speed: 3}},
	// Plotters/printers at work: trays shuffle, steel→paper-white.
	'P': {Kind: game.TileDecor, Ch: '▤', Tex: game.TexFloor, Ground: "#2A2F37", Prop: game.PropMachine, PropHex: "#9AA3AD", Anim: &game.TileAnim{
		Frames: []rune{'▤', '▦', '▤', '▥'}, ColorA: "#6B7480", ColorB: "#C2CBD6", Speed: 5}},
	// Exhibit plinths.
	'd': {Kind: game.TileDecor, Ch: '▆', Color: "#8A93A0", Tex: game.TexFloor, Ground: "#2A2F37", Prop: game.PropPlinth, PropHex: "#9AA0A8"},
	// Showcased pieces sparkling atop each plinth.
	'i': {Kind: game.TileDecor, Ch: '◈', Tex: game.TexFloor, Ground: "#2A2F37", Prop: game.PropGem, PropHex: "#FFC861", Anim: &game.TileAnim{
		Frames: []rune{'◈', '◇', '◆', '◇'}, ColorA: "#FFC861", ColorB: "#7DF0FF", Speed: 3}},
	// Spotlights: bright, near-white.
	'o': {Kind: game.TileObject, Ch: '◉', Tex: game.TexFloor, Ground: "#2A2F37", Prop: game.PropLamp, PropHex: "#FFE3A0", Anim: &game.TileAnim{
		ColorA: "#FFE3A0", ColorB: "#FFFFFF", Speed: 4}},
	// Lobby-style plants for a little warmth.
	'g': {Kind: game.TileDecor, Ch: '♣', Tex: game.TexGrass, Ground: "#3F8A5A", Prop: game.PropBush, PropHex: "#7BD88F", Anim: &game.TileAnim{
		ColorA: "#4FD6BE", ColorB: "#7BD88F", Speed: 3}},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "democenter", Display: "Demo Center",
		Rows: rows, Legend: legend,
		SpawnX: 33, SpawnY: 8, Jitter: 0,
		Title:     "🖨 Demo Center",
		Body:      "Showroom of Durst output.\n\nWander the plinths — the printers are warming up.",
		PanelLeft: true,
	})
}
