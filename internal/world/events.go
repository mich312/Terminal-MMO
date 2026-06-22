package world

// EventType enumerates everything the world can tell a session.
type EventType int

const (
	// EventTick fires at 2 Hz and drives the portal pulse.
	EventTick EventType = iota
	// EventJoined: a player entered an area (fresh connect or transition).
	EventJoined
	// EventLeft: a player left an area. Detail holds the destination area's
	// display name, or "" if they disconnected.
	EventLeft
	// EventMoved: a player moved within an area.
	EventMoved
	// EventChat: proximity chat. Delivered only to players within
	// ChatRadius (Chebyshev) of the sender, in the same area.
	EventChat
	// EventSlide: a presentation deck's shared slide changed. Detail holds the
	// deck id, Slide the new index.
	EventSlide
	// EventEmote: a proximity emote ("/me waves"). Like EventChat — delivered
	// only within ChatRadius in the same area. Detail holds the action text.
	EventEmote
	// EventWhisper: a private message. Delivered only to Target. Player is the
	// sender, Detail the text.
	EventWhisper
	// EventDeck: a presentation deck was created or edited, so the Presentation
	// Wing should rebuild its stages. Detail holds the deck id.
	EventDeck
	// EventPlaced: a player placed or removed a structure in the shared world.
	// X,Y is the cell, Player the owner, Detail the placeable kind ("" on
	// removal). Both clients re-read placements from the world, so this just
	// nudges a redraw.
	EventPlaced
	// EventTrade: a player-to-player trade changed state. Target is the other
	// party (from the recipient's view) and Detail is the phase:
	// "request", "open", "update", "done", "cancel", or "declined".
	EventTrade
	// EventPlayerDamaged: Target took a non-lethal hit (docs/WEAPON_PLAN.md).
	// Player is the attacker, Detail the weapon name (or "" for bare hands),
	// X/Y the struck tile (for a hit-spark). Delivered to the whole area so
	// onlookers see it; the victim reacts even though it didn't act.
	EventPlayerDamaged
	// EventPlayerDowned: Target was knocked out. Player is the attacker.
	EventPlayerDowned
	// EventPlayerRespawn: Target is back on their feet at full HP. X/Y is the
	// respawn tile.
	EventPlayerRespawn
)

// Trade event phases, carried in Event.Detail.
const (
	TradeRequest  = "request"  // someone wants to trade with you
	TradeOpen     = "open"     // the table is now open for both
	TradeUpdate   = "update"   // an offer or ready flag changed
	TradeDone     = "done"     // both confirmed; apply your delta
	TradeCancel   = "cancel"   // the table closed without a swap
	TradeDeclined = "declined" // your request was declined
)

// ChatRadius is the Chebyshev distance within which chat is heard.
const ChatRadius = 8

// Event is a single world notification, delivered over each session's
// buffered channel.
type Event struct {
	Type   EventType
	Player string // who did it
	Area   string // area id it happened in
	X, Y   int
	Detail string // chat text, destination name, room key
	Target string // recipient name, for EventWhisper
	Slide  int
	Pulse  bool   // for EventTick
	Frame  uint64 // monotonic tick counter, for multi-frame animation
}
