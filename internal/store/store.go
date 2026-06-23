// Package store is the memory between visits. Live game state never lives
// here — only the visitor log, the guestbook and an append-only event log.
// The game must stay fully playable when the DB is unavailable, so Open
// falls back to a no-op store and every write failure is just logged.
package store

import (
	"log"
	"time"
)

// VisitInfo powers the welcome-back line.
type VisitInfo struct {
	VisitCount int
	LastSeen   time.Time // zero on first visit
	FirstVisit bool
}

// GuestbookEntry is one signed line in the lobby guestbook.
type GuestbookEntry struct {
	Name      string
	Message   string
	CreatedAt time.Time
}

// Placement is one player-placed structure in the shared world: a kind (a
// game.Placeable id) at an absolute (x,y), with its owner and when it was built.
type Placement struct {
	X, Y    int
	Kind    string // placeable id (workbench, fence, chest, …)
	Owner   string // the SSH username that placed it
	State   string // opaque JSON (machine buffers + wall-clock); "" for static props
	Created int64  // unix seconds
}

// Claim is one player's deed over a settlement plot (docs/CLAIMS_PLAN.md): the
// worldgen plot id, the holder, the parcel's bounding box, and the wall-clock of
// the owner's last presence (which drives lapse).
type Claim struct {
	PlotID                 string
	Owner                  string
	MinX, MinY, MaxX, MaxY int
	LastTouch              int64 // unix seconds
}

// Cleared is one terrain cell a player has cleared with a tool (a felled tree or
// broken boulder — docs/BUILD_TOOLS_PLAN.md): the cell, who cleared it, and the
// wall-clock of the last touch (which drives regrowth when the owner is absent).
type Cleared struct {
	X, Y      int
	Owner     string
	LastTouch int64 // unix seconds
}

// Project is the persisted state of one community build (docs/COMMUNITY_PLAN.md):
// the build id, the phase currently under construction, the resources banked
// toward that phase, and whether the whole build is finished. Shared world
// state, like a co-op gate's pool — saved on every accepted contribution.
type Project struct {
	ID    string
	Phase int
	Pool  map[string]int // resource id -> amount banked toward the current phase
	Done  bool
}

// DeckRecord is a persisted presentation deck (owned by a user).
type DeckRecord struct {
	ID      string
	Owner   string
	Title   string
	Source  string
	Created int64 // unix seconds
}

// Store is the only door to persistence. Writes never fail the game.
type Store interface {
	// RecordVisit upserts the player row and returns the (already
	// incremented) visit info.
	RecordVisit(name string) VisitInfo
	// RecordDisconnect refreshes last_seen on the way out.
	RecordDisconnect(name string)
	// RecordAreaVisit appends area to the player's areas_visited set.
	RecordAreaVisit(name, area string)
	// LogEvent appends to the events table (joins, transitions).
	LogEvent(name, typ, detail string)
	// SignGuestbook stores one guestbook line.
	SignGuestbook(name, message string) error
	// GuestbookEntries returns the latest n entries, newest first.
	GuestbookEntries(n int) []GuestbookEntry
	// SaveAvatar persists a player's avatar customization.
	SaveAvatar(name, color string, style, accessory int)
	// LoadAvatar returns a player's saved avatar; ok is false if none stored.
	LoadAvatar(name string) (color string, style, accessory int, ok bool)
	// SaveDeck upserts a user-owned presentation deck.
	SaveDeck(id, owner, title, source string, createdUnix int64)
	// LoadDecks returns every persisted deck, oldest first.
	LoadDecks() []DeckRecord
	// DeleteDeck removes a persisted deck by id.
	DeleteDeck(id string)
	// SavePosition upserts where a player stands in an area (e.g. the Wilds).
	SavePosition(name, area string, x, y int)
	// LoadPosition returns a player's saved position in an area; ok is false if none.
	LoadPosition(name, area string) (x, y int, ok bool)
	// SaveDiscovery upserts one 8×8 fog-of-war chunk as a 64-bit cell mask.
	SaveDiscovery(name string, cx, cy int, mask uint64)
	// LoadDiscovery returns a player's discovered chunks keyed by chunk coord.
	LoadDiscovery(name string) map[[2]int]uint64
	// SaveCaveDiscovery upserts one fog-of-war chunk for a single cave (namespaced
	// by the cave's id, its overworld entrance), so caves are remembered apart.
	SaveCaveDiscovery(name, cave string, cx, cy int, mask uint64)
	// LoadCaveDiscovery returns a player's discovered chunks of one cave.
	LoadCaveDiscovery(name, cave string) map[[2]int]uint64
	// AddItem increments a player's count of one inventory item by one.
	AddItem(name, item string)
	// SpendItem decrements a player's count of one item by one (floor 0).
	SpendItem(name, item string)
	// LoadInventory returns a player's item counts (id → count), never nil.
	LoadInventory(name string) map[string]int
	// MarkCollected records that a player has picked up the item at (x,y).
	MarkCollected(name string, x, y int)
	// LoadCollected returns the world cells a player has already harvested.
	LoadCollected(name string) map[[2]int]bool
	// UnlockHat records that a player has found an accessory (by its index).
	UnlockHat(name string, hat int)
	// LoadHats returns the accessory indices a player owns.
	LoadHats(name string) map[int]bool
	// MarkSpecies records that a player has observed a wildlife species.
	MarkSpecies(name, kind string)
	// LoadCompendium returns the wildlife species a player has observed.
	LoadCompendium(name string) map[string]bool
	// SaveCompanion records (or replaces) a player's tamed companion species.
	SaveCompanion(name, kind string)
	// LoadCompanion returns a player's tamed companion species; ok is false if none.
	LoadCompanion(name string) (kind string, ok bool)
	// FixPersonalGate records that a player has repaired a personal gate.
	FixPersonalGate(name, gate string)
	// LoadPersonalGates returns the personal gates a player has repaired.
	LoadPersonalGates(name string) map[string]bool
	// SaveGateWorld upserts a co-op gate's shared pool count and fixed flag.
	SaveGateWorld(gate string, pool int, fixed bool)
	// LoadGateWorld returns the shared co-op gate pools and fixed flags.
	LoadGateWorld() (pools map[string]int, fixed map[string]bool)
	// AddPlacement upserts a player-placed structure at (x,y) (the shared
	// placements layer overlaid on the deterministic Wilds).
	AddPlacement(p Placement)
	// RemovePlacement deletes whatever is placed at (x,y).
	RemovePlacement(x, y int)
	// LoadPlacements returns every placement in the world (a small, shared set).
	LoadPlacements() []Placement
	// SaveClaim upserts a player's land claim, keyed by plot id (the shared
	// claims layer — see docs/CLAIMS_PLAN.md).
	SaveClaim(c Claim)
	// RemoveClaim deletes the claim on a plot (a release or lapse).
	RemoveClaim(plotID string)
	// LoadClaims returns every land claim in the world (a small, shared set).
	LoadClaims() []Claim
	// SaveCleared upserts a cleared terrain cell (the regrowable clearing overlay).
	SaveCleared(c Cleared)
	// RemoveCleared deletes a cleared cell (a regrowth or undo).
	RemoveCleared(x, y int)
	// LoadCleared returns every cleared cell in the world.
	LoadCleared() []Cleared
	// SaveArtifact records a discovered unique weapon and its discoverer (the
	// shared legends registry — see docs/WEAPON_PLAN.md).
	SaveArtifact(id, owner string)
	// LoadArtifacts returns every claimed artifact id mapped to its discoverer.
	LoadArtifacts() map[string]string
	// SaveProject upserts a community build's shared state — its current phase,
	// the resources banked toward it, and its done flag (docs/COMMUNITY_PLAN.md).
	SaveProject(p Project)
	// LoadProjects returns every community build's saved state.
	LoadProjects() []Project
	Close() error
}

