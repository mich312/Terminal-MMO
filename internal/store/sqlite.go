package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS players (
	name          TEXT PRIMARY KEY,
	first_seen    INTEGER NOT NULL,
	last_seen     INTEGER NOT NULL,
	visit_count   INTEGER NOT NULL DEFAULT 1,
	areas_visited TEXT NOT NULL DEFAULT '[]'
);
CREATE TABLE IF NOT EXISTS guestbook (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	message    TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	type       TEXT NOT NULL,
	detail     TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS decks (
	id         TEXT PRIMARY KEY,
	owner      TEXT NOT NULL,
	title      TEXT NOT NULL,
	source     TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS positions (
	name TEXT NOT NULL,
	area TEXT NOT NULL,
	x    INTEGER NOT NULL,
	y    INTEGER NOT NULL,
	PRIMARY KEY (name, area)
);
CREATE TABLE IF NOT EXISTS discovery (
	name TEXT NOT NULL,
	cx   INTEGER NOT NULL,
	cy   INTEGER NOT NULL,
	mask INTEGER NOT NULL,
	PRIMARY KEY (name, cx, cy)
);
CREATE TABLE IF NOT EXISTS inventory (
	name  TEXT NOT NULL,
	item  TEXT NOT NULL,
	count INTEGER NOT NULL,
	PRIMARY KEY (name, item)
);
CREATE TABLE IF NOT EXISTS collected (
	name TEXT NOT NULL,
	x    INTEGER NOT NULL,
	y    INTEGER NOT NULL,
	PRIMARY KEY (name, x, y)
);
`

type sqliteStore struct {
	mu sync.Mutex
	db *sql.DB
}

func openSQLite(path string) (*sqliteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// modernc.org/sqlite + concurrent writers don't mix; serialize here.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	// Migrations: add avatar columns to pre-existing DBs. ALTER fails harmlessly
	// if the column already exists, so ignore those errors.
	for _, stmt := range []string{
		`ALTER TABLE players ADD COLUMN avatar_color TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE players ADD COLUMN avatar_style INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE players ADD COLUMN avatar_accessory INTEGER NOT NULL DEFAULT 0`,
	} {
		db.Exec(stmt)
	}
	return &sqliteStore{db: db}, nil
}

// SaveAvatar persists a player's avatar customization (upserting the row).
func (s *sqliteStore) SaveAvatar(name, color string, style, accessory int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	if _, err := s.db.Exec(
		`INSERT INTO players (name, first_seen, last_seen, visit_count, areas_visited,
			avatar_color, avatar_style, avatar_accessory)
		 VALUES (?, ?, ?, 0, '[]', ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
			avatar_color = excluded.avatar_color,
			avatar_style = excluded.avatar_style,
			avatar_accessory = excluded.avatar_accessory`,
		name, now, now, color, style, accessory); err != nil {
		log.Printf("store: save avatar: %v", err)
	}
}

// SavePosition upserts a player's position within an area.
func (s *sqliteStore) SavePosition(name, area string, x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`INSERT INTO positions (name, area, x, y) VALUES (?, ?, ?, ?)
		 ON CONFLICT(name, area) DO UPDATE SET x = excluded.x, y = excluded.y`,
		name, area, x, y); err != nil {
		log.Printf("store: save position: %v", err)
	}
}

// LoadPosition returns a player's saved position within an area.
func (s *sqliteStore) LoadPosition(name, area string) (x, y int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.QueryRow(
		`SELECT x, y FROM positions WHERE name = ? AND area = ?`, name, area).Scan(&x, &y); err != nil {
		return 0, 0, false
	}
	return x, y, true
}

// SaveDiscovery upserts one fog-of-war chunk. mask is a 64-bit cell bitmap; it
// round-trips through SQLite's signed INTEGER as the same 64 bits.
func (s *sqliteStore) SaveDiscovery(name string, cx, cy int, mask uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`INSERT INTO discovery (name, cx, cy, mask) VALUES (?, ?, ?, ?)
		 ON CONFLICT(name, cx, cy) DO UPDATE SET mask = excluded.mask`,
		name, cx, cy, int64(mask)); err != nil {
		log.Printf("store: save discovery: %v", err)
	}
}

// LoadDiscovery returns every discovered chunk for a player.
func (s *sqliteStore) LoadDiscovery(name string) map[[2]int]uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT cx, cy, mask FROM discovery WHERE name = ?`, name)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[[2]int]uint64{}
	for rows.Next() {
		var cx, cy int
		var mask int64
		if err := rows.Scan(&cx, &cy, &mask); err == nil {
			out[[2]int{cx, cy}] = uint64(mask)
		}
	}
	return out
}

// AddItem increments a player's count of one inventory item.
func (s *sqliteStore) AddItem(name, item string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`INSERT INTO inventory (name, item, count) VALUES (?, ?, 1)
		 ON CONFLICT(name, item) DO UPDATE SET count = count + 1`,
		name, item); err != nil {
		log.Printf("store: add item: %v", err)
	}
}

// LoadInventory returns a player's item counts.
func (s *sqliteStore) LoadInventory(name string) map[string]int {
	out := map[string]int{}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT item, count FROM inventory WHERE name = ?`, name)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var item string
		var count int
		if err := rows.Scan(&item, &count); err == nil {
			out[item] = count
		}
	}
	return out
}

// MarkCollected records that a player harvested the item at (x,y).
func (s *sqliteStore) MarkCollected(name string, x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO collected (name, x, y) VALUES (?, ?, ?)`,
		name, x, y); err != nil {
		log.Printf("store: mark collected: %v", err)
	}
}

