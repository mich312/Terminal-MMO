package world

import (
	"strings"
	"time"

	"github.com/durst-group/durstworld/internal/markdown"
)

// Deck is a player-authored markdown presentation. Decks are live, in-memory
// state (like chat and presence): they vanish when the server restarts. Slides
// is the source split on --- lines; Current is the shared slide index everyone
// in the Presentation Wing sees. The whole struct is copied out under the lock.
type Deck struct {
	ID      string
	Title   string
	Owner   string
	Source  string // raw markdown
	Slides  []string
	Current int
	Created time.Time
}

// presentArea is the area id decks live in; deck events fan out to everyone
// there so their stage view stays in sync.
const presentArea = "presentation"

// MaxDecks caps how many presentation stages can exist at once. The wing is a
// fixed set of rooms: once full, new decks are refused until an owner retires
// one. Tunable.
const MaxDecks = 8

// SetDeckPersist registers a callback that saves a deck whenever it is created
// or edited (main wires this to the SQLite store). nil disables persistence.
func (w *World) SetDeckPersist(fn func(Deck)) {
	w.mu.Lock()
	w.persist = fn
	w.mu.Unlock()
}

// SetDeckRemove registers a callback that deletes a persisted deck by id (main
// wires this to the SQLite store). nil disables deletion persistence.
func (w *World) SetDeckRemove(fn func(id string)) {
	w.mu.Lock()
	w.removeFn = fn
	w.mu.Unlock()
}

// RemoveDeck retires a deck and its stage. Only the owner may retire it; the
// wing is rebuilt for everyone and the deck is deleted from storage. Returns
// false if the caller isn't the owner or the deck is gone.
func (w *World) RemoveDeck(id, by string) bool {
	w.mu.Lock()
	d, ok := w.decks[id]
	if !ok || d.Owner != by {
		w.mu.Unlock()
		return false
	}
	delete(w.decks, id)
	for i, x := range w.deckOrder {
		if x == id {
			w.deckOrder = append(w.deckOrder[:i], w.deckOrder[i+1:]...)
			break
		}
	}
	w.broadcastToArea(presentArea, Event{Type: EventDeck, Player: by, Area: presentArea, Detail: id})
	remove := w.removeFn
	w.mu.Unlock()
	if remove != nil {
		remove(id)
	}
	return true
}

// LoadDeck inserts a persisted deck at startup without re-saving or
// broadcasting. Decks are restored oldest-first so creation order is preserved.
func (w *World) LoadDeck(id, owner, title, source string, created time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.decks == nil {
		w.decks = make(map[string]*Deck)
	}
	if _, exists := w.decks[id]; exists {
		return
	}
	w.decks[id] = &Deck{
		ID: id, Owner: owner, Title: title, Source: source,
		Slides: markdown.SplitSlides(source), Created: created,
	}
	w.deckOrder = append(w.deckOrder, id)
}

// DeckCount returns how many decks currently exist (for the wing's cap check).
func (w *World) DeckCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.decks)
}

// CreateDeck stores a new deck authored by owner and returns its id, or "" if
// the wing is full (MaxDecks reached). The slide index starts at 0. Everyone in
// the Presentation Wing is told to rebuild, and the deck is persisted (if a
// store is wired).
func (w *World) CreateDeck(owner, title, source string) string {
	w.mu.Lock()
	if w.decks == nil {
		w.decks = make(map[string]*Deck)
	}
	if len(w.decks) >= MaxDecks {
		w.mu.Unlock()
		return ""
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled"
	}
	id := w.uniqueDeckID(title)
	w.decks[id] = &Deck{
		ID: id, Title: title, Owner: owner, Source: source,
		Slides: markdown.SplitSlides(source), Created: time.Now(),
	}
	w.deckOrder = append(w.deckOrder, id)
	w.broadcastToArea(presentArea, Event{Type: EventDeck, Player: owner, Area: presentArea, Detail: id})
	snap, persist := *w.decks[id], w.persist
	w.mu.Unlock()
	if persist != nil {
		persist(snap)
	}
	return id
}

// UpdateDeck replaces a deck's title and source. Only the owner may edit; the
// current slide is clamped into the new range. Returns false if not allowed.
func (w *World) UpdateDeck(id, by, title, source string) bool {
	w.mu.Lock()
	d, ok := w.decks[id]
	if !ok || d.Owner != by {
		w.mu.Unlock()
		return false
	}
	if t := strings.TrimSpace(title); t != "" {
		d.Title = t
	}
	d.Source = source
	d.Slides = markdown.SplitSlides(source)
	if d.Current > len(d.Slides)-1 {
		d.Current = len(d.Slides) - 1
	}
	w.broadcastToArea(presentArea, Event{Type: EventDeck, Player: by, Area: presentArea, Detail: id})
	snap, persist := *d, w.persist
	w.mu.Unlock()
	if persist != nil {
		persist(snap)
	}
	return true
}

// AdvanceDeck moves a deck's shared slide by delta (clamped). Only the owner —
// the presenter — drives the slides. Returns the new index, or -1 if the caller
// isn't the owner or the deck is gone.
func (w *World) AdvanceDeck(id, by string, delta int) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	d, ok := w.decks[id]
	if !ok || d.Owner != by {
		return -1
	}
	idx := d.Current + delta
	if idx < 0 {
		idx = 0
	}
	if idx > len(d.Slides)-1 {
		idx = len(d.Slides) - 1
	}
	d.Current = idx
	w.broadcastToArea(presentArea, Event{Type: EventSlide, Player: by, Area: presentArea, Detail: id, Slide: idx})
	return idx
}

// Decks returns snapshots of all decks in creation order.
func (w *World) Decks() []Deck {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Deck, 0, len(w.deckOrder))
	for _, id := range w.deckOrder {
		if d, ok := w.decks[id]; ok {
			out = append(out, *d)
		}
	}
	return out
}

// GetDeck returns a snapshot of one deck.
func (w *World) GetDeck(id string) (Deck, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	d, ok := w.decks[id]
	if !ok {
		return Deck{}, false
	}
	return *d, true
}

// uniqueDeckID slugifies a title and appends a counter until unique. Caller
// holds w.mu.
func (w *World) uniqueDeckID(title string) string {
	base := slugify(title)
	if base == "" {
		base = "deck"
	}
	for i := 1; ; i++ {
		w.deckSeq++
		id := base
		if _, taken := w.decks[id]; taken || i > 1 {
			id = base + "-" + itoa(w.deckSeq)
		}
		if _, taken := w.decks[id]; !taken {
			return id
		}
	}
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
