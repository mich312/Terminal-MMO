package web

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
)

// The browser's panels.
//
// This is the one place where the browser client is genuinely simpler than the
// terminal ones. HD has no glyph layer, so it rasterizes every panel — the
// compendium, the character editor, the trade table — into the pixel frame by
// hand, which is most of internal/game/hd_ui.go. A browser already knows how to
// lay out a list, so the server ships the *data* and the DOM does the drawing.
//
// The data itself comes from the same functions the HD panels draw from
// (game.Compendium, game.Controls, game.TradeViewFor …), so there is no second
// source of truth about what's in your pack or what a recipe costs.

// openPanel switches the open panel and sends its contents.
func (s *session) openPanel(name string) {
	s.panel = name
	s.sendPanel()
}

// closePanel tells the client to dismiss whatever is open.
func (s *session) closePanel() {
	s.panel = ""
	s.sendNow(PanelMsg{T: MsgPanel, Panel: ""})
}

// handlePanel processes a panel message from the browser: opening, closing,
// moving the selection, or taking the panel's action (craft this, buy that,
// toggle ready).
func (s *session) handlePanel(m ClientMsg) {
	if m.Panel == "" && m.Act == "" {
		s.closePanel()
		return
	}
	if m.Panel != "" && m.Panel != s.panel {
		s.panel, s.panelSel = m.Panel, 0
	}
	if m.Sel >= 0 {
		s.panelSel = m.Sel
	}
	switch s.panel {
	case "craft":
		s.craftAction(m.Act)
	case "character":
		s.characterAction(m.Act, m.Sel)
	case "machine":
		s.machineAction(m.Act)
	case "stall":
		s.stallAction(m.Act)
	case "trade":
		s.tradeAction(m.Act)
	case "build":
		s.buildAction(m.Act)
	}
	if s.panel == "" {
		return
	}
	s.sendPanel()
	s.render(false)
}

// sendPanel renders the open panel's current contents.
func (s *session) sendPanel() {
	switch s.panel {
	case "compendium":
		s.sendNow(s.compendiumPanel())
	case "character":
		s.sendNow(s.characterPanel())
	case "who":
		s.sendNow(s.whoPanel())
	case "help":
		s.sendNow(s.helpPanel())
	case "craft":
		s.sendNow(s.craftPanel())
	case "machine":
		s.sendNow(s.machinePanel())
	case "stall":
		s.sendNow(s.stallPanel())
	case "trade":
		s.sendNow(s.tradePanel())
	case "":
		s.sendNow(PanelMsg{T: MsgPanel, Panel: ""})
	}
}

// compendiumPanel is the codex of every find and wearable — the same content
// the 'i' panel shows in HD, including the entries you haven't found yet (dim,
// but still described, so it reads as a checklist).
func (s *session) compendiumPanel() PanelMsg {
	p := PanelMsg{T: MsgPanel, Panel: "compendium", Title: "Compendium"}
	for _, grp := range game.Compendium(s.gctx.Inventory) {
		p.Rows = append(p.Rows, PanelRow{Label: grp.Title, Value: "", Desc: "", Dim: false, Warn: false, Sel: false, Hex: ui.HexAccent})
		for _, e := range grp.Entries {
			val := ""
			if e.Owned > 0 {
				val = "×" + strconv.Itoa(e.Owned)
			}
			desc := e.Item.About
			if e.Item.Found != "" {
				desc += " — " + e.Item.Found
			}
			if e.Note != "" {
				desc += " · " + e.Note
			}
			p.Rows = append(p.Rows, PanelRow{
				Label: e.Item.Name, Value: val, Desc: desc,
				Hex: e.Item.Hex, Dim: e.Owned == 0,
			})
		}
	}
	p.Rows = append(p.Rows, PanelRow{Label: "Wearables", Hex: ui.HexAccent})
	for _, wr := range game.Wearables(s.gctx) {
		val := ""
		switch {
		case wr.Worn:
			val = "worn"
		case wr.Owned:
			val = "found"
		}
		desc := wr.Source
		if wr.Power != "" {
			desc += " · " + wr.Power
		}
		p.Rows = append(p.Rows, PanelRow{
			Label: wr.Name, Value: val, Desc: desc, Dim: !wr.Owned,
		})
	}
	return p
}

// characterPanel is the avatar editor: body, color and the hats you've earned.
func (s *session) characterPanel() PanelMsg {
	p := PanelMsg{T: MsgPanel, Panel: "character", Title: "Character", Sel: s.panelSel,
		Footer: "← → to change · Esc to close"}
	self, _ := s.w.Self(s.name)
	p.Rows = []PanelRow{
		{Label: "Body", Value: strconv.Itoa(self.Style + 1)},
		{Label: "Color", Value: string(self.Color), Hex: string(self.Color)},
		{Label: "Hat", Value: game.AccessoryName(self.Accessory)},
	}
	return p
}

