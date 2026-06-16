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
	// EventSlide: a presentation-room slide changed. Detail holds the room
	// key, Slide the new index.
	EventSlide
	// EventEmote: a proximity emote ("/me waves"). Like EventChat — delivered
	// only within ChatRadius in the same area. Detail holds the action text.
	EventEmote
	// EventWhisper: a private message. Delivered only to Target. Player is the
	// sender, Detail the text.
	EventWhisper
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
