package game

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/world"
)

// Glyph-client trade: a modal panel driven by keys (←→ pick from your pack,
// +/- stage an offer, r ready, esc cancel), fed by the world snapshot. Trade
// initiation/accept are chat commands (see commands.go).

// cmdTrade asks a named nearby player to trade.
func cmdTrade(m *Model, args []string) tea.Cmd {
	if len(args) == 0 {
		m.addSystemLine("usage: /trade <player> — stand next to them first")
		return nil
	}
	target, ok := m.resolvePlayer(args[0])
	if !ok {
		m.addSystemLine("no other player here named " + args[0])
		return nil
	}
	if err := m.ctx.World.RequestTrade(m.ctx.Name, target); err != nil {
		m.addSystemLine(err.Error())
		return nil
	}
	m.addSystemLine("asked " + target + " to trade — waiting for them to accept")
	return nil
}

// cmdAccept accepts the latest pending trade request.
func cmdAccept(m *Model, args []string) tea.Cmd {
	if m.tradeReq == "" {
		m.addSystemLine("no trade request to accept")
		return nil
	}
	from := m.tradeReq
	m.tradeReq = ""
	if err := m.ctx.World.AcceptTrade(m.ctx.Name, from); err != nil {
		m.addSystemLine(err.Error())
	}
	return nil
}

// cmdDecline declines the latest pending trade request.
func cmdDecline(m *Model, args []string) tea.Cmd {
	if m.tradeReq == "" {
		m.addSystemLine("no trade request to decline")
		return nil
	}
	m.ctx.World.DeclineTrade(m.ctx.Name, m.tradeReq)
	m.tradeReq = ""
	m.addSystemLine("declined the trade")
	return nil
}

// resolvePlayer finds an online player by case-insensitive name (never self).
func (m *Model) resolvePlayer(name string) (string, bool) {
	la := strings.ToLower(name)
	for _, p := range m.ctx.World.AllPlayers() {
		if p.Name != m.ctx.Name && strings.ToLower(p.Name) == la {
			return p.Name, true
		}
	}
	return "", false
}

// handleTradeEvent reacts to the world's trade phases on the event stream.
func (m *Model) handleTradeEvent(ev world.Event) {
	switch ev.Detail {
	case world.TradeRequest:
		m.tradeReq = ev.Player
		m.addSystemLine(ev.Player + " wants to trade — /accept or /decline")
	case world.TradeOpen:
		m.showTrade, m.tradeSel, m.tradeReq = true, 0, ""
		m.showInfo, m.showChar, m.showPlayers = false, false, false
	case world.TradeUpdate:
		// The panel re-renders from live world state each frame; nothing to store.
	case world.TradeDone:
		if s, ok := ApplyCompletedTrade(m.ctx); ok {
			m.addToast(s)
		}
		m.showTrade = false
	case world.TradeCancel:
		if m.showTrade {
			m.addSystemLine("trade cancelled")
		}
		m.showTrade = false
	case world.TradeDeclined:
		m.addSystemLine(ev.Player + " declined to trade")
	}
}

// handleTradeKey drives the modal trade table.
func (m *Model) handleTradeKey(msg tea.KeyMsg) tea.Cmd {
	pack := packRows(m.ctx.Inventory)
	switch msg.String() {
	case "esc", "q":
		m.ctx.World.CancelTrade(m.ctx.Name) // the cancel event closes the panel
	case "enter":
		m.chatInput.Focus() // haggle while the table is open
	case "left", "h":
		if len(pack) > 0 {
			m.tradeSel = (m.tradeSel + len(pack) - 1) % len(pack)
		}
	case "right", "l":
		if len(pack) > 0 {
			m.tradeSel = (m.tradeSel + 1) % len(pack)
		}
	case "+", "=":
		if id, ok := tradeSelID(pack, m.tradeSel); ok {
			AdjustOffer(m.ctx, id, +1)
		}
	case "-", "_":
		if id, ok := tradeSelID(pack, m.tradeSel); ok {
			AdjustOffer(m.ctx, id, -1)
		}
	case "r":
		snap, _ := m.ctx.World.TradeSnapshot(m.ctx.Name)
		m.ctx.World.SetReady(m.ctx.Name, !snap.YouReady)
	}
	return nil
}

// tradeSelID returns the item id of the selected pack slot.
func tradeSelID(pack []TradeRow, sel int) (string, bool) {
	if len(pack) == 0 {
		return "", false
	}
	if sel < 0 || sel >= len(pack) {
		sel = 0
	}
	return pack[sel].Item.ID, true
}

// tradePanel renders the live trade table as a text overlay.
func (m *Model) tradePanel() string {
	v, ok := TradeViewFor(m.ctx, m.tradeSel)
	if !ok {
		return ""
	}
	rows := []string{m.theme.PanelTitle.Render("Trade with " + v.Them.Name), ""}
	rows = append(rows, m.tradeSideLines("Your offer", v.You)...)
	rows = append(rows, "")
	rows = append(rows, m.tradeSideLines(v.Them.Name+"'s offer", v.Them)...)
	rows = append(rows, "", m.theme.Dim.Render("Your pack:"), m.tradePackLine(v), "")

	sel := "—"
	if id, ok := tradeSelID(v.Pack, m.tradeSel); ok {
		if it, ok := ItemByID(id); ok {
			sel = it.Name
		}
	}
	rows = append(rows, m.theme.Dim.Render(
		fmt.Sprintf("←→ select (%s) · + offer · - withdraw · r ready · esc cancel", sel)))
	return m.theme.Panel.Render(strings.Join(rows, "\n"))
}

// tradeSideLines renders one trader's header (with a ready chip) and staged rows.
func (m *Model) tradeSideLines(title string, p TradeParty) []string {
	chip := m.theme.Warn.Render("deciding")
	if p.Ready {
		chip = m.theme.Accent.Render("READY")
	}
	lines := []string{fmt.Sprintf("%s  %s", m.theme.Bright.Render(title), chip)}
	if len(p.Offer) == 0 {
		return append(lines, "  "+m.theme.Dim.Render("(nothing yet)"))
	}
	for _, r := range p.Offer {
		glyph := m.theme.Fg(lipgloss.Color(r.Item.Hex)).Render(string(r.Item.Glyph))
		lines = append(lines, fmt.Sprintf("  %s %s %s", glyph,
			m.theme.ChatText.Render(padRight(r.Item.Name, 16)),
			m.theme.Accent.Render(fmt.Sprintf("×%d", r.N))))
	}
	return lines
}

// tradePackLine renders the pack picker as a single row of glyph×count chips,
// the selected one bracketed.
func (m *Model) tradePackLine(v TradeView) string {
	if len(v.Pack) == 0 {
		return "  " + m.theme.Dim.Render("(empty)")
	}
	parts := make([]string, 0, len(v.Pack))
	for i, r := range v.Pack {
		glyph := m.theme.Fg(lipgloss.Color(r.Item.Hex)).Render(string(r.Item.Glyph))
		chip := fmt.Sprintf("%s×%d", glyph, r.N)
		if i == m.tradeSel {
			chip = m.theme.Accent.Render("[") + chip + m.theme.Accent.Render("]")
		} else {
			chip = " " + chip + " "
		}
		parts = append(parts, chip)
	}
	return "  " + strings.Join(parts, " ")
}
