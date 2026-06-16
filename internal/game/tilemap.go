package game

// Tilemap: maps are hand-crafted string slices using simple source runes
// ('#' wall, '.' floor, digits for portals, …) plus a per-area legend for
// anything special. Text labels (signs, name plates) are overlaid at render
// time so map letters never clash with legend runes.

type TileKind int

const (
	TileVoid TileKind = iota
	TileFloor
	TileWall
	TileDecor  // impassable decoration
	TilePortal // walking onto it transitions
	TileObject // interactable (guestbook, presenter spot)
)

// TileTex selects a pixel-art surface texture for the HD renderer (clean,
// biome-specific accent pixels over the tile's base color). TexFlat is a plain
// solid tile — the default for indoor/hand-built maps.
type TileTex uint8

const (
	TexFlat TileTex = iota
	TexGrass
	TexSand
	TexWater
	TexDirt
	TexForest
	TexRock
)

// Tile is one cell of a parsed map.
type Tile struct {
	Kind     TileKind
	Ch       rune // display rune
	Walkable bool
	Portal   string    // destination area id, for TilePortal
	Label    string    // portal display name, for status-bar hints
	Object   string    // object id, for TileObject
	Anim     *TileAnim // optional animation
	Color    string    // optional base color (hex); overrides the kind palette
	Tex      TileTex   // HD surface texture
}

// TileAnim makes a tile come alive: its glyph cycles through Frames and its
// color shimmers between ColorA and ColorB (hex), both advanced by the global
// animation frame. Empty Frames keeps the tile's base glyph; empty colors
// keep the kind's base color. Speed is ticks per step (≥1; 0 means 1).
type TileAnim struct {
	Frames []rune
	ColorA string
	ColorB string
	Speed  int
}

// LegendEntry describes how a source rune becomes a tile.
type LegendEntry struct {
	Kind     TileKind
	Ch       rune // display rune; 0 = same as source
	Walkable bool
	Portal   string
	Label    string
	Object   string
	Anim     *TileAnim
	Color    string
}

// MapText is a label drawn over the map (display only; the tiles underneath
// keep their semantics).
type MapText struct {
	X, Y int
	S    string
}

// TileMap is a parsed, ready-to-render map.
type TileMap struct {
	W, H  int
	Tiles [][]Tile
	Texts []MapText
}

// baseLegend covers the runes every map shares.
var baseLegend = map[rune]LegendEntry{
	'#': {Kind: TileWall, Ch: '█'},
	'.': {Kind: TileFloor, Ch: '·', Walkable: true},
	' ': {Kind: TileVoid, Ch: ' '},
}

// ParseMap turns rows + legend into a TileMap. Short rows are padded with
// void. Runes found in neither legend become impassable decor rendered
// as-is.
func ParseMap(rows []string, legend map[rune]LegendEntry, texts []MapText) *TileMap {
	w := 0
	for _, r := range rows {
		if n := len([]rune(r)); n > w {
			w = n
		}
	}
	tm := &TileMap{W: w, H: len(rows), Texts: texts}
	for _, row := range rows {
		runes := []rune(row)
		line := make([]Tile, w)
		for x := 0; x < w; x++ {
			src := ' '
			if x < len(runes) {
				src = runes[x]
			}
			le, ok := legend[src]
			if !ok {
				le, ok = baseLegend[src]
			}
			if !ok {
				le = LegendEntry{Kind: TileDecor, Ch: src}
			}
			ch := le.Ch
			if ch == 0 {
				ch = src
			}
			line[x] = Tile{
				Kind:     le.Kind,
				Ch:       ch,
				Walkable: le.Walkable,
				Portal:   le.Portal,
				Label:    le.Label,
				Object:   le.Object,
				Anim:     le.Anim,
				Color:    le.Color,
			}
		}
		tm.Tiles = append(tm.Tiles, line)
	}
	return tm
}

// At returns the tile at x,y; out-of-bounds is void.
func (tm *TileMap) At(x, y int) Tile {
	if y < 0 || y >= tm.H || x < 0 || x >= tm.W {
		return Tile{Kind: TileVoid, Ch: ' '}
	}
	return tm.Tiles[y][x]
}

// Walkable reports whether a player may stand on x,y.
func (tm *TileMap) Walkable(x, y int) bool {
	return tm.At(x, y).Walkable
}

// FindTiles returns the coordinates of every tile whose source matched the
// given object id (e.g. all presenter spots).
func (tm *TileMap) FindObject(object string) (xs, ys []int) {
	for y := 0; y < tm.H; y++ {
		for x := 0; x < tm.W; x++ {
			if tm.Tiles[y][x].Object == object {
				xs = append(xs, x)
				ys = append(ys, y)
			}
		}
	}
	return
}

// NearObject reports whether x,y is on or 4-adjacent to a tile with the
// given object id.
func (tm *TileMap) NearObject(x, y int, object string) bool {
	for _, d := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if tm.At(x+d[0], y+d[1]).Object == object {
			return true
		}
	}
	return false
}

// PortalNear returns the portal tile on or 4-adjacent to x,y, if any.
func (tm *TileMap) PortalNear(x, y int) (Tile, bool) {
	for _, d := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		t := tm.At(x+d[0], y+d[1])
		if t.Kind == TilePortal {
			return t, true
		}
	}
	return Tile{}, false
}