// whoPanel lists everyone online and where they are.
func (s *session) whoPanel() PanelMsg {
	p := PanelMsg{T: MsgPanel, Panel: "who", Title: "Who's online"}
	for _, pl := range s.w.AllPlayers() {
		label := pl.Name
		if pl.Name == s.name {
			label += " (you)"
		}
		p.Rows = append(p.Rows, PanelRow{
			Label: label,
			Value: game.DisplayName(pl.Area),
			Hex:   string(pl.Color),
		})
	}
	p.Footer = fmt.Sprintf("%d online", len(p.Rows))
	return p
}

// helpPanel is every key and chat command, straight from the shared reference
// both terminal clients use.
func (s *session) helpPanel() PanelMsg {
	p := PanelMsg{T: MsgPanel, Panel: "help", Title: "Controls"}
	for _, grp := range game.Controls() {
		p.Rows = append(p.Rows, PanelRow{Label: grp.Title, Hex: ui.HexAccent})
		for _, c := range grp.Items {
			p.Rows = append(p.Rows, PanelRow{Label: c.Keys, Desc: c.Desc})
		}
	}
	p.Rows = append(p.Rows, PanelRow{Label: "Chat commands", Hex: ui.HexAccent})
	for _, c := range game.CommandReference() {
		p.Rows = append(p.Rows, PanelRow{Label: c[0], Desc: c[1]})
	}
	return p
}

// craftPanel is the crafting bench: every recipe, what it needs, and how many
// you could make right now.
func (s *session) craftPanel() PanelMsg {
	p := PanelMsg{T: MsgPanel, Panel: "craft", Title: "Crafting", Sel: s.panelSel,
		Footer: "Enter to craft · Esc to close"}
	for i, r := range game.Recipes {
		n := game.Craftable(r, s.gctx.Inventory)
		val := ""
		if n > 0 {
			val = "×" + strconv.Itoa(n)
		}
		p.Rows = append(p.Rows, PanelRow{
			Label: r.Name,
			Value: val,
			Desc:  game.RecipeNeeds(r) + " — " + r.Blurb,
			Warn:  n == 0,
			Sel:   i == s.panelSel,
		})
	}
	return p
}

func (s *session) craftAction(act string) {
	if act != "use" {
		return
	}
	if s.panelSel >= 0 && s.panelSel < len(game.Recipes) {
		if game.Craft(s.gctx, game.Recipes[s.panelSel]) {
			s.system("crafted " + game.Recipes[s.panelSel].Name)
		} else {
			s.system("not enough materials for " + game.Recipes[s.panelSel].Name)
		}
	}
}

// characterAction cycles a field of the avatar editor.
func (s *session) characterAction(act string, field int) {
	switch act {
	case "prev":
		game.CycleAvatarField(s.gctx, field, -1)
	case "next":
		game.CycleAvatarField(s.gctx, field, 1)
	}
}

// machinePanel shows a placed machine's buffers and whether it's running.
func (s *session) machinePanel() PanelMsg {
	p := PanelMsg{T: MsgPanel, Panel: "machine", Title: "Machine",
		Footer: "e collect · f refuel · Esc close"}
	v, ok := game.MachineSnapshot(s.gctx, s.machXY[0], s.machXY[1])
	if !ok {
		return PanelMsg{T: MsgPanel, Panel: ""}
	}
	p.Title = v.Kind.Name
	state := "idle — needs input"
	if v.Running {
		state = fmt.Sprintf("running — next in %ds", v.NextSec)
	}
	p.Rows = []PanelRow{
		{Label: "Status", Value: state, Warn: !v.Running},
		{Label: "In", Value: fmt.Sprintf("%d × %s", v.State.In, itemName(v.Kind.In))},
		{Label: "Out", Value: fmt.Sprintf("%d × %s", v.State.Out, itemName(v.Kind.Out))},
	}
	return p
}

func (s *session) machineAction(act string) {
	switch act {
	case "use":
		if n := game.CollectMachine(s.gctx, s.machXY[0], s.machXY[1]); n > 0 {
			s.system(fmt.Sprintf("collected %d", n))
		}
	case "fuel":
		if n := game.RefuelMachine(s.gctx, s.machXY[0], s.machXY[1]); n > 0 {
			s.system(fmt.Sprintf("loaded %d", n))
		}
	}
}

