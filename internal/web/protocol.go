// Package web serves Durst World to the browser: a WebSocket session that
// drives the same game.Area objects the SSH clients do, and a top-down 3D
// client that renders the scene with WebGL.
//
// It is the third renderer (after the glyph TUI and the HD sixel client) and
// shares the live world with both — a browser player and an ssh player stand in
// the same Wilds, see each other move, and chat across the divide. Nothing about
// the world, the areas or persistence is browser-specific: this package only
// translates. The terminal clients rasterize the scene to pixels; this one
// serializes it to JSON and lets the GPU draw it.
//
// The wire format is built around one idea: tiles are addressed by *absolute
// world coordinates*, not by their position in the camera window. Walking pans
// the window over ground the client has usually already seen, so a frame that
// scrolls by a tile costs one row of tiles rather than a whole screen — and the
// 3D scene keeps its geometry instead of rebuilding it every step.
package web

// Protocol version. The client refuses to run against a server it doesn't
// recognize, so a stale cached page fails loudly instead of rendering nonsense.
const ProtocolVersion = 1

// Message types, server → client.
const (
	MsgHello = "hello" // once, on connect: who you are and the shape vocabulary
	MsgScene = "scene" // per frame: tile deltas, actors, lighting, prompts
	MsgChat  = "chat"  // one chat/system line for the log
	MsgPanel = "panel" // the contents of an open UI panel
	MsgBye   = "bye"   // the session is over (with a reason)
)

// Message types, client → server.
const (
	CmdKey    = "key"    // a game key press ("w", "shift+up", "e", "tab", …)
	CmdChat   = "chat"   // a submitted chat line (may be a /command)
	CmdPanel  = "panel"  // open/close/navigate a UI panel
	CmdResize = "resize" // the viewport changed; adjust the tile window
	CmdPing   = "ping"   // keepalive
)

// Hello is the first message: identity, the tile-window bounds the server will
// honor, and the prop → shape vocabulary the renderer builds meshes from.
// Sending the vocabulary (rather than hardcoding it in JavaScript) keeps the
// authoritative list in Go beside the props themselves, so the two can't drift.
type Hello struct {
	T       string           `json:"t"`
	Version int              `json:"version"`
	Name    string           `json:"name"`
	MaxW    int              `json:"maxW"`
	MaxH    int              `json:"maxH"`
	Shapes  map[string]Shape `json:"shapes"`
	Props   map[int]string   `json:"props"` // TileProp id → shape name
	Texes   map[int]string   `json:"texes"` // TileTex id → surface name
}

// Scene is one frame. Everything in it is either a delta against what the
// client already holds (tiles, palette) or small enough to resend outright
// (actors, lighting, the one-line prompts).
type Scene struct {
	T string `json:"t"`

	// Area identity. Reset clears the client's whole tile cache — the world
	// under it changed, so nothing carried over is valid.
	Area     string  `json:"area"`
	AreaName string  `json:"areaName"`
	Reset    bool    `json:"reset,omitempty"`
	Flare    float64 `json:"flare,omitempty"` // 1→0 title emphasis after entering

	// Camera window, in absolute world coordinates. The client centers its
	// follow-cam on the local player, not on this, but uses it to evict tiles.
	OX int `json:"ox"`
	OY int `json:"oy"`
	W  int `json:"w"`
	H  int `json:"h"`

	// PalAdd extends the session's append-only color table; a color is sent
	// once and referenced by index forever after. Ground palettes are small and
	// repeat heavily, so after the first few frames this is empty.
	PalAdd []string `json:"palAdd,omitempty"`

	// Tiles is a flat run of TileStride ints per tile (see EncodeTile). Only
	// tiles that are new to the client or have changed appear.
	Tiles []int `json:"tiles,omitempty"`
	// Drop lists absolute x,y pairs the client should forget (they fell out of
	// the retained radius), so a long walk doesn't grow the scene without bound.
	Drop []int `json:"drop,omitempty"`

	Players   []Actor   `json:"players"`
	Creatures []Actor   `json:"creatures"`
	Light     *Light    `json:"light,omitempty"`
	Ambient   *Ambient  `json:"ambient,omitempty"`
	Labels    []Label   `json:"labels,omitempty"`
	Minimap   *Minimap  `json:"minimap,omitempty"`
	Build     *Build    `json:"build,omitempty"`
	Slide     *Slide    `json:"slide,omitempty"`
	Overlay   []float64 `json:"overlay,omitempty"` // raycaster columns (Doom)

	Prompt string `json:"prompt,omitempty"`
	Toast  string `json:"toast,omitempty"`
	Claim  string `json:"claim,omitempty"`
	Hurt   bool   `json:"hurt,omitempty"`
	Frame  int    `json:"frame"`
}

// TileStride is how many ints EncodeTile writes per tile.
const TileStride = 8

// Tile field offsets within a stride, mirrored by the client's decoder.
const (
	TileX = iota
	TileY
	TileKind
	TileTex
	TileGround // palette index
	TileProp
	TilePropColor // palette index
	TileFlags
)

