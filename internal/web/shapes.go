package web

import "github.com/durst-group/durstworld/internal/game"

// The 3D shape vocabulary.
//
// The terminal clients draw a prop by stamping hand-authored pixel art. A 3D
// client can't use that art, but it doesn't need to: every prop already carries
// a *meaning* (a tree, a forge, a cathedral) and a color, and that's enough to
// build geometry from. This table is the translation — each of the game's props
// mapped to a builder the renderer knows, a footprint, a height and how much it
// glows at night.
//
// Keeping it in Go, next to the props themselves, is deliberate: the vocabulary
// ships to the browser in the hello message, so the client never hardcodes a
// prop id. A prop added later and not listed here still renders — as a plain
// marker box in the tile's own color — rather than vanishing or breaking the
// frame. That's the failure mode we want: visibly unfinished, never broken.

// Shape is how one prop becomes geometry. Build selects the client's mesh
// builder; Style is the variant within it. Sizes are in tiles, so a 2×3
// building is exactly two tiles wide and three deep whatever the zoom.
type Shape struct {
	Build string  `json:"build"`
	Style string  `json:"style,omitempty"`
	W     float64 `json:"w"`              // footprint width, in tiles
	D     float64 `json:"d"`              // footprint depth, in tiles
	H     float64 `json:"h"`              // height, in tiles
	Glow  float64 `json:"glow,omitempty"` // emissive strength at night (0–1)
	Sway  float64 `json:"sway,omitempty"` // wind animation amount (0–1)
	// Jitter is how much this prop may vary between instances: the client spins
	// it to a random facing and scales it by up to ±Jitter, seeded from the
	// tile's own coordinates so it stays identical for every player and across
	// reconnects.
	//
	// It exists because identical, grid-aligned copies are what make a
	// generated world look generated. A forest of byte-identical trees all
	// facing the same way reads as tiling instantly; the same trees turned and
	// resized a little read as a forest. Only things that *grew* get this —
	// a cathedral squared to the street is not a mistake to be corrected.
	Jitter float64 `json:"jitter,omitempty"`
}

// The builders the client implements. Everything below composes from these
// eleven; the Style field picks the variant inside each one.
const (
	BuildTree     = "tree"     // trunk + canopy (round, conifer, palm, acacia, stump)
	BuildClump    = "clump"    // ground growth (grass, flowers, reeds, crops)
	BuildRock     = "rock"     // irregular stone (rock, boulder, spire, column)
	BuildBox      = "box"      // furniture and machinery
	BuildBuilding = "building" // walls + pitched roof, sized by footprint
	BuildFence    = "fence"    // rails, posts, curtain walls, towers
	BuildFlat     = "flat"     // something lying on the ground (bridges, pools)
	BuildGlow     = "glow"     // a light source or luminous find
	BuildPortal   = "portal"   // an area entrance (gate, sealed arch, cave mouth)
	BuildItem     = "item"     // a small pickup resting on the tile
	BuildCreature = "creature" // a living body
	BuildChess    = "chess"    // an arcade chess piece
)

