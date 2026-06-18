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
	// Climate surfaces for the overworld's richer biomes.
	TexSnow    // bright drifts with sparse sparkle
	TexSavanna // dry, sparse golden grass
	TexSwamp   // murky mud blotches with algae sheen
	// Indoor surfaces for the hand-built rooms.
	TexFloor // subtly speckled interior floor
	TexBrick // staggered brick courses, for walls
	TexMetal // riveted metal plate, for machine halls
	TexField // plowed farm furrows, for village fields
)

// TileProp is a sprite drawn over the ground in the HD renderer — flowers,
// trees, boulders — so decorations stop being solid squares.
type TileProp uint8

const (
	PropNone TileProp = iota
	PropFlower
	PropTuft
	PropTree
	PropBoulder
	PropBush
	PropRock
	PropStump
	PropHouse  // decorative multi-tile building
	PropPortal // animated area-entrance gate
	PropHat    // a wearable hat lying in the world (found, then equipped)
	PropSealed // a broken/dormant gate arch (a sealed portal, before repair)
	// Indoor furniture for the hand-built rooms.
	PropMachine  // boxy machine / plotter
	PropScreen   // wall-mounted display panel (animated)
	PropPlinth   // exhibit pedestal
	PropGem      // a found item lying in the world (mundane forage — no glow)
	PropGemGlow  // a luminous found item (crystal, mushroom) — glows at night
	PropLamp     // light fixture / spotlight (glows)
	PropCrate    // crate / desk block
	PropCore     // reactor energy orb (glows, hero feature)
	PropTurbine  // turbine / generator unit (glows)
	PropPipe     // pipe segment with a valve light
	PropFountain // water feature centerpiece (glows)
	// Signature overworld flora, one recognizable silhouette per biome.
	PropAcacia // savanna: flat-topped umbrella tree (tall, blocks)
	PropPalm   // beach: fronds on a leaning trunk (tall, blocks)
	PropFir    // snow: snow-tipped conifer (tall, blocks)
	PropReed     // swamp: a clump of thin cattail reeds (in-tile)
	PropCrag     // hill: a jagged rock spire (in-tile, blocks)
	PropCampfire // a traveler's campfire — flickers and casts warm light at night
	// Settlement structures for villages in the Wilds.
	PropWell      // a stone village well — the heart of a settlement (blocks)
	PropFenceH    // palisade rail running east–west (blocks)
	PropFenceV    // palisade rail running north–south (blocks)
	PropFencePost // palisade corner / junction post (blocks)
	// Multi-tile village buildings (drawn bottom-left-anchored, overhanging up
	// and right). PropBldBody marks a footprint tile that the anchor's sprite
	// covers — it blocks movement but draws nothing itself.
	PropBldCottage   // 1×1 cottage
	PropBldHouse     // 2×2 house
	PropBldLonghouse // 3×2 longhouse
	PropBldBarn      // 2×2 barn
	PropBldChurch    // 2×3 church with a steeple
	PropBldBody      // a covered footprint tile (no draw)
	PropCrop         // ripe grain standing in a field (harvestable)
	PropStone        // cut-stone rubble at a quarry (harvestable)
	PropLog          // a stack of logs at a lumber camp (harvestable)
	PropFish         // a fish by a jetty (harvestable)
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
	Color    string    // base color (hex); the glyph renderer draws Ch in this
	Tex      TileTex   // HD ground surface texture
	Ground   string    // HD ground base color (hex); falls back to Color if empty
	Prop     TileProp  // HD decoration sprite drawn over the ground
	PropHex  string    // HD prop color (hex); falls back to Color if empty
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
	// HD pixel-renderer data (ignored by the glyph renderer): a ground texture,
	// the ground base color, and an optional sprite prop drawn over it.
	Tex     TileTex
	Ground  string
	Prop    TileProp
	PropHex string
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

// baseLegend covers the runes every map shares. Walls and floors carry HD
// texture/ground data so the hand-built rooms render as pixel art (not flat
// blocks) in HD mode; the glyph renderer ignores those fields. An area can
// override these in its own legend (e.g. a metal or carpeted floor).
var baseLegend = map[rune]LegendEntry{
	'#': {Kind: TileWall, Ch: '█', Tex: TexBrick, Ground: "#3E4650"},
	'.': {Kind: TileFloor, Ch: '·', Walkable: true, Tex: TexFloor, Ground: "#2A2F37"},
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
				Tex:      le.Tex,
				Ground:   le.Ground,
				Prop:     le.Prop,
				PropHex:  le.PropHex,
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
