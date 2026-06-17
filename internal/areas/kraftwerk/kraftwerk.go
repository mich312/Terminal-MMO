// Package kraftwerk is a placeholder area: the Durst power hall — a glowing
// reactor core you can circle, flanked by turbine banks, coolant lines and a
// console bay, with a portal back to the lobby. Built as a game.FlavorArea
// (map data plus a single RegisterFlavor call).
package kraftwerk

import "github.com/durst-group/durstworld/internal/game"

var rows = []string{
	"####################################",
	"#..=============....=============..#",
	"#..mmmmmmmm..............mmmmmmmm..#",
	"#..mmmmmmmm..............mmmmmmmm..#",
	"#..................................#",
	"#............o........o............#",
	"#..............rCCCCr...~~~~~~~~~..#",
	"#..............CRRRRC...~~~~~~~~~..#",
	"0..............CRRRRC............o.#",
	"#..............rCCCCr..............#",
	"#..................................#",
	"#..kkkkkk....o........o............#",
	"#..mmmmmmmm..............mmmmmmmm..#",
	"#..mmmmmmmm..............mmmmmmmm..#",
	"#..................................#",
	"####################################",
}

var legend = map[rune]game.LegendEntry{
	'0': {Kind: game.TilePortal, Ch: '◊', Walkable: true, Portal: "lobby", Label: "Lobby"},
	// Reactor core: a hot energy swirl, cold blue cycling to near-white.
	'R': {Kind: game.TileDecor, Ch: '◉', Anim: &game.TileAnim{
		Frames: []rune{'◉', '◎', '●', '◎'}, ColorA: "#2E6BFF", ColorB: "#EAFBFF", Speed: 1}},
	// Reactor casing: the steel shell around the core.
	'C': {Kind: game.TileDecor, Ch: '▒', Color: "#566372"},
	// Casing corners: bolted plates.
	'r': {Kind: game.TileDecor, Ch: '▚', Color: "#6B7480"},
	// Turbine banks hum: glyph pulses through block shades, color cycles cold→hot.
	'm': {Kind: game.TileDecor, Ch: '▓', Anim: &game.TileAnim{
		Frames: []rune{'▓', '▒', '░', '▒'}, ColorA: "#3A4654", ColorB: "#7DF0FF", Speed: 2}},
	// Control consoles: blinking panels, green/amber telemetry.
	'k': {Kind: game.TileDecor, Ch: '▦', Anim: &game.TileAnim{
		Frames: []rune{'▦', '▩', '▦', '▣'}, ColorA: "#7BD88F", ColorB: "#FFC861", Speed: 3}},
	// Steam pipes overhead.
	'=': {Kind: game.TileDecor, Ch: '═', Color: "#6B7480"},
	// Coolant channel: flowing water glyphs in blues.
	'~': {Kind: game.TileDecor, Ch: '~', Anim: &game.TileAnim{
		Frames: []rune{'~', '≈', '~', '≋'}, ColorA: "#2E6BFF", ColorB: "#56E1FF", Speed: 3}},
	// Catwalk lamps: warm flicker, the hall's working light.
	'o': {Kind: game.TileObject, Ch: '◉', Anim: &game.TileAnim{
		ColorA: "#FF8A4C", ColorB: "#FFC861", Speed: 2}},
}

func init() {
	game.RegisterFlavor(game.FlavorConfig{
		ID: "kraftwerk", Display: "Kraftwerk",
		Rows: rows, Legend: legend,
		SpawnX: 2, SpawnY: 8, Jitter: 0,
		Title: "⚡ Durst Kraftwerk",
		Body:  "Home of spin-offs and experiments.\n\nThe reactor's warming up — mind the coolant.",
		// The hall sits in shadow; the player's lamp reaches 11 tiles.
		Light: 11,
	})
}
