package game

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

const (
	MinWidth  = 80
	MinHeight = 24

	chatLogLines = 4
	chatMax      = 120
)

type phase int

const (
	phaseBoot phase = iota
	phasePlay
	phaseTransition
)

var bootLines = []string{
	"establishing uplink to BRIXEN HQ…",
	"calibrating printheads… ok",
	"warming up the lobby espresso machine… ok",
	"loading world…",
}

var banner = []string{
	` ____  _   _ ____  ____ _____  __        _____  ____  _     ____`,
	`|  _ \| | | |  _ \/ ___|_   _| \ \      / / _ \|  _ \| |   |  _ \`,
	`| | | | | | | |_) \___ \ | |    \ \ /\ / / | | | |_) | |   | | | |`,
	`| |_| | |_| |  _ < ___) || |     \ V  V /| |_| |  _ <| |___| |_| |`,
	`|____/ \___/|_| \_\____/ |_|      \_/\_/  \___/|_| \_\_____|____/`,
}

type bootTickMsg struct{}
type shimmerTickMsg struct{}

// Model is the root bubbletea model for one SSH session.
type Model struct {
	ctx    *Ctx
	events <-chan world.Event
	visit  store.VisitInfo

	width, height int
	phase         phase
	pulse         bool

	areaID string
	area   Area

	// boot
	bootStep int

	// transition shimmer
	pendingArea string
	shimmerStep int

	// chat
	chatInput ui.TextInput
	chatLog   []string // pre-rendered lines, newest last

	// overlays / global state
	showPlayers bool
	quitArmed   bool
}

// NewModel wires a session model. The player is already Joined to the
// world (no area yet); visit info is already recorded.
func NewModel(ctx *Ctx, events <-chan world.Event, visit store.VisitInfo) *Model {
	return &Model{
		ctx:       ctx,
		events:    events,
		visit:     visit,
		areaID:    "lobby",
		chatInput: ui.NewTextInput("say: ", chatMax),
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		WaitForEvent(m.events),
		tea.Tick(220*time.Millisecond, func(time.Time) tea.Msg { return bootTickMsg{} }),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case EventsClosedMsg:
		return m, tea.Quit

	case WorldEventMsg:
		cmd := m.handleWorldEvent(world.Event(msg))
		return m, tea.Batch(WaitForEvent(m.events), cmd)

	case bootTickMsg:
		if m.phase != phaseBoot {
			return m, nil
		}
		m.bootStep++
		if m.bootStep >= len(bootLines)+3 {
			return m, m.enterWorld()
		}
		return m, tea.Tick(220*time.Millisecond, func(time.Time) tea.Msg { return bootTickMsg{} })

	case shimmerTickMsg:
		if m.phase != phaseTransition {
			return m, nil
		}
		m.shimmerStep++
		if m.shimmerStep >= 5 {
			return m, m.finishTransition()
		}
		return m, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return shimmerTickMsg{} })

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// anything else (area-internal ticks etc.) goes to the area
	if m.area != nil {
		return m, m.updateArea(msg)
	}
	return m, nil
}

// enterWorld finishes the boot sequence: build the lobby and drop in.
func (m *Model) enterWorld() tea.Cmd {
	m.phase = phasePlay
	m.area = NewArea(m.areaID, m.ctx)
	m.addToast(m.welcomeLine())
	m.ctx.Store.RecordAreaVisit(m.ctx.Name, m.areaID)
	m.ctx.Store.LogEvent(m.ctx.Name, "join", m.areaID)
	self, _ := m.ctx.World.Self(m.ctx.Name)
	return m.area.Init(&self)
}

func (m *Model) welcomeLine() string {
	if m.visit.FirstVisit {
		return fmt.Sprintf("Welcome to Durst World, %s.", m.ctx.Name)
	}
	return fmt.Sprintf("Welcome back, %s — visit #%d. Last seen %s.",
		m.ctx.Name, m.visit.VisitCount, humanSince(m.visit.LastSeen))
}

func humanSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 2*time.Minute:
		return "moments ago"
	case d < 2*time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// handleWorldEvent updates global state (chat log, toasts, pulse) and
// forwards the event to the active area for its own bookkeeping.
func (m *Model) handleWorldEvent(ev world.Event) tea.Cmd {
	switch ev.Type {
	case world.EventTick:
		m.pulse = ev.Pulse
	case world.EventChat:
		name := ui.ChatNameStyle.Foreground(ui.AvatarColor(ev.Player)).Render(ev.Player)
		m.addChatLine(name + ui.ChatTextStyle.Render(": "+ev.Detail))
	case world.EventJoined:
		if ev.Player != m.ctx.Name && m.area != nil {
			m.addToast(fmt.Sprintf("· %s entered the %s ·", ev.Player, m.area.Name()))
		}
	case world.EventLeft:
		if ev.Player != m.ctx.Name {
			if ev.Detail != "" {
				m.addToast(fmt.Sprintf("· %s headed to %s ·", ev.Player, ev.Detail))
			} else {
				m.addToast(fmt.Sprintf("· %s left ·", ev.Player))
			}
		}
	}
	if m.area != nil {
		return m.updateArea(WorldEventMsg(ev))
	}
	return nil
}

func (m *Model) addChatLine(rendered string) {
	m.chatLog = append(m.chatLog, rendered)
	if len(m.chatLog) > chatLogLines {
		m.chatLog = m.chatLog[len(m.chatLog)-chatLogLines:]
	}
}

func (m *Model) addToast(text string) {
	m.addChatLine(ui.ToastStyle.Render(text))
}

// updateArea runs the area's Update and handles a possible transition.
func (m *Model) updateArea(msg tea.Msg) tea.Cmd {
	next, cmd := m.area.Update(msg)
	if t, ok := next.(Transition); ok {
		return tea.Batch(cmd, m.startTransition(t.To))
	}
	m.area = next
	return cmd
}

func (m *Model) startTransition(dest string) tea.Cmd {
	m.ctx.Store.LogEvent(m.ctx.Name, "transition", m.areaID+" → "+dest)
	m.ctx.Store.RecordAreaVisit(m.ctx.Name, dest)
	m.pendingArea = dest
	m.phase = phaseTransition
	m.shimmerStep = 0
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return shimmerTickMsg{} })
}

func (m *Model) finishTransition() tea.Cmd {
	m.ctx.From = m.areaID
	m.areaID = m.pendingArea
	m.pendingArea = ""
	m.area = NewArea(m.areaID, m.ctx)
	m.phase = phasePlay
	self, _ := m.ctx.World.Self(m.ctx.Name)
	return m.area.Init(&self)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always works, with the same confirm as q.
	if msg.Type == tea.KeyCtrlC {
		if m.quitArmed {
			return m, tea.Quit
		}
		m.quitArmed = true
		return m, nil
	}

	if m.phase == phaseBoot {
		// any key skips the boot sequence
		return m, m.enterWorld()
	}
	if m.phase == phaseTransition {
		return m, nil
	}

	// chat input has priority
	if m.chatInput.Focused() {
		switch msg.Type {
		case tea.KeyEnter:
			text := strings.TrimSpace(m.chatInput.Value)
			m.chatInput.Blur()
			if text != "" {
				m.ctx.World.Chat(m.ctx.Name, text)
			}
		case tea.KeyEsc:
			m.chatInput.Blur()
		default:
			m.chatInput.HandleKey(msg)
		}
		return m, nil
	}

	// areas with an open panel (guestbook) grab everything
	if cap, ok := m.area.(InputCapturer); ok && cap.CapturesInput() {
		return m, m.updateArea(msg)
	}

	if m.showPlayers {
		m.showPlayers = false
		if msg.Type == tea.KeyTab {
			return m, nil
		}
		// fall through: any other key both closes the overlay and acts
	}

	armed := m.quitArmed
	m.quitArmed = false

	switch msg.String() {
	case "q":
		if armed {
			return m, tea.Quit
		}
		m.quitArmed = true
		return m, nil
	case "tab":
		m.showPlayers = !m.showPlayers
		return m, nil
	case "enter":
		m.chatInput.Focus()
		return m, nil
	}

	return m, m.updateArea(msg)
}

// ─── View ────────────────────────────────────────────────────────────────

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < MinWidth || m.height < MinHeight {
		return ui.Center(
			ui.WarnStyle.Render("Please enlarge your terminal")+"\n\n"+
				ui.DimStyle.Render(fmt.Sprintf("Durst World needs at least %d×%d (you have %d×%d)",
					MinWidth, MinHeight, m.width, m.height)),
			m.width, m.height)
	}

	switch m.phase {
	case phaseBoot:
		return m.bootView()
	case phaseTransition:
		return m.shimmerView()
	}
	return m.playView()
}

func (m *Model) bootView() string {
	var b strings.Builder
	n := m.bootStep
	if n > len(bootLines) {
		n = len(bootLines)
	}
	for i := 0; i < n; i++ {
		b.WriteString(ui.DimStyle.Render("▸ "+bootLines[i]) + "\n")
	}
	if m.bootStep >= len(bootLines) {
		fade := m.bootStep - len(bootLines) // 0,1,2
		var st lipgloss.Style
		switch fade {
		case 0:
			st = ui.FaintStyle
		case 1:
			st = ui.DimStyle
		default:
			st = lipgloss.NewStyle().Foreground(ui.ColorAccent).Bold(true)
		}
		b.WriteString("\n")
		for _, l := range banner {
			b.WriteString(st.Render(l) + "\n")
		}
		b.WriteString("\n" + ui.LabelStyle.Render(m.welcomeLine()) + "\n")
		b.WriteString(ui.DimStyle.Render("move WASD/arrows · Enter chat · Tab who's online · q quit"))
	}
	return ui.Center(b.String(), m.width, m.height)
}

func (m *Model) shimmerView() string {
	runes := []rune{'░', '▒', '▓', '▒', '░'}
	r := string(runes[m.shimmerStep%len(runes)])
	row := strings.Repeat(r, m.width)
	var b strings.Builder
	for i := 0; i < m.height; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		if (i+m.shimmerStep)%2 == 0 {
			b.WriteString(ui.DimStyle.Render(row))
		} else {
			b.WriteString(ui.AccentStyle.Render(row))
		}
	}
	return b.String()
}

func (m *Model) playView() string {
	title := ui.TitleStyle.Render("DURST WORLD") +
		ui.StatusStyle.Render(" "+m.area.Name())
	titleBar := lipgloss.NewStyle().Width(m.width).Render(title)

	mapHeight := m.height - 1 - chatLogLines - 1 // title, chat, status
	areaView := m.area.View(m.width, mapHeight)
	areaBlock := ui.Center(areaView, m.width, mapHeight)
	// Center returns exactly mapHeight lines; clamp in case the area is tall
	if lines := strings.Split(areaBlock, "\n"); len(lines) > mapHeight {
		areaBlock = strings.Join(lines[:mapHeight], "\n")
	}

	chat := m.chatView()
	status := m.statusView()

	view := titleBar + "\n" + areaBlock + "\n" + chat + "\n" + status

	if m.showPlayers {
		panel := m.playerListPanel()
		pw := lipgloss.Width(panel)
		ph := lipgloss.Height(panel)
		view = ui.Overlay(view, panel, (m.width-pw)/2, (m.height-ph)/2)
	}
	return view
}

func (m *Model) chatView() string {
	lines := make([]string, chatLogLines)
	pad := chatLogLines - len(m.chatLog)
	for i, l := range m.chatLog {
		lines[pad+i] = " " + l
	}
	return strings.Join(lines, "\n")
}

func (m *Model) statusView() string {
	if m.chatInput.Focused() {
		bar := " " + m.chatInput.View() +
			ui.DimStyle.Render("   (Enter send · Esc cancel · heard within 8 tiles)")
		return lipgloss.NewStyle().Width(m.width).Render(bar)
	}

	here := len(m.ctx.World.PlayersInArea(m.areaID))
	online := len(m.ctx.World.AllPlayers())

	parts := []string{
		ui.StatusHintStyle.Render(" " + m.area.Name() + " "),
		ui.StatusStyle.Render(fmt.Sprintf("%d here · %d online", here, online)),
	}
	if m.quitArmed {
		parts = append(parts, ui.WarnStyle.Background(ui.ColorBarBg).Render(" Press q again to leave Durst World "))
	} else {
		if h, ok := m.area.(Hinter); ok {
			if hint := h.Hint(); hint != "" {
				parts = append(parts, ui.StatusHintStyle.Render(" "+hint+" "))
			}
		}
		parts = append(parts, ui.StatusStyle.Render("move WASD/↑↓←→ · Enter chat · Tab players · q quit"))
	}
	bar := strings.Join(parts, ui.StatusStyle.Render("·"))
	return lipgloss.NewStyle().Width(m.width).Background(ui.ColorBarBg).Render(bar)
}

func (m *Model) playerListPanel() string {
	players := m.ctx.World.AllPlayers()
	var rows []string
	rows = append(rows, ui.PanelTitleStyle.Render(fmt.Sprintf("Online — %d", len(players))))
	rows = append(rows, "")
	for _, p := range players {
		area := DisplayName(p.Area)
		if p.Area == "" {
			area = "connecting…"
		}
		name := ui.ChatNameStyle.Foreground(p.Color).Render(p.Name)
		if p.Name == m.ctx.Name {
			name += ui.DimStyle.Render(" (you)")
		}
		rows = append(rows, fmt.Sprintf("%s %s", name, ui.DimStyle.Render("— "+area)))
	}
	rows = append(rows, "")
	rows = append(rows, ui.DimStyle.Render("Tab to close"))
	return ui.PanelStyle.Render(strings.Join(rows, "\n"))
}
