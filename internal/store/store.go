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
func (noopStore) Close() error                          { return nil }
