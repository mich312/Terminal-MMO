package game

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// command is one slash-command. run may produce local feedback (chat lines or
// an info panel on the Model) and/or return a tea.Cmd (e.g. a teleport).
type command struct {
	name    string
	aliases []string
	usage   string
	summary string
	run     func(m *Model, args []string) tea.Cmd
}

// commands and commandIndex are populated in init() rather than as var
// initializers to avoid a static initialization cycle (commands references
// cmdHelp, which references commands).
var commands []*command
var commandIndex map[string]*command

func init() {
	commands = []*command{
		{
			name: "help", usage: "/help [command]",
			summary: "list commands, or show one command's usage",
			run:     cmdHelp,
		},
		{
			name: "who", usage: "/who",
			summary: "who's online and where",
			run:     cmdWho,
		},
		{
			name: "where", usage: "/where",
			summary: "your current area and coordinates",
			run:     cmdWhere,
		},
		{
			name: "me", usage: "/me <action>",
			summary: "emote an action to those nearby",
			run:     cmdMe,
		},
		{
			name: "whisper", aliases: []string{"w", "tell", "msg"}, usage: "/w <name> <message>",
			summary: "send a private message to one player",
			run:     cmdWhisper,
		},
		{
			name: "roll", usage: "/roll [NdM]",
			summary: "roll dice for everyone nearby (default 1d6)",
			run:     cmdRoll,
		},
		{
			name: "color", aliases: []string{"colour"}, usage: "/color [0-21]",
			summary: "change your avatar color",
			run:     cmdColor,
		},
		{
			name: "avatar", aliases: []string{"av"}, usage: "/avatar [style] [hat]",
			summary: "change your sprite style and accessory",
			run:     cmdAvatar,
		},
		{
			name: "goto", aliases: []string{"go"}, usage: "/goto <area>",
			summary: "teleport to an area",
			run:     cmdGoto,
		},
		{
			name: "inventory", aliases: []string{"i", "inv"}, usage: "/inventory",
			summary: "show the items you've collected",
			run:     cmdInventory,
		},
		{
			name: "character", aliases: []string{"char"}, usage: "/character",
			summary: "preview and customize your avatar",
			run:     cmdCharacter,
		},
		{
			name: "craft", aliases: []string{"k"}, usage: "/craft",
			summary: "open the workbench and craft from your pack",
			run:     cmdCraft,
		},
		{
			name: "sell", usage: "/sell <n> <item> for <m> <item>",
			summary: "post an offer at your Concession (stocks it from your pack)",
			run:     cmdSell,
		},
		{
			name: "collect", usage: "/collect",
			summary: "sweep your Concession's till into your pack",
			run:     cmdCollect,
		},
		{
			name: "clear", usage: "/clear",
			summary: "clear your chat log",
			run:     cmdClear,
		},
	}

	commandIndex = map[string]*command{}
	for _, c := range commands {
		commandIndex[c.name] = c
		for _, a := range c.aliases {
			commandIndex[a] = c
		}
	}
}

// runChatLine routes a submitted line: a leading "/" runs a command, anything
// else is proximity chat. Returns a tea.Cmd for commands that need one.
func (m *Model) runChatLine(text string) tea.Cmd {
	if !strings.HasPrefix(text, "/") {
		m.ctx.World.Chat(m.ctx.Name, text)
		return nil
	}
	fields := strings.Fields(text[1:])
	if len(fields) == 0 {
		return nil
	}
	name := strings.ToLower(fields[0])
	cmd, ok := commandIndex[name]
	if !ok {
		m.addSystemLine(fmt.Sprintf("unknown command /%s — try /help", name))
		return nil
	}
	return cmd.run(m, fields[1:])
}

func cmdHelp(m *Model, args []string) tea.Cmd {
	if len(args) > 0 {
		if c, ok := commandIndex[strings.ToLower(strings.TrimPrefix(args[0], "/"))]; ok {
			m.addSystemLine(c.usage + " — " + c.summary)
		} else {
			m.addSystemLine("no such command: /" + args[0])
		}
		return nil
	}
	lines := make([]string, 0, len(commands))
	for _, c := range commands {
		lines = append(lines, fmt.Sprintf("%s  %s",
			m.theme.Accent.Render(padRight(c.usage, 20)),
			m.theme.ChatText.Render(c.summary)))
	}
	m.showInfoPanel("Commands", lines)
	return nil
}

