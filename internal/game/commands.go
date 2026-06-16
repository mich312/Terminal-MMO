package game

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/durst-group/durstworld/internal/ui"
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
			name: "color", aliases: []string{"colour"}, usage: "/color [0-7]",
			summary: "change your avatar color",
			run:     cmdColor,
		},
		{
			name: "goto", aliases: []string{"go"}, usage: "/goto <area>",
			summary: "teleport to an area",
			run:     cmdGoto,
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
		m.addSystemLine(fmt.Sprintf("avatar color set to #%d", idx))
	}
	return nil
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
