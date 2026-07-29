package game

import (
	"math/rand"

	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// SetupAvatar restores a player's persisted color/style/accessory, or — on a
// first visit — rolls a random look and remembers it, so everyone spawns with a
// distinct avatar that then stays theirs across reconnects.
//
// Every client calls this at join, which is why it lives here rather than in
// any one of them: your look is part of who you are in the world, not a
// property of how you happen to be connected. Walk in over SSH, come back in a
// browser, and you're the same character.
func SetupAvatar(w *world.World, st store.Store, name string) {
	if color, style, accessory, ok := st.LoadAvatar(name); ok {
		if color != "" {
			w.SetColor(name, lipgloss.Color(color))
		}
		w.SetAvatar(name, style, accessory)
		// Grandfather a hat the player is already wearing into their owned set,
		// so gating stays consistent for anyone from before hats were earned.
		if accessory != 0 {
			st.UnlockHat(name, accessory)
		}
		return
	}
	// New players spawn with a random body/color but no hat — hats are earned by
	// exploring the Wilds.
	color := ui.AvatarColorByIndex(rand.Intn(ui.NumAvatarColors()))
	style := rand.Intn(NumAvatarStyles())
	w.SetColor(name, color)
	w.SetAvatar(name, style, 0)
	st.SaveAvatar(name, string(color), style, 0)
}