func cmdWho(m *Model, args []string) tea.Cmd {
	players := m.ctx.World.AllPlayers()
	sort.Slice(players, func(i, j int) bool { return players[i].Name < players[j].Name })
	lines := make([]string, 0, len(players))
	for _, p := range players {
		area := DisplayName(p.Area)
		if p.Area == "" {
			area = "connecting…"
		}
		name := m.theme.ChatName.Foreground(p.Color).Render(p.Name)
		if p.Name == m.ctx.Name {
			name += m.theme.Dim.Render(" (you)")
		}
		lines = append(lines, fmt.Sprintf("%s %s", name, m.theme.Dim.Render("— "+area)))
	}
	m.showInfoPanel(fmt.Sprintf("Online — %d", len(players)), lines)
	return nil
}

func cmdWhere(m *Model, args []string) tea.Cmd {
	self, ok := m.ctx.World.Self(m.ctx.Name)
	if !ok {
		return nil
	}
	m.addSystemLine(fmt.Sprintf("%s · (%d, %d)", DisplayName(self.Area), self.X, self.Y))
	return nil
}

func cmdMe(m *Model, args []string) tea.Cmd {
	if len(args) == 0 {
		m.addSystemLine("usage: /me <action>")
		return nil
	}
	m.ctx.World.Emote(m.ctx.Name, strings.Join(args, " "))
	return nil
}

func cmdWhisper(m *Model, args []string) tea.Cmd {
	if len(args) < 2 {
		m.addSystemLine("usage: /w <name> <message>")
		return nil
	}
	to := args[0]
	text := strings.Join(args[1:], " ")
	if m.ctx.World.Whisper(m.ctx.Name, to, text) {
		m.addChatLine(m.theme.Accent.Render("→ "+to+": ") + m.theme.ChatText.Render(text))
	} else {
		m.addSystemLine(to + " is not online.")
	}
	return nil
}

func cmdRoll(m *Model, args []string) tea.Cmd {
	spec := "1d6"
	if len(args) > 0 {
		spec = args[0]
	}
	n, sides, ok := parseDice(spec)
	if !ok {
		m.addSystemLine("usage: /roll [NdM] (e.g. 2d6, d20)")
		return nil
	}
	rolls := make([]string, n)
	total := 0
	for i := 0; i < n; i++ {
		r := rand.Intn(sides) + 1
		total += r
		rolls[i] = strconv.Itoa(r)
	}
	detail := strconv.Itoa(total)
	if n > 1 {
		detail = strings.Join(rolls, "+") + " = " + strconv.Itoa(total)
	}
	m.ctx.World.Emote(m.ctx.Name, fmt.Sprintf("rolls %dd%d: %s", n, sides, detail))
	return nil
}

func cmdColor(m *Model, args []string) tea.Cmd {
	var idx int
	if len(args) > 0 {
		i, err := strconv.Atoi(args[0])
		if err != nil || i < 0 || i >= ui.NumAvatarColors() {
			m.addSystemLine(fmt.Sprintf("pick a color 0–%d", ui.NumAvatarColors()-1))
			return nil
		}
		idx = i
	} else {
		idx = rand.Intn(ui.NumAvatarColors())
	}
	if m.ctx.World.SetColor(m.ctx.Name, ui.AvatarColorByIndex(idx)) {
		m.persistAvatar()
		m.addSystemLine(fmt.Sprintf("avatar color set to #%d", idx))
	}
	return nil
}