// propShapes maps every prop the game defines to its geometry. Ordered to
// mirror the const block in internal/game/tilemap.go so the two read as a pair.
var propShapes = map[game.TileProp]Shape{
	// Overworld flora. Trees sway; ground cover sways more.
	game.PropFlower: {BuildClump, "flower", 0.5, 0.5, 0.40, 0, 0.6, 0.22},
	game.PropTuft:   {BuildClump, "tuft", 0.5, 0.5, 0.45, 0, 0.7, 0.22},
	game.PropTree:   {BuildTree, "round", 1.0, 1.0, 2.2, 0, 0.25, 0.14},
	game.PropBush:   {BuildClump, "bush", 0.8, 0.8, 0.5, 0, 0.35, 0.22},
	game.PropStump:  {BuildTree, "stump", 0.6, 0.6, 0.35, 0, 0, 0.14},
	game.PropAcacia: {BuildTree, "acacia", 1.4, 1.4, 2.4, 0, 0.2, 0.14},
	game.PropPalm:   {BuildTree, "palm", 1.2, 1.2, 2.8, 0, 0.4, 0.14},
	game.PropFir:    {BuildTree, "conifer", 1.0, 1.0, 2.8, 0, 0.15, 0.14},
	game.PropReed:   {BuildClump, "reed", 0.7, 0.7, 0.9, 0, 0.8, 0.22},
	game.PropCrop:   {BuildClump, "crop", 0.8, 0.8, 0.7, 0, 0.5, 0.22},

	// Stone.
	game.PropBoulder:    {BuildRock, "boulder", 0.9, 0.9, 0.8, 0, 0, 0.18},
	game.PropRock:       {BuildRock, "rock", 0.6, 0.6, 0.35, 0, 0, 0.18},
	game.PropCrag:       {BuildRock, "spire", 0.9, 0.9, 1.6, 0, 0, 0.18},
	game.PropStone:      {BuildRock, "rubble", 0.8, 0.8, 0.4, 0, 0, 0.18},
	game.PropStalagmite: {BuildRock, "spire", 0.5, 0.5, 0.9, 0, 0, 0.18},
	game.PropColumn:     {BuildRock, "column", 0.8, 0.8, 3.0, 0, 0, 0.18},
	game.PropFlowstone:  {BuildRock, "flowstone", 0.9, 0.9, 1.1, 0, 0, 0.18},
	game.PropTimbering:  {BuildFence, "timber", 1.0, 1.0, 1.6, 0, 0, 0},

	// Settlement structures. Footprints match the multi-tile sprites, drawn
	// bottom-left-anchored the way the tileset anchors them.
	game.PropWell:      {BuildBox, "well", 0.9, 0.9, 0.8, 0, 0, 0},
	game.PropFenceH:    {BuildFence, "h", 1.0, 0.2, 0.6, 0, 0, 0},
	game.PropFenceV:    {BuildFence, "v", 0.2, 1.0, 0.6, 0, 0, 0},
	game.PropFencePost: {BuildFence, "post", 0.25, 0.25, 0.8, 0, 0, 0},
	game.PropStoneWall: {BuildFence, "wall", 1.0, 1.0, 1.4, 0, 0, 0},
	game.PropTower:     {BuildFence, "tower", 1.0, 1.0, 2.6, 0, 0, 0},
	game.PropBrazier:   {BuildGlow, "fire", 0.4, 0.4, 1.0, 0.9, 0, 0.12},
	game.PropStall:     {BuildBox, "stall", 1.0, 1.0, 1.0, 0, 0, 0},

	game.PropHouse:          {BuildBuilding, "house", 1.0, 1.0, 1.6, 0, 0, 0},
	game.PropBldCottage:     {BuildBuilding, "cottage", 1.0, 1.0, 1.5, 0.15, 0, 0},
	game.PropBldHouse:       {BuildBuilding, "house", 2.0, 2.0, 2.0, 0.15, 0, 0},
	game.PropBldLonghouse:   {BuildBuilding, "longhouse", 3.0, 2.0, 1.9, 0.15, 0, 0},
	game.PropBldBarn:        {BuildBuilding, "barn", 2.0, 2.0, 2.2, 0, 0, 0},
	game.PropBldChurch:      {BuildBuilding, "church", 2.0, 3.0, 3.4, 0.1, 0, 0},
	game.PropBldKeep:        {BuildBuilding, "keep", 3.0, 3.0, 3.6, 0.1, 0, 0},
	game.PropBldCathedral:   {BuildBuilding, "cathedral", 3.0, 4.0, 4.6, 0.1, 0, 0},
	game.PropBldTownhouse:   {BuildBuilding, "townhouse", 2.0, 3.0, 3.0, 0.2, 0, 0},
	game.PropBldMarketHall:  {BuildBuilding, "markethall", 3.0, 3.0, 2.4, 0.15, 0, 0},
	game.PropBldSmithy:      {BuildBuilding, "smithy", 2.0, 2.0, 1.9, 0.55, 0, 0},
	game.PropBldTavern:      {BuildBuilding, "tavern", 2.0, 2.0, 2.1, 0.5, 0, 0},
	game.PropBldRowhouse:    {BuildBuilding, "rowhouse", 2.0, 3.0, 2.6, 0.2, 0, 0},
	game.PropBldNarrowhouse: {BuildBuilding, "narrowhouse", 1.0, 2.0, 2.4, 0.2, 0, 0},
	game.PropBldDeephouse:   {BuildBuilding, "deephouse", 2.0, 4.0, 3.0, 0.2, 0, 0},
	game.PropMill:           {BuildBuilding, "mill", 2.0, 2.0, 3.0, 0, 0, 0},
	game.PropNoticeBoard:    {BuildBox, "sign", 0.9, 0.25, 0.9, 0, 0, 0},
	game.PropBldBody:        {}, // a covered footprint tile: the anchor draws it

	// Bridges and ground coverings — walkable, so they lie flat.
	game.PropBridgeH: {BuildFlat, "bridge-h", 1.0, 1.0, 0.08, 0, 0, 0},
	game.PropBridgeV: {BuildFlat, "bridge-v", 1.0, 1.0, 0.08, 0, 0, 0},
	game.PropBedroll: {BuildFlat, "bedroll", 0.8, 0.6, 0.1, 0, 0, 0},
	game.PropChasm:   {BuildFlat, "chasm", 1.0, 1.0, 0.05, 0, 0, 0},

	// Indoor furniture and machinery.
	game.PropMachine:   {BuildBox, "machine", 0.9, 0.9, 1.1, 0.2, 0, 0},
	game.PropScreen:    {BuildBox, "screen", 1.0, 0.2, 0.9, 0.65, 0, 0},
	game.PropPlinth:    {BuildBox, "plinth", 0.7, 0.7, 0.7, 0, 0, 0},
	game.PropCrate:     {BuildBox, "crate", 0.8, 0.8, 0.7, 0, 0, 0},
	game.PropPipe:      {BuildBox, "pipe", 1.0, 0.4, 0.5, 0.3, 0, 0},
	game.PropTurbine:   {BuildBox, "turbine", 0.9, 0.9, 1.3, 0.5, 0, 0},
	game.PropWorkbench: {BuildBox, "workbench", 0.9, 0.9, 0.8, 0, 0, 0},
	game.PropSawmill:   {BuildBox, "sawmill", 1.0, 1.0, 1.1, 0.25, 0, 0},
	game.PropFurnace:   {BuildBox, "furnace", 1.0, 1.0, 1.3, 0.75, 0, 0},
	game.PropChest:     {BuildBox, "chest", 0.7, 0.5, 0.5, 0, 0, 0},
	game.PropLog:       {BuildBox, "logs", 0.9, 0.7, 0.5, 0, 0, 0},

	// Light sources and luminous finds.
	game.PropLamp:       {BuildGlow, "lamp", 0.3, 0.3, 1.4, 0.85, 0, 0},
	game.PropCore:       {BuildGlow, "orb", 1.0, 1.0, 1.4, 1.0, 0, 0},
	game.PropFountain:   {BuildGlow, "fountain", 1.0, 1.0, 0.8, 0.45, 0, 0},
	game.PropCampfire:   {BuildGlow, "fire", 0.6, 0.6, 0.5, 0.9, 0, 0.12},
	game.PropGemGlow:    {BuildGlow, "gem", 0.4, 0.4, 0.4, 0.7, 0, 0.12},
	game.PropCaveShroom: {BuildGlow, "shroom", 0.6, 0.6, 0.5, 0.8, 0, 0.12},
	game.PropGlowPool:   {BuildFlat, "pool", 1.0, 1.0, 0.06, 0.6, 0, 0},
	game.PropLightShaft: {BuildGlow, "shaft", 0.9, 0.9, 3.0, 0.5, 0, 0},
	game.PropRelic:      {BuildGlow, "relic", 0.5, 0.5, 0.5, 0.6, 0, 0.12},
	game.PropGeode:      {BuildGlow, "geode", 0.6, 0.6, 0.5, 0.8, 0, 0.12},

	// Small pickups resting on the ground.
	game.PropGem:  {BuildItem, "gem", 0.35, 0.35, 0.35, 0.15, 0, 0.15},
	game.PropHat:  {BuildItem, "hat", 0.5, 0.5, 0.3, 0.3, 0, 0.15},
	game.PropFish: {BuildItem, "fish", 0.5, 0.5, 0.2, 0, 0, 0.15},

	// Area entrances.
	game.PropPortal:    {BuildPortal, "gate", 1.0, 1.0, 2.0, 0.9, 0, 0},
	game.PropSealed:    {BuildPortal, "sealed", 1.0, 1.0, 1.8, 0.1, 0, 0},
	game.PropCaveMouth: {BuildPortal, "cave", 1.0, 1.0, 1.5, 0, 0, 0},

	// Wildlife baked into a tile (live animals arrive as actors instead).
	game.PropRabbit:   {BuildCreature, "rabbit", 0.5, 0.5, 0.45, 0, 0, 0.1},
	game.PropDeer:     {BuildCreature, "deer", 0.8, 0.8, 1.2, 0, 0, 0.1},
	game.PropFox:      {BuildCreature, "fox", 0.7, 0.7, 0.5, 0, 0, 0.1},
	game.PropBird:     {BuildCreature, "bird", 0.4, 0.4, 0.35, 0, 0, 0.1},
	game.PropFishWild: {BuildCreature, "fish", 0.5, 0.5, 0.25, 0, 0, 0.1},

	// The arcade's chess cabinet.
	game.PropChessPawn:   {BuildChess, "pawn", 0.6, 0.6, 0.5, 0, 0, 0},
	game.PropChessKnight: {BuildChess, "knight", 0.6, 0.6, 0.75, 0, 0, 0},
	game.PropChessBishop: {BuildChess, "bishop", 0.6, 0.6, 0.8, 0, 0, 0},
	game.PropChessRook:   {BuildChess, "rook", 0.6, 0.6, 0.7, 0, 0, 0},
	game.PropChessQueen:  {BuildChess, "queen", 0.6, 0.6, 0.9, 0, 0, 0},
	game.PropChessKing:   {BuildChess, "king", 0.6, 0.6, 1.0, 0, 0, 0},
}

