package game

// Controls is the single source of truth for the key reference shown by the
// "?" help overlay. Both clients (the glyph bubbletea client and the default HD
// pixel client) render from it, so the two never drift and every control the
// game responds to is discoverable from one place.
//
// Key labels are deliberately ASCII ("WASD / arrows", not "WASD/↑↓←→") so they
// render identically in HD, whose bitmap font is ASCII-only.

// Control is one key (or key group) and what it does.
type Control struct {
	Keys string
	Desc string
}

// ControlGroup groups related controls under a heading.
type ControlGroup struct {
	Title string
	Items []Control
}

// Controls returns the full control reference, grouped for display.
func Controls() []ControlGroup {
	return []ControlGroup{
		{"Move", []Control{
			{"WASD / arrows", "walk (enter portals)"},
			{"Y U B N", "move diagonally"},
			{"Shift + move", "run"},
		}},
		{"Act", []Control{
			{"e", "use what you're on"},
			{"Enter", "chat to players nearby"},
			{"/", "run a command"},
		}},
		{"Panels", []Control{
			{"c", "character editor"},
			{"i", "inventory & hats"},
			{"Tab", "who's online"},
			{"?", "this help"},
			{"q", "quit (press twice)"},
		}},
	}
}

// CommandReference returns the slash-commands as (usage, summary) pairs — the
// same list /help shows, exposed so the HD client and the "?" overlay can render
// it without a Model.
func CommandReference() [][2]string {
	out := make([][2]string, 0, len(commands))
	for _, c := range commands {
		out = append(out, [2]string{c.usage, c.summary})
	}
	return out
}