// LoadCollected returns the cells a player has already harvested.
func (s *sqliteStore) LoadCollected(name string) map[[2]int]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT x, y FROM collected WHERE name = ?`, name)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[[2]int]bool{}
	for rows.Next() {
		var x, y int
		if err := rows.Scan(&x, &y); err == nil {
			out[[2]int{x, y}] = true
		}
	}
	return out
}

// LoadAvatar returns a player's saved avatar customization.
func (s *sqliteStore) LoadAvatar(name string) (color string, style, accessory int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.QueryRow(
		`SELECT avatar_color, avatar_style, avatar_accessory FROM players WHERE name = ?`,
		name).Scan(&color, &style, &accessory)
	if err != nil {
		return "", 0, 0, false
	}
	return color, style, accessory, true
}

func (s *sqliteStore) RecordVisit(name string) VisitInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()

	var prevLast, visits int64
	err := s.db.QueryRow(`SELECT last_seen, visit_count FROM players WHERE name = ?`, name).
		Scan(&prevLast, &visits)
	switch {
	case err == sql.ErrNoRows:
		if _, err := s.db.Exec(
			`INSERT INTO players (name, first_seen, last_seen, visit_count, areas_visited)
			 VALUES (?, ?, ?, 1, '[]')`, name, now, now); err != nil {
			log.Printf("store: record visit: %v", err)
		}
		return VisitInfo{VisitCount: 1, FirstVisit: true}
	case err != nil:
		log.Printf("store: record visit: %v", err)
		return VisitInfo{VisitCount: 1, FirstVisit: true}
	}

	if _, err := s.db.Exec(
		`UPDATE players SET last_seen = ?, visit_count = visit_count + 1 WHERE name = ?`,
		now, name); err != nil {
		log.Printf("store: record visit: %v", err)
	}
	return VisitInfo{
		VisitCount: int(visits) + 1,
		LastSeen:   time.Unix(prevLast, 0),
	}
}

func (s *sqliteStore) RecordDisconnect(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`UPDATE players SET last_seen = ? WHERE name = ?`,
		time.Now().Unix(), name); err != nil {
		log.Printf("store: record disconnect: %v", err)
	}
}

func (s *sqliteStore) RecordAreaVisit(name, area string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw string
	if err := s.db.QueryRow(`SELECT areas_visited FROM players WHERE name = ?`, name).
		Scan(&raw); err != nil {
		return // player row missing or unreadable; not worth fighting
	}
	var areas []string
	if err := json.Unmarshal([]byte(raw), &areas); err != nil {
		// Corrupt blob: log and leave it untouched rather than silently
		// overwriting it with a fresh single-element list.
		log.Printf("store: areas_visited for %q is corrupt (%v); leaving it", name, err)
		return
	}
	for _, a := range areas {
		if a == area {
			return
		}
	}
	areas = append(areas, area)
	buf, _ := json.Marshal(areas)
	if _, err := s.db.Exec(`UPDATE players SET areas_visited = ? WHERE name = ?`,
		string(buf), name); err != nil {
		log.Printf("store: record area visit: %v", err)
	}
}

func (s *sqliteStore) LogEvent(name, typ, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`INSERT INTO events (name, type, detail, created_at) VALUES (?, ?, ?, ?)`,
		name, typ, detail, time.Now().Unix()); err != nil {
		log.Printf("store: log event: %v", err)
	}
}

func (s *sqliteStore) SignGuestbook(name, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO guestbook (name, message, created_at) VALUES (?, ?, ?)`,
		name, message, time.Now().Unix())
	if err != nil {
		log.Printf("store: sign guestbook: %v", err)
	}
	return err
}

func (s *sqliteStore) GuestbookEntries(n int) []GuestbookEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT name, message, created_at FROM guestbook ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		log.Printf("store: guestbook entries: %v", err)
		return nil
	}
	defer rows.Close()
	var out []GuestbookEntry
	for rows.Next() {
		var e GuestbookEntry
		var ts int64
		if err := rows.Scan(&e.Name, &e.Message, &ts); err != nil {
			continue
		}
		e.CreatedAt = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

// SaveDeck upserts a player-authored presentation deck, keyed by id (owned by
// owner). The created_at of an existing row is preserved.
func (s *sqliteStore) SaveDeck(id, owner, title, source string, createdUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`INSERT INTO decks (id, owner, title, source, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			owner = excluded.owner, title = excluded.title, source = excluded.source`,
		id, owner, title, source, createdUnix); err != nil {
		log.Printf("store: save deck: %v", err)
	}
}

// DeleteDeck removes a persisted deck by id.
func (s *sqliteStore) DeleteDeck(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM decks WHERE id = ?`, id); err != nil {
		log.Printf("store: delete deck: %v", err)
	}
}

// LoadDecks returns every persisted deck, oldest first.
func (s *sqliteStore) LoadDecks() []DeckRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, owner, title, source, created_at FROM decks ORDER BY created_at, id`)
	if err != nil {
		log.Printf("store: load decks: %v", err)
		return nil
	}
	defer rows.Close()
	var out []DeckRecord
	for rows.Next() {
		var d DeckRecord
		if err := rows.Scan(&d.ID, &d.Owner, &d.Title, &d.Source, &d.Created); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}