// Tile flag bits.
const (
	FlagWalkable = 1 << iota
	FlagPortal
	FlagAnimated
)

// Actor is a player or a creature on the grid. Coordinates are absolute; the
// client interpolates between frames so movement glides instead of snapping.
type Actor struct {
	Name   string `json:"n,omitempty"`
	Kind   string `json:"k,omitempty"` // creature species; "" for players
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Color  int    `json:"c"`           // palette index
	Facing int    `json:"f"`           // world.Dir
	Style  int    `json:"s,omitempty"` // avatar body style
	Access int    `json:"a,omitempty"` // worn accessory index
	Weapon string `json:"w,omitempty"`
	HP     int    `json:"hp,omitempty"`
	MaxHP  int    `json:"mhp,omitempty"`
	Downed bool   `json:"down,omitempty"`
	Self   bool   `json:"me,omitempty"`
	Owner  string `json:"o,omitempty"` // a tamed creature's player
}

// Light is an area's radial light — the Wilds' discovery circle, a cave's
// lantern. The client turns it into real falloff and fog rather than the
// terminal's per-tile dimming.
type Light struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Radius  int     `json:"r"`
	Warm    bool    `json:"warm,omitempty"`
	Sunless bool    `json:"sunless,omitempty"`
	Floor   float64 `json:"floor,omitempty"`
}

// Ambient is the scene's sky tint and strength — the live day/night wash the
// terminal clients apply per pixel, here driving the sun and ambient terms.
type Ambient struct {
	Hex      string  `json:"hex"`
	Strength float64 `json:"strength"`
	// Night is Strength normalized to 0 (full daylight) … 1 (deep night).
	//
	// Strength on its own is not enough to light a 3D scene: it is "how hard to
	// wash tiles toward this tint", and its top end is whatever the day cycle's
	// darkest entry happens to be — a number only the server's own table knows.
	// A client that guessed at the range lit deep night like a bright afternoon.
	// So the server, which owns the table, does the normalizing.
	Night float64 `json:"night"`
}

// Label is floating world-space text: a portal's destination name, a claim
// marker. Drawn as a sprite that always faces the camera.
type Label struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Text string `json:"t"`
	Kind string `json:"k,omitempty"` // "portal" | "sign"
}

// Minimap is the coarse overview ('m'), one hex per cell, already resolved by
// the area. Empty Hex means unexplored.
type Minimap struct {
	Title string     `json:"title"`
	Rows  [][]string `json:"rows"`
	SelfX int        `json:"sx"`
	SelfY int        `json:"sy"`
}

// Build is the build-mode palette state: which placeable is selected, the
// contextual footer for the ghost cell, and whether that footer is a warning.
type Build struct {
	Sel    int      `json:"sel"`
	Footer string   `json:"footer,omitempty"`
	Warn   bool     `json:"warn,omitempty"`
	Items  []string `json:"items,omitempty"`
}

// Slide is the presentation deck's current slide — markdown source the client
// renders into a DOM panel (rather than the bitmap font HD has to use).
type Slide struct {
	Source string `json:"src"`
	Footer string `json:"footer,omitempty"`
}

// ChatMsg is one line for the browser's chat log.
type ChatMsg struct {
	T    string `json:"t"`
	Text string `json:"text"`
	Hex  string `json:"hex,omitempty"`
	Kind string `json:"kind,omitempty"` // "say" | "system" | "whisper"
}

// PanelMsg carries the contents of an open UI panel. The browser renders these
// as HTML, so the server ships data and never a rasterized layout — this is the
// one place the browser client is genuinely simpler than HD, which has to draw
// every panel into the pixel frame by hand.
type PanelMsg struct {
	T      string      `json:"t"`
	Panel  string      `json:"panel"` // "" closes whatever is open
	Title  string      `json:"title,omitempty"`
	Rows   []PanelRow  `json:"rows,omitempty"`
	Sel    int         `json:"sel,omitempty"`
	Footer string      `json:"footer,omitempty"`
	Extra  interface{} `json:"extra,omitempty"`
}

// PanelRow is one line in a panel: a label, an optional value, and flags for
// how to style it (dim for undiscovered compendium entries, warn for a recipe
// you can't afford).
type PanelRow struct {
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
	Desc  string `json:"desc,omitempty"`
	Hex   string `json:"hex,omitempty"`
	Dim   bool   `json:"dim,omitempty"`
	Warn  bool   `json:"warn,omitempty"`
	Sel   bool   `json:"sel,omitempty"`
}

// Bye ends a session with a human-readable reason.
type Bye struct {
	T      string `json:"t"`
	Reason string `json:"reason"`
}

// ClientMsg is anything the browser sends. One struct covers every command;
// the fields not relevant to a given T are simply absent.
type ClientMsg struct {
	T     string `json:"t"`
	Key   string `json:"key,omitempty"`
	Text  string `json:"text,omitempty"`
	Panel string `json:"panel,omitempty"`
	Act   string `json:"act,omitempty"` // the panel action: use, add, sub, ready, …
	Sel   int    `json:"sel,omitempty"`
	W     int    `json:"w,omitempty"`
	H     int    `json:"h,omitempty"`
}