// stallPanel shows a Concession's offers — and, for its owner, the till.
func (s *session) stallPanel() PanelMsg {
	st, ok := game.StallSnapshot(s.gctx, s.stallXY[0], s.stallXY[1])
	if !ok {
		return PanelMsg{T: MsgPanel, Panel: ""}
	}
	owner := game.StallOwner(s.gctx, s.stallXY[0], s.stallXY[1])
	p := PanelMsg{T: MsgPanel, Panel: "stall", Title: "Concession", Sel: s.panelSel}
	p.Footer = "Enter to buy · Esc close"
	if owner {
		p.Footer = "f collect till · x remove offer · Esc close"
	}
	for i, o := range st.Offers {
		p.Rows = append(p.Rows, PanelRow{
			Label: fmt.Sprintf("%d × %s", o.GiveN, itemName(o.GiveItem)),
			Value: fmt.Sprintf("for %d × %s", o.AskN, itemName(o.AskItem)),
			Desc:  fmt.Sprintf("%d in stock · %d sold", o.Stock, o.Sold),
			Warn:  !game.CanAcceptOffer(o, s.gctx.Inventory),
			Sel:   i == s.panelSel,
		})
	}
	if len(st.Offers) == 0 {
		p.Rows = append(p.Rows, PanelRow{Label: "nothing for sale", Dim: true})
	}
	if owner && len(st.Till) > 0 {
		var parts []string
		for id, n := range st.Till {
			parts = append(parts, fmt.Sprintf("%d × %s", n, itemName(id)))
		}
		p.Rows = append(p.Rows, PanelRow{Label: "Till", Value: strings.Join(parts, ", "), Hex: ui.HexAccent})
	}
	return p
}

func (s *session) stallAction(act string) {
	switch act {
	case "use":
		if _, ok := game.AcceptOffer(s.gctx, s.stallXY[0], s.stallXY[1], s.panelSel); ok {
			s.system("bought")
		} else {
			s.system("you can't afford that")
		}
	case "fuel": // 'f' — collect the till
		if n := game.CollectTill(s.gctx, s.stallXY[0], s.stallXY[1]); n > 0 {
			s.system(fmt.Sprintf("collected %d from the till", n))
		}
	case "remove":
		if game.RemoveOffer(s.gctx, s.stallXY[0], s.stallXY[1], s.panelSel) && s.panelSel > 0 {
			s.panelSel--
		}
	}
}

// tradePanel is the live trade table: both offers, both ready flags, and your
// pack to stage from. It reads world state every time, so the two sides never
// drift apart.
func (s *session) tradePanel() PanelMsg {
	v, ok := game.TradeViewFor(s.gctx, s.panelSel)
	if !ok {
		return PanelMsg{T: MsgPanel, Panel: ""}
	}
	p := PanelMsg{T: MsgPanel, Panel: "trade", Title: "Trade with " + v.Them.Name,
		Sel: s.panelSel, Footer: "← → pick · +/- stage · r ready · Esc cancel"}
	p.Rows = append(p.Rows, PanelRow{Label: "You offer", Hex: ui.HexAccent,
		Value: readyLabel(v.You.Ready)})
	for _, r := range v.You.Offer {
		p.Rows = append(p.Rows, PanelRow{Label: r.Item.Name, Value: "×" + strconv.Itoa(r.N), Hex: r.Item.Hex})
	}
	p.Rows = append(p.Rows, PanelRow{Label: v.Them.Name + " offers", Hex: ui.HexAccent,
		Value: readyLabel(v.Them.Ready)})
	for _, r := range v.Them.Offer {
		p.Rows = append(p.Rows, PanelRow{Label: r.Item.Name, Value: "×" + strconv.Itoa(r.N), Hex: r.Item.Hex})
	}
	p.Rows = append(p.Rows, PanelRow{Label: "Your pack", Hex: ui.HexAccent})
	for i, r := range v.Pack {
		p.Rows = append(p.Rows, PanelRow{
			Label: r.Item.Name, Value: "×" + strconv.Itoa(r.N),
			Hex: r.Item.Hex, Sel: i == s.panelSel,
		})
	}
	p.Extra = map[string]any{"pack": len(v.Pack)}
	return p
}

func readyLabel(ready bool) string {
	if ready {
		return "ready"
	}
	return "not ready"
}

func (s *session) tradeAction(act string) {
	switch act {
	case "add":
		_ = game.OfferSlot(s.gctx, s.panelSel, +1)
	case "sub":
		_ = game.OfferSlot(s.gctx, s.panelSel, -1)
	case "ready":
		snap, _ := s.w.TradeSnapshot(s.name)
		s.w.SetReady(s.name, !snap.YouReady)
	case "cancel":
		s.w.CancelTrade(s.name)
		s.closePanel()
	}
}

// buildAction drives build mode from the browser's palette, which shows the
// same placeables the HD build panel does.
func (s *session) buildAction(act string) {
	switch act {
	case "next":
		s.sendArea(keyRunes("r"))
	case "place":
		s.sendArea(keyRunes("e"))
	case "remove":
		s.sendArea(keyRunes("x"))
	case "close":
		s.sendArea(keyRunes("b"))
		s.panel = ""
	}
}

// itemName resolves an item id to its display name, falling back to the id so
// an unknown id is still legible rather than blank.
func itemName(id string) string {
	if it, ok := game.ItemByID(id); ok {
		return it.Name
	}
	return id
}

// rgbaHex renders an HD chat line's color as CSS hex, so the browser log keeps
// the same color language as the terminal one.
func rgbaHex(c color.RGBA) string {
	if c.A == 0 {
		return ""
	}
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}
