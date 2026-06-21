package game

import (
	"fmt"
	"math"
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

	// Intro cinematic timing. The title holds with an animated gradient
	// sweep, then the camera pans straight down onto the play field.
	introFrame = 70 * time.Millisecond
	introHold  = 15 // frames the title screen holds (~1.0s)
	introPan   = 16 // frames spent panning down onto the field (~1.1s)
)

type phase int

const (
	phaseIntro phase = iota
	phasePlay
	phaseTransition
)

// banner is the DURST WORLD wordmark, colored live during the intro.
var banner = []string{
	` ____  _   _ ____  ____ _____  __        _____  ____  _     ____`,
	`|  _ \| | | |  _ \/ ___|_   _| \ \      / / _ \|  _ \| |   |  _ \`,
	`| | | | | | | |_) \___ \ | |    \ \ /\ / / | | | |_) | |   | | | |`,
	`| |_| | |_| |  _ < ___) || |     \ V  V /| |_| |  _ <| |___| |_| |`,
	`|____/ \___/|_| \_\____/ |_|      \_/\_/  \___/|_| \_\_____|____/`,
}

type introTickMsg struct{}
type shimmerTickMsg struct{}

// Model is the root bubbletea model for one SSH session.
type Model struct {
	ctx    *Ctx
	theme  *ui.Theme
	events <-chan world.Event
	visit  store.VisitInfo

	width, height int
	phase         phase
	pulse         bool

	areaID string
	area   Area

	// intro cinematic
	introStep int

	// transition shimmer
	pendingArea string
	shimmerStep int

	// chat
	chatInput ui.TextInput
	chatLog   []string // pre-rendered lines, newest last

	// overlays / global state
	showPlayers bool
	showInfo    bool // generic info panel (/help, /who)
	infoTitle   string
	infoLines   []string
	showChar    bool // interactive character panel (/character)
	charField   int  // selected field in the character panel: 0 style, 1 color, 2 hat
	quitArmed   bool
}

// NewModel wires a session model. The player is already Joined to the
// world (no area yet); visit info is already recorded.
func NewModel(ctx *Ctx, events <-chan world.Event, visit store.VisitInfo) *Model {
	th := ctx.Theme
	if th == nil {
		th = ui.Default
	}
	return &Model{
		ctx:       ctx,
		theme:     th,
		events:    events,
		visit:     visit,
		areaID:    "wilds", // the Wilds is the spawn hub
		chatInput: ui.NewTextInput("say: ", chatMax),
	}
}

func (m *Model) Init() tea.Cmd {
	// Build the lobby up front so the intro can pan onto the real field, but
	// don't place/announce the player until the cinematic lands (beginPlay).
	m.area = NewArea(m.areaID, m.ctx)
	return tea.Batch(
		WaitForEvent(m.events),
		tea.Tick(introFrame, func(time.Time) tea.Msg { return introTickMsg{} }),
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

	case introTickMsg:
		if m.phase != phaseIntro {
			return m, nil
		}
		m.introStep++
		if m.introStep >= introHold+introPan {
			return m, m.beginPlay()
		}
		return m, tea.Tick(introFrame, func(time.Time) tea.Msg { return introTickMsg{} })

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

// beginPlay lands the cinematic: place the player in the (already-built)
// lobby, announce them to the world, and start play.
func (m *Model) beginPlay() tea.Cmd {
	m.phase = phasePlay
	if m.area == nil {
		m.area = NewArea(m.areaID, m.ctx)
	}
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
		name := m.theme.ChatName.Foreground(ui.AvatarColor(ev.Player)).Render(ev.Player)
		m.addChatLine(name + m.theme.ChatText.Render(": "+ev.Detail))
	case world.EventEmote:
		m.addChatLine(m.theme.Accent.Render("✱ "+ev.Player+" ") + m.theme.ChatText.Italic(true).Render(ev.Detail))
	case world.EventWhisper:
		from := m.theme.ChatName.Foreground(ui.AvatarColor(ev.Player)).Render(ev.Player)
		m.addChatLine(from + m.theme.Accent.Render(" whispers: ") + m.theme.ChatText.Render(ev.Detail))
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
	m.addChatLine(m.theme.Toast.Render(text))
}

// addSystemLine adds local-only command feedback (not sent to the world).
func (m *Model) addSystemLine(text string) {
	m.addChatLine(m.theme.Dim.Render("» ") + m.theme.ChatText.Render(text))
}

// showInfoPanel pops a dismissable overlay with pre-rendered lines (/help,
// /who). Any key closes it.
func (m *Model) showInfoPanel(title string, lines []string) {
	m.infoTitle = title
	m.infoLines = lines
	m.showInfo = true
}

// showHelp opens the "?" overlay: the full control reference (keys + what they
// do) followed by the chat commands, both drawn from the shared source of truth
// in controls.go so it can never fall out of step with what the game accepts.
func (m *Model) showHelp() {
	var lines []string
	for _, g := range Controls() {
		lines = append(lines, m.theme.PanelTitle.Render(g.Title))
		for _, c := range g.Items {
			lines = append(lines, fmt.Sprintf("  %s %s",
				m.theme.Bright.Render(padRight(c.Keys, 14)),
				m.theme.ChatText.Render(c.Desc)))
		}
		lines = append(lines, "")
	}
	lines = append(lines, m.theme.PanelTitle.Render("Chat commands"))
	for _, c := range CommandReference() {
		lines = append(lines, fmt.Sprintf("  %s %s",
			m.theme.Accent.Render(padRight(c[0], 18)),
			m.theme.ChatText.Render(c[1])))
	}
	m.showInfoPanel("Help", lines)
}

// handleCharKey drives the interactive character panel: ↑↓ pick a field, ←→
// change it live (persisting the avatar), esc/enter/q close.
func (m *Model) handleCharKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "enter", "q":
		m.showChar = false
	case "up", "k":
		m.charField = (m.charField + CharFields - 1) % CharFields
	case "down", "j", "tab":
		m.charField = (m.charField + 1) % CharFields
	case "left", "h":
		CycleAvatarField(m.ctx, m.charField, -1)
	case "right", "l":
		CycleAvatarField(m.ctx, m.charField, 1)
	}
	return nil
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

	if m.phase == phaseIntro {
		// any key skips the cinematic straight into play
		return m, m.beginPlay()
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
				return m, m.runChatLine(text)
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

	if m.showInfo {
		// any key dismisses the info panel and is otherwise swallowed
		m.showInfo = false
		return m, nil
	}

	if m.showChar {
		return m, m.handleCharKey(msg)
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
	case "?":
		m.showHelp()
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
			m.theme.Warn.Render("Please enlarge your terminal")+"\n\n"+
				m.theme.Dim.Render(fmt.Sprintf("Durst World needs at least %d×%d (you have %d×%d)",
					MinWidth, MinHeight, m.width, m.height)),
			m.width, m.height)
	}

	switch m.phase {
	case phaseIntro:
		return m.introView()
	case phaseTransition:
		return m.shimmerView()
	}
	return m.playView()
}

// introView renders the cinematic: an animated DURST WORLD title that, after
// a short hold, the camera pans straight down off — revealing the play field
// underneath. The canvas is [title screen] stacked on [play field]; a window
// of screen height slides from the top of the title to the top of the field.
func (m *Model) introView() string {
	H := m.height
	canvas := append(padLines(m.titleScreen(), H), padLines(m.playView(), H)...)

	off := 0
	if m.introStep >= introHold {
		p := float64(m.introStep-introHold) / float64(introPan)
		if p > 1 {
			p = 1
		}
		ease := p * p * (3 - 2*p) // smoothstep
		off = int(ease*float64(H) + 0.5)
		if off > H {
			off = H
		}
	}
	return strings.Join(canvas[off:off+H], "\n")
}

// titleScreen is the full-screen DURST WORLD card: a live gradient sweep over
// the wordmark, a tagline, an animated half-block energy bar, and a hint.
func (m *Model) titleScreen() string {
	phase := float64(m.introStep) * 0.08
	big := m.theme.Shimmer(banner, ui.HexAccent, ui.HexAccent2, phase)

	bw := len([]rune(banner[0]))
	wave := buildWave(m.theme, bw, m.introStep)

	tagline := m.theme.Dim.Render("a living terminal world · Durst HQ")
	hint := m.theme.Faint.Render("press any key to enter")

	body := big + "\n\n" + center(tagline, bw) + "\n" + wave + "\n\n" + center(hint, bw)
	return ui.Center(body, m.width, m.height)
}

// buildWave draws a 2-row half-block sine bar that ripples with step — a
// small showcase of the sub-character pixel layer.
func buildWave(th *ui.Theme, width, step int) string {
	const rows = 4 // 4 pixels tall → 2 text rows
	pix := make([][]lipgloss.Color, rows)
	for r := range pix {
		pix[r] = make([]lipgloss.Color, width)
		for x := range pix[r] {
			pix[r][x] = ui.Transparent
		}
	}
	for x := 0; x < width; x++ {
		s := math.Sin(float64(x)*0.22 + float64(step)*0.35)
		h := int((s*0.5+0.5)*float64(rows-1) + 0.5) // crest height 0..rows-1
		for r := rows - 1; r >= rows-1-h; r-- {
			f := float64(rows-1-r) / float64(rows-1)
			pix[r][x] = ui.Blend(ui.HexAccent, ui.HexAccent2, f)
		}
	}
	return th.HalfBlock(pix)
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
			b.WriteString(m.theme.Dim.Render(row))
		} else {
			b.WriteString(m.theme.Accent.Render(row))
		}
	}
	return b.String()
}

func (m *Model) playView() string {
	title := m.theme.Title.Render("DURST WORLD") +
		m.theme.Status.Render(" "+m.area.Name())
	titleBar := m.theme.Wrap(m.width).Render(title)

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

	if m.showInfo {
		panel := m.infoPanel()
		pw := lipgloss.Width(panel)
		ph := lipgloss.Height(panel)
		view = ui.Overlay(view, panel, (m.width-pw)/2, (m.height-ph)/2)
	} else if m.showChar {
		panel := m.charPanel()
		pw := lipgloss.Width(panel)
		ph := lipgloss.Height(panel)
		view = ui.Overlay(view, panel, (m.width-pw)/2, (m.height-ph)/2)
	} else if m.showPlayers {
		panel := m.playerListPanel()
		pw := lipgloss.Width(panel)
		ph := lipgloss.Height(panel)
		view = ui.Overlay(view, panel, (m.width-pw)/2, (m.height-ph)/2)
	}
	return view
}

// charPanel renders the interactive character editor: a live avatar preview
// over three cycleable fields (style, color, hat). The hat row hints how to
// unlock more when the player only has the default.
func (m *Model) charPanel() string {
	cur, ok := m.ctx.World.Self(m.ctx.Name)
	if !ok {
		return ""
	}
	rows := []string{m.theme.PanelTitle.Render("Character"), ""}
	for _, line := range AvatarPreview(m.theme, cur.Style, cur.Accessory, cur.Color) {
		rows = append(rows, "   "+line)
	}
	rows = append(rows, "")

	hat := AccessoryName(cur.Accessory)
	if len(OwnedHats(m.ctx)) == 1 {
		hat += m.theme.Dim.Render("  (find hats in the Wilds)")
	}
	fields := []struct{ label, val string }{
		{"Style", AvatarStyleName(cur.Style)},
		{"Color", fmt.Sprintf("#%d", ui.AvatarColorIndex(cur.Color))},
		{"Hat", hat},
	}
	for i, f := range fields {
		label := m.theme.Dim.Render(padRight(f.label, 6))
		if i == m.charField {
			rows = append(rows, fmt.Sprintf("%s %s %s",
				m.theme.Accent.Render("►"), label,
				m.theme.Bright.Render("◄ "+f.val+" ►")))
		} else {
			rows = append(rows, fmt.Sprintf("  %s %s", label, m.theme.ChatText.Render(f.val)))
		}
	}
	rows = append(rows, "", m.theme.Dim.Render("↑↓ field · ←→ change · esc close"))
	return m.theme.Panel.Render(strings.Join(rows, "\n"))
}

func (m *Model) infoPanel() string {
	var rows []string
	rows = append(rows, m.theme.PanelTitle.Render(m.infoTitle), "")
	if len(m.infoLines) == 0 {
		rows = append(rows, m.theme.Dim.Render("(nothing to show)"))
	} else {
		rows = append(rows, m.infoLines...)
	}
	rows = append(rows, "", m.theme.Dim.Render("any key to close"))
	return m.theme.Panel.Render(strings.Join(rows, "\n"))
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
			m.theme.Dim.Render("   (Enter send · Esc cancel · /help for commands)")
		return m.theme.Wrap(m.width).Render(bar)
	}

	here := len(m.ctx.World.PlayersInArea(m.areaID))
	online := len(m.ctx.World.AllPlayers())

	parts := []string{
		m.theme.StatusHint.Render(" " + m.area.Name() + " "),
		m.theme.Status.Render(fmt.Sprintf("%d here · %d online", here, online)),
	}
	if m.quitArmed {
		parts = append(parts, m.theme.Warn.Background(ui.ColorBarBg).Render(" Press q again to leave Durst World "))
	} else {
		if h, ok := m.area.(Hinter); ok {
			if hint := h.Hint(); hint != "" {
				parts = append(parts, m.theme.StatusHint.Render(" "+hint+" "))
			}
		}
		parts = append(parts, m.theme.Status.Render("WASD/↑↓←→ move · Enter chat · Tab players · ? help · q quit"))
	}
	bar := strings.Join(parts, m.theme.Status.Render("·"))
	return m.theme.Bar(m.width).Render(bar)
}

func (m *Model) playerListPanel() string {
	players := m.ctx.World.AllPlayers()
	var rows []string
	rows = append(rows, m.theme.PanelTitle.Render(fmt.Sprintf("Online — %d", len(players))))
	rows = append(rows, "")
	for _, p := range players {
		area := DisplayName(p.Area)
		if p.Area == "" {
			area = "connecting…"
		}
		name := m.theme.ChatName.Foreground(p.Color).Render(p.Name)
		if p.Name == m.ctx.Name {
			name += m.theme.Dim.Render(" (you)")
		}
		rows = append(rows, fmt.Sprintf("%s %s", name, m.theme.Dim.Render("— "+area)))
	}
	rows = append(rows, "")
	rows = append(rows, m.theme.Dim.Render("Tab to close"))
	return m.theme.Panel.Render(strings.Join(rows, "\n"))
}

// padLines splits s into exactly n lines, padding with blanks or truncating.
func padLines(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		return lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

// center pads a single (possibly styled) line to sit centered in width w.
func center(s string, w int) string {
	pad := (w - lipgloss.Width(s)) / 2
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad) + s
}