func cmdAvatar(m *Model, args []string) tea.Cmd {
	cur, ok := m.ctx.World.Self(m.ctx.Name)
	if !ok {
		return nil
	}
	if len(args) == 0 {
		m.addSystemLine("styles: " + listIndexed(NumAvatarStyles(), AvatarStyleName))
		m.addSystemLine("hats:   " + ownedHats(m))
		for i := 1; i < NumAccessories(); i++ {
			if m.ctx.Hats[i] {
				if p := AccessoryPower(i); p != "" {
					m.addSystemLine(fmt.Sprintf("  %s — %s", AccessoryName(i), p))
				}
			}
		}
		m.addSystemLine(fmt.Sprintf("you: %s + %s — usage: /avatar <style> [hat]  · /character to preview",
			AvatarStyleName(cur.Style), AccessoryName(cur.Accessory)))
		return nil
	}
	style := resolveIndex(args[0], cur.Style, NumAvatarStyles(), AvatarStyleName)
	acc := cur.Accessory
	if len(args) > 1 {
		want := resolveIndex(args[1], cur.Accessory, NumAccessories(), AccessoryName)
		if want != 0 && !m.ctx.Hats[want] {
			m.addSystemLine("you haven't found the " + AccessoryName(want) + " yet — explore the Wilds to wear it")
			return nil
		}
		acc = want
	}
	if m.ctx.World.SetAvatar(m.ctx.Name, style, acc) {
		m.persistAvatar()
		line := fmt.Sprintf("avatar: %s + %s", AvatarStyleName(style), AccessoryName(acc))
		if p := AccessoryPower(acc); p != "" {
			line += " — " + p // tell them what the thing on their head now does
		}
		m.addSystemLine(line)
	}
	return nil
}

// persistAvatar saves the player's current color/style/accessory so it survives
// reconnects.
func (m *Model) persistAvatar() {
	if p, ok := m.ctx.World.Self(m.ctx.Name); ok {
		m.ctx.Store.SaveAvatar(m.ctx.Name, string(p.Color), p.Style, p.Accessory)
	}
}

// ownedHats lists the accessories the player has unlocked (0:none is always
// available), for the /avatar listing.
func ownedHats(m *Model) string {
	parts := []string{"0:none"}
	for i := 1; i < NumAccessories(); i++ {
		if m.ctx.Hats[i] {
			parts = append(parts, fmt.Sprintf("%d:%s", i, AccessoryName(i)))
		}
	}
	if len(parts) == 1 {
		return "none yet — find hats out in the Wilds"
	}
	return strings.Join(parts, "  ")
}

// listIndexed renders "0:name  1:name  …" for a command's options listing.
func listIndexed(n int, name func(int) string) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fmt.Sprintf("%d:%s", i, name(i))
	}
	return strings.Join(parts, "  ")
}

// resolveIndex accepts an index or a (case-insensitive) name; unknown values
// keep the fallback.
func resolveIndex(arg string, fallback, n int, name func(int) string) int {
	if i, err := strconv.Atoi(arg); err == nil && i >= 0 && i < n {
		return i
	}
	la := strings.ToLower(arg)
	for i := 0; i < n; i++ {
		if strings.ToLower(name(i)) == la {
			return i
		}
	}
	return fallback
}

func cmdGoto(m *Model, args []string) tea.Cmd {
	if len(args) == 0 {
		m.addSystemLine("usage: /goto <area> — try one of: " + strings.Join(RegisteredAreas(), ", "))
		return nil
	}
	dest := strings.ToLower(args[0])
	if dest == m.areaID {
		m.addSystemLine("you're already in the " + DisplayName(dest) + ".")
		return nil
	}
	if !AreaRegistered(dest) {
		m.addSystemLine("no such area: " + dest + " — try: " + strings.Join(RegisteredAreas(), ", "))
		return nil
	}
	if m.area != nil {
		if cap, ok := m.area.(InputCapturer); ok && cap.CapturesInput() {
			return nil
		}
	}
	return m.startTransition(dest)
}

func cmdInventory(m *Model, args []string) tea.Cmd {
	inv := m.ctx.Inventory
	total := 0
	lines := make([]string, 0, len(Items))
	for _, it := range Items {
		n := inv[it.ID]
		if n == 0 {
			continue
		}
		total += n
		glyph := m.theme.Fg(lipgloss.Color(it.Hex)).Render(string(it.Glyph))
		lines = append(lines, fmt.Sprintf("%s  %s %s",
			glyph, m.theme.ChatText.Render(padRight(it.Name, 14)),
			m.theme.Accent.Render(fmt.Sprintf("×%d", n))))
	}
	if total == 0 {
		m.addSystemLine("your pack is empty — explore the Wilds and press e on a ◆ to pick it up")
		return nil
	}
	m.showInfoPanel(fmt.Sprintf("Inventory — %d", total), lines)
	return nil
}

func cmdCharacter(m *Model, args []string) tea.Cmd {
	m.closePanels()
	m.showChar = true
	m.charField = 0
	return nil
}

