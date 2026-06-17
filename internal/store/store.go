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
func (noopStore) SaveDeck(string, string, string, string, int64) {}
func (noopStore) LoadDecks() []DeckRecord                        { return nil }
func (noopStore) Close() error                                   { return nil }
