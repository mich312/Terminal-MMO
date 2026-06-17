// Package kraftwerk is a placeholder area: a small machine hall with
// flavor text and a portal back to the lobby. It is the worked example of a
// game.FlavorArea — map data plus a single RegisterFlavor call.
package kraftwerk

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"##############################",
	"#..o....................o....#",
	"#..mmm....mmm....mmm.........#",
	"#..mmm....mmm....mmm.........#",
	"#............................#",
	"0...........~~~~~~~~.........#",
	"#............................#",
	"#.......mmmmm....mmmmm.......#",
	"#.......mmmmm....mmmmm.......#",
	"#..o....................o....#",
	"#............................#",
	"##############################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	// Machines hum: glyph pulses through block shades, color cycles cold→hot.
	'm': {Kind: game.TileDecor, Ch: '▓', Anim: &game.TileAnim{
		Frames: []rune{'▓', '▒', '░', '▒'}, ColorA: "#3A4654", ColorB: "#7DF0FF", Speed: 2}},
	// Coolant channel: flowing water glyphs in blues.
	'~': {Kind: game.TileDecor, Ch: '~', Anim: &game.TileAnim{
		Frames: []rune{'~', '≈', '~', '≋'}, ColorA: "#2E6BFF", ColorB: "#56E1FF", Speed: 3}},
	// Lamps: warm flicker, and the only real light in the hall.
	'o': {Kind: game.TileObject, Ch: '◉', Anim: &game.TileAnim{
		ColorA: "#FF8A4C", ColorB: "#FFC861", Speed: 2}},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "kraftwerk", Display: "Kraftwerk",
		Rows: rows, Legend: legend,
		SpawnX: 2, SpawnY: 5, Jitter: 1,
		Title: "⚡ Durst Kraftwerk",
		Body:  "Home of spin-offs and experiments.\n\nThe machines are warming up — mind the coolant.",
		// The hall sits in shadow; the player's lamp reaches 9 tiles.
		Light: 9,
	})
}