// Open tries SQLite at path; on any failure it logs a warning and returns
// a no-op store so the game keeps running.
func Open(path string) Store {
	s, err := openSQLite(path)
	if err != nil {
		log.Printf("store: WARNING: persistence disabled (%v) — playing without memory", err)
		return noopStore{}
	}
	return s
}

type noopStore struct{}

func (noopStore) RecordVisit(name string) VisitInfo {
	return VisitInfo{VisitCount: 1, FirstVisit: true}
}
func (noopStore) RecordDisconnect(string)               {}
func (noopStore) RecordAreaVisit(string, string)        {}
func (noopStore) LogEvent(string, string, string)       {}
func (noopStore) SignGuestbook(string, string) error    { return nil }
func (noopStore) GuestbookEntries(int) []GuestbookEntry { return nil }
func (noopStore) SaveAvatar(string, string, int, int)   {}
func (noopStore) LoadAvatar(string) (string, int, int, bool) {
	return "", 0, 0, false
}
func (noopStore) SaveDeck(string, string, string, string, int64)     {}
func (noopStore) LoadDecks() []DeckRecord                            { return nil }
func (noopStore) DeleteDeck(string)                                  {}
func (noopStore) SavePosition(string, string, int, int)              {}
func (noopStore) LoadPosition(string, string) (int, int, bool)       { return 0, 0, false }
func (noopStore) SaveDiscovery(string, int, int, uint64)             {}
func (noopStore) LoadDiscovery(string) map[[2]int]uint64             { return nil }
func (noopStore) SaveCaveDiscovery(string, string, int, int, uint64) {}
func (noopStore) LoadCaveDiscovery(string, string) map[[2]int]uint64 { return nil }
func (noopStore) AddItem(string, string)                             {}
func (noopStore) SpendItem(string, string)                           {}
func (noopStore) LoadInventory(string) map[string]int                { return map[string]int{} }
func (noopStore) MarkCollected(string, int, int)                     {}
func (noopStore) LoadCollected(string) map[[2]int]bool               { return nil }
func (noopStore) UnlockHat(string, int)                              {}
func (noopStore) LoadHats(string) map[int]bool                       { return nil }
func (noopStore) MarkSpecies(string, string)                         {}
func (noopStore) LoadCompendium(string) map[string]bool              { return nil }
func (noopStore) SaveCompanion(string, string)                       {}
func (noopStore) LoadCompanion(string) (string, bool)                { return "", false }
func (noopStore) FixPersonalGate(string, string)                     {}
func (noopStore) LoadPersonalGates(string) map[string]bool           { return nil }
func (noopStore) SaveGateWorld(string, int, bool)                    {}
func (noopStore) AddPlacement(Placement)                             {}
func (noopStore) RemovePlacement(int, int)                           {}
func (noopStore) LoadPlacements() []Placement                        { return nil }
func (noopStore) SaveClaim(Claim)                                    {}
func (noopStore) RemoveClaim(string)                                 {}
func (noopStore) LoadClaims() []Claim                                { return nil }
func (noopStore) SaveArtifact(string, string)                       {}
func (noopStore) LoadArtifacts() map[string]string                  { return map[string]string{} }
func (noopStore) SaveCleared(Cleared)                                {}
func (noopStore) RemoveCleared(int, int)                             {}
func (noopStore) LoadCleared() []Cleared                             { return nil }
func (noopStore) LoadGateWorld() (map[string]int, map[string]bool) {
	return map[string]int{}, map[string]bool{}
}
func (noopStore) SaveProject(Project)     {}
func (noopStore) LoadProjects() []Project { return nil }
func (noopStore) Close() error            { return nil }