func cmdCraft(m *Model, args []string) tea.Cmd {
	m.closePanels()
	m.showCraft = true
	m.craftSel = 0
	return nil
}

// cmdSell posts an offer at the Concession the owner is standing beside:
// "/sell 10 plank for 6 stone". Items are by id or display name (lowercased).
func cmdSell(m *Model, args []string) tea.Cmd {
	x, y, ok := StationNear(m.ctx, func(p world.Placement) bool {
		return IsStall(p.Kind) && p.Owner == m.ctx.Name
	})
	if !ok {
		m.addSystemLine("stand beside your own Concession to post an offer")
		return nil
	}
	giveN, give, askN, ask, ok := parseSell(args)
	if !ok {
		m.addSystemLine("usage: /sell <n> <item> for <m> <item>  (e.g. /sell 10 plank for 6 stone)")
		return nil
	}
	if n := AddOffer(m.ctx, x, y, give, giveN, ask, askN); n > 0 {
		m.addSystemLine(fmt.Sprintf("listed %d %s for %d %s (stocked %d)",
			giveN, itemName(give), askN, itemName(ask), n))
	} else {
		m.addSystemLine(fmt.Sprintf("can't list that — need at least %d %s in your pack", giveN, itemName(give)))
	}
	return nil
}

func cmdCollect(m *Model, args []string) tea.Cmd {
	x, y, ok := StationNear(m.ctx, func(p world.Placement) bool {
		return IsStall(p.Kind) && p.Owner == m.ctx.Name
	})
	if !ok {
		m.addSystemLine("stand beside your own Concession to collect the till")
		return nil
	}
	if n := CollectTill(m.ctx, x, y); n > 0 {
		m.addSystemLine(fmt.Sprintf("collected %d items from the till", n))
	} else {
		m.addSystemLine("the till is empty")
	}
	return nil
}

// parseSell reads "<n> <item> for <m> <item>" into its parts.
func parseSell(args []string) (giveN int, give string, askN int, ask string, ok bool) {
	// find the "for" separator
	fi := -1
	for i, a := range args {
		if strings.EqualFold(a, "for") {
			fi = i
			break
		}
	}
	if fi < 2 || fi+2 >= len(args)+1 || fi+3 != len(args) {
		return 0, "", 0, "", false
	}
	gn, err1 := strconv.Atoi(args[0])
	an, err2 := strconv.Atoi(args[fi+1])
	if err1 != nil || err2 != nil || gn <= 0 || an <= 0 {
		return 0, "", 0, "", false
	}
	give = resolveItem(strings.Join(args[1:fi], " "))
	ask = resolveItem(args[fi+2])
	if give == "" || ask == "" {
		return 0, "", 0, "", false
	}
	return gn, give, an, ask, true
}

// resolveItem maps an id or a (case-insensitive) display name to an item id.
func resolveItem(s string) string {
	s = strings.TrimSpace(s)
	if _, ok := ItemByID(strings.ToLower(s)); ok {
		return strings.ToLower(s)
	}
	for _, it := range Items {
		if strings.EqualFold(it.Name, s) || strings.EqualFold(it.ID, s) {
			return it.ID
		}
	}
	return ""
}

func itemName(id string) string {
	if it, ok := ItemByID(id); ok {
		return it.Name
	}
	return id
}

func cmdClear(m *Model, args []string) tea.Cmd {
	m.chatLog = nil
	return nil
}

// parseDice parses "NdM", "dM" (N=1) or "M" (1dM). Bounds keep things sane.
func parseDice(spec string) (n, sides int, ok bool) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	n = 1
	if i := strings.IndexByte(spec, 'd'); i >= 0 {
		if i > 0 {
			v, err := strconv.Atoi(spec[:i])
			if err != nil {
				return 0, 0, false
			}
			n = v
		}
		v, err := strconv.Atoi(spec[i+1:])
		if err != nil {
			return 0, 0, false
		}
		sides = v
	} else {
		v, err := strconv.Atoi(spec)
		if err != nil {
			return 0, 0, false
		}
		sides = v
	}
	if n < 1 || n > 20 || sides < 2 || sides > 1000 {
		return 0, 0, false
	}
	return n, sides, true
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