// shapeNames is the prop id → shape-key table sent to the client. The client
// looks a prop up here, then looks the key up in the Shapes map — one
// indirection that lets several props share one piece of geometry.
func shapeNames() (map[int]string, map[string]Shape) {
	ids := make(map[int]string, len(propShapes))
	shapes := make(map[string]Shape, len(propShapes))
	for prop, sh := range propShapes {
		if sh.Build == "" {
			continue // PropBldBody and friends: deliberately no geometry
		}
		key := sh.Build + ":" + sh.Style
		ids[int(prop)] = key
		shapes[key] = sh
	}
	return ids, shapes
}

// propShapeKey resolves a single prop to its shape key, for the places that
// need one outside the tile stream — a live creature's body, for instance.
func propShapeKey(prop game.TileProp) (string, bool) {
	sh, ok := propShapes[prop]
	if !ok || sh.Build == "" {
		return "", false
	}
	return sh.Build + ":" + sh.Style, true
}

// texNames maps the ground textures to surface names the client uses to pick a
// material finish — how rough the ground is, whether it ripples like water,
// whether it sparkles like snow. The color still comes from the tile itself, so
// a biome recolor needs no change here.
var texNames = map[game.TileTex]string{
	game.TexFlat:    "flat",
	game.TexGrass:   "grass",
	game.TexSand:    "sand",
	game.TexWater:   "water",
	game.TexDirt:    "dirt",
	game.TexForest:  "forest",
	game.TexRock:    "rock",
	game.TexSnow:    "snow",
	game.TexSavanna: "savanna",
	game.TexSwamp:   "swamp",
	game.TexFloor:   "floor",
	game.TexBrick:   "brick",
	game.TexMetal:   "metal",
	game.TexField:   "field",
}

func texIDs() map[int]string {
	out := make(map[int]string, len(texNames))
	for tex, name := range texNames {
		out[int(tex)] = name
	}
	return out
}
