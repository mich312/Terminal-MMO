// Package arcade is the Durst Arcade: a neon-lit hall of game cabinets ranged
// along the walls, a giant marquee screen at the back, a prize counter, and a
// portal home to the lobby. Built as a game.FlavorArea (map data plus one
// RegisterFlavor call); the legend drives both the glyph and HD pixel renderers.
package arcade

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"##############################################",
	"#............................................#",
	"#..AA....AA....AA....AA........@@@@@@@@@.....#",
	"#..AA....AA....AA....AA........@@@@@@@@@.....#",
	"#..............................@@@@@@@@@.....#",
	"#..............................@@@@@@@@@.....#",
	"#.o........................................o.#",
	"#............................................#",
	"0............................................#",
	"#............................................#",
	"#.......................................o....#",
	"#..AA....AA....AA....AA......................#",
	"#..AA....AA....AA....AA......................#",
	"#.o.................................o........#",
	"#.......====================.................#",
	"#.......====================.................#",
	"##############################################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	// Dark arcade carpet and walls.
	'.': {Kind: game.TileFloor, Ch: '·', Walkable: true, Tex: game.TexFloor, Ground: "#1B1430"},
	'#': {Kind: game.TileWall, Ch: '█', Tex: game.TexBrick, Ground: "#2A2150"},
	// Arcade cabinets: a glowing screen that cycles neon (HD: PropScreen).
	'A': {Kind: game.TileDecor, Ch: '▟', Tex: game.TexFloor, Ground: "#241A3E", Prop: game.PropScreen, PropHex: "#56E1FF", Anim: &game.TileAnim{
		Frames: []rune{'▜', '▟', '▙', '▛'}, ColorA: "#FF5FA2", ColorB: "#56E1FF", Speed: 2}},
	// The back-wall marquee: a big animated display.
	'@': {Kind: game.TileDecor, Ch: '▒', Tex: game.TexFloor, Ground: "#160E2E", Prop: game.PropScreen, PropHex: "#C792EA", Anim: &game.TileAnim{
		Frames: []rune{'░', '▒', '▓', '▒'}, ColorA: "#C792EA", ColorB: "#FFC861", Speed: 1}},
	// Prize counter along the floor (HD: a crate-style desk block).
	'=': {Kind: game.TileDecor, Ch: '▄', Tex: game.TexFloor, Ground: "#3A2A14", Prop: game.PropCrate, PropHex: "#FFC861"},
	// Neon floor lamps: a warm-to-magenta pulse (HD: PropLamp glow).
	'o': {Kind: game.TileObject, Ch: '◉', Tex: game.TexFloor, Ground: "#241A3E", Prop: game.PropLamp, PropHex: "#FF5FA2", Anim: &game.TileAnim{
		ColorA: "#FF5FA2", ColorB: "#56E1FF", Speed: 2}},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "arcade", Display: "Arcade",
		Rows: rows, Legend: legend,
		SpawnX: 3, SpawnY: 8, Jitter: 1,
		Title: "🎮 Durst Arcade",
		Body:  "Rows of cabinets hum in the dark, screens flickering neon.\n\nWander the aisles — and step back through the portal to return to the lobby.",
		// A dark hall lit by the glow of the machines and the player's own light.
		Light: 15,
	})
}
