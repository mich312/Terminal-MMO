package web

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/game"
)

// Chat and the slash commands.
//
// The command set is the HD client's, deliberately: someone who plays over SSH
// and then opens the browser should not have to learn a second vocabulary. The
// browser adds nothing here except that /help and /who open real panels instead
// of rasterized ones.

// handleChat routes a submitted line: plain text is proximity chat (heard
// within 8 tiles), a leading "/" is a command.
func (s *session) handleChat(text string) {
	if !strings.HasPrefix(text, "/") {
		s.w.Chat(s.name, text)
		return
	}
	fields := strings.Fields(text[1:])
	if len(fields) == 0 {
		return
	}
	arg := func(i int) string {
		if len(fields) > i {
			return fields[i]
		}
		return ""
	}
	rest := func(i int) string {
		if len(fields) > i {
			return strings.Join(fields[i:], " ")
		}
		return ""
	}

	switch strings.ToLower(fields[0]) {
	case "help", "h":
		s.openPanel("help")
	case "who":
		s.openPanel("who")
	case "compendium", "codex", "i":
		s.openPanel("compendium")
	case "character", "char":
		s.openPanel("character")
	case "craft":
		s.openPanel("craft")
	case "where":
		if self, ok := s.w.Self(s.name); ok {
			s.system(fmt.Sprintf("%s — (%d, %d)", game.DisplayName(s.areaID), self.X, self.Y))
		}
	case "me":
		if r := rest(1); r != "" {
			s.w.Emote(s.name, r)
		}
	case "w", "tell", "msg", "whisper":
		to, msg := arg(1), rest(2)
		if to == "" || msg == "" {
			s.system("usage: /w <name> <message>")
			break
		}
		if s.w.Whisper(s.name, to, msg) {
			s.sendNow(ChatMsg{T: MsgChat, Text: "→ " + to + ": " + msg, Kind: "whisper"})
		} else {
			s.system(to + " is not online")
		}
	case "roll":
		s.w.Chat(s.name, text) // the world's command layer owns dice
	case "goto", "go":
		s.gotoArea(arg(1))
	case "trade", "tr":
		s.startTrade(arg(1))
	case "accept":
		if s.tradeReq == "" {
			s.system("no trade request to accept")
			break
		}
		from := s.tradeReq
		s.tradeReq = ""
		if err := s.w.AcceptTrade(s.name, from); err != nil {
			s.system(err.Error())
		}
	case "decline":
		if s.tradeReq != "" {
			s.w.DeclineTrade(s.name, s.tradeReq)
			s.tradeReq = ""
			s.system("declined the trade")
		}
	case "wield":
		s.gctx.Wielded = arg(1)
		if s.gctx.Wielded == "" {
			s.system("wielding: strongest arm you carry")
		} else {
			s.system("wielding " + itemName(s.gctx.Wielded))
		}
	case "clear":
		s.sendNow(ChatMsg{T: MsgChat, Kind: "clear"})
	default:
		s.system("unknown command — /help for the list")
	}
}

// gotoArea teleports to a registered area, or lists them when called bare.
func (s *session) gotoArea(dest string) {
	if dest == "" {
		s.system("go to — /goto <name>:")
		for _, ln := range game.GotoListLines() {
			s.system(ln)
		}
		return
	}
	dest = strings.ToLower(dest)
	if dest == s.areaID {
		return
	}
	if !game.AreaRegistered(dest) {
		s.system("no such area: " + dest + " — /goto for the list")
		return
	}
	s.enter(s.areaID, dest)
	s.render(true)
}

// startTrade asks a player standing near you to open a table.
func (s *session) startTrade(who string) {
	if who == "" {
		s.system("usage: /trade <player> — stand next to them")
		return
	}
	target, found := "", false
	lower := strings.ToLower(who)
	for _, p := range s.w.PlayersInArea(s.areaID) {
		if p.Name != s.name && strings.ToLower(p.Name) == lower {
			target, found = p.Name, true
		}
	}
	if !found {
		s.system("no other player here named " + who)
		return
	}
	if err := s.w.RequestTrade(s.name, target); err != nil {
		s.system(err.Error())
		return
	}
	s.system("asked " + target + " to trade")
}

// keyRunes builds the rune key message areas expect for a single character.
func keyRunes(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
