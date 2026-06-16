package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"

	"github.com/durst-group/durstworld/internal/areas/wilds"
	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/pixel"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

// HD ("real pixel") mode is an experimental sixel renderer for the Wilds,
// reachable with:  ssh -t -p 2222 you@host hd
//
// It deliberately bypasses bubbletea — image escapes are out-of-band and would
// fight bubbletea's frame diffing — and writes sixel frames straight to the SSH
// session. It shares the live world (the same world.World, the same Wilds seed),
// so you and bubbletea players see each other. The point is to feel real SSH
// performance: latency, bandwidth, and framerate on an actual connection.
const (
	hdScale   = 12 // pixels per tile
	hdFPS     = 12
	hdRefresh = 48 // frames between full repaints
	hdMaxTile = 140
)

// setupAvatar restores a player's persisted color/style/accessory, or — on a
// first visit — rolls a random look and remembers it, so everyone spawns with a
// distinct avatar that then stays theirs across reconnects.
func setupAvatar(w *world.World, st store.Store, name string) {
	if color, style, accessory, ok := st.LoadAvatar(name); ok {
		if color != "" {
			w.SetColor(name, lipgloss.Color(color))
		}
		w.SetAvatar(name, style, accessory)
		return
	}
	color := ui.AvatarColorByIndex(rand.Intn(ui.NumAvatarColors()))
	style := rand.Intn(game.NumAvatarStyles())
	accessory := rand.Intn(game.NumAccessories())
	w.SetColor(name, color)
	w.SetAvatar(name, style, accessory)
	st.SaveAvatar(name, string(color), style, accessory)
}

// isHD reports whether a session asked for HD mode (an "hd" argument, e.g.
// `ssh -t host hd`).
func isHD(s ssh.Session) bool {
	for _, a := range s.Command() {
		if a == "hd" {
			return true
		}
	}
	return false
}

// hdMiddleware intercepts HD sessions and serves the sixel renderer instead of
// bubbletea; everything else falls through unchanged. Placed inside activeterm
// so a PTY is guaranteed.
func hdMiddleware(w *world.World, st store.Store) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			if isHD(s) {
				runHD(s, w, st)
				return
			}
			next(s)
		}
	}
}

func runHD(s ssh.Session, w *world.World, st store.Store) {
	ptyReq, winCh, ok := s.Pty()
	if !ok {
		fmt.Fprint(s, "HD mode needs a PTY — use: ssh -t -p 2222 you@host hd\r\n")
		return
	}

	name, events := w.Join(s.User())
	st.RecordVisit(name)
	setupAvatar(w, st, name)
	log.Printf("%s connected (HD/sixel)", name)
	defer func() {
		w.Leave(name)
		st.RecordDisconnect(name)
		log.Printf("%s disconnected (HD)", name)
	}()
	go func() { // HD polls the world each frame; drain presence events so senders never block
		for range events {
		}
	}()

	gen := worldgen.New(wilds.Seed)
	px, py := hdSpawn(gen)
	w.EnterArea(name, "wilds", px, py, "The Wilds")

	cellW, cellH := hdCellSize(ptyReq.Window)
	win := ptyReq.Window

	out := bufio.NewWriterSize(s, 1<<20)
	out.WriteString("\x1b[2J\x1b[?25l")
	defer func() { out.WriteString("\x1b[?25h\x1b[0m\r\n"); out.Flush() }()

	// Input runs on its own goroutine, emitting movement intents. Rendering is
	// driven only by the ticker (below), so a slow frame can never block input.
	intents := make(chan string, 32)
	go readIntents(s, intents)

	var (
		prev       []byte
		pw, ph     int
		frame      int
		sent       int
		framesSent int
		start      = time.Now()
		toast      string
		toastUntil time.Time
	)

	draw := func() {
		cols, rows := win.Width, win.Height
		if cols < 8 || rows < 6 {
			return
		}
		vw := clamp(cols*cellW/hdScale, 8, hdMaxTile)
		vh := clamp((rows-1)*cellH/hdScale, 8, hdMaxTile)
		ox, oy := px-vw/2, py-vh/2
		tm := hdWindow(gen, ox, oy, vw, vh)
		cam := game.Camera{W: vw, H: vh}
		img := game.RenderRGBA(nil, tm, w.PlayersInArea("wilds"), name, frame, cam, game.Light{}, ox, oy, hdScale, false)
		W, H := img.Bounds().Dx(), img.Bounds().Dy()

		doFull := prev == nil || W != pw || H != ph || frame%hdRefresh == 0
		if !doFull {
			if r, changed := pixel.DirtyRect(prev, img.Pix, W, H); !changed {
				// nothing moved — send nothing
			} else if r.Dx()*r.Dy() > W*H/2 {
				doFull = true
			} else {
				sr := pixel.SnapToCells(r, cellW, cellH, W, H)
				sub := pixel.EncodeSixel(pixel.Crop(img, sr), false)
				out.WriteString("\x1b7")
				fmt.Fprintf(out, "\x1b[%d;%dH", sr.Min.Y/cellH+1, sr.Min.X/cellW+1)
				out.Write(sub)
				out.WriteString("\x1b8")
				sent += len(sub)
			}
		}
		if doFull {
			full := pixel.EncodeSixel(img, false)
			out.WriteString("\x1b7\x1b[H")
			out.Write(full)
			out.WriteString("\x1b8")
			sent += len(full)
		}
		if toast != "" && time.Now().Before(toastUntil) {
			out.WriteString("\x1b7")
			fmt.Fprintf(out, "\x1b[%d;1H\x1b[2K\x1b[1;97;44m %s \x1b[0m", win.Height, toast)
			out.WriteString("\x1b8")
		}
		out.Flush()

		prev = append(prev[:0], img.Pix...)
		pw, ph = W, H
		framesSent++
	}

	hud := func() {
		dur := time.Since(start).Seconds()
		if dur <= 0 {
			dur = 1
		}
		out.WriteString("\x1b[?25h\x1b[0m\r\n")
		fmt.Fprintf(out, "HD session over: %d frames, %.0f KB sent, %.1f KB/s avg over %.0fs\r\n",
			framesSent, float64(sent)/1024, float64(sent)/1024/dur, dur)
		out.Flush()
	}

	draw()
	ticker := time.NewTicker(time.Second / hdFPS)
	defer ticker.Stop()
	for {
		select {
		case win = <-winCh:
			prev = nil // size changed → force a full repaint next tick
			out.WriteString("\x1b[2J")
		case <-ticker.C:
			// Coalesce input: apply only the most recent movement this tick, so a
			// held key advances one step per frame and stops the instant it is
			// released (terminals send no key-up, so we must not queue repeats).
			move := ""
			for draining := true; draining; {
				select {
				case k, ok := <-intents:
					if !ok || k == "quit" {
						hud()
						return
					}
					move = k
				default:
					draining = false
				}
			}
			if move != "" && hdMove(gen, &px, &py, move) {
				w.Move(name, px, py)
				if dest, ok := hdPortalUnder(gen, px, py); ok {
					toast = "◈ " + game.DisplayName(dest) + " — open it from the classic client (HD renders the Wilds only)"
					toastUntil = time.Now().Add(4 * time.Second)
				}
			}
			frame++ // advance tile/portal animation
			draw()
		}
	}
}

// readIntents parses raw terminal input into movement intents (or "quit") and
// sends them on out, dropping when the buffer is full so held-key repeats can't
// pile up. Arrow keys are CSI/SS3 escapes; ";2" means Shift (run).
func readIntents(s ssh.Session, out chan<- string) {
	b := make([]byte, 64)
	esc := 0
	var csi []byte
	emit := func(k string) {
		if k == "" {
			return
		}
		select {
		case out <- k:
		default:
		}
	}
	for {
		n, err := s.Read(b)
		for i := 0; i < n; i++ {
			c := b[i]
			switch {
			case esc == 0 && c == 0x1b:
				esc = 1
			case esc == 1:
				if c == '[' || c == 'O' {
					esc, csi = 2, csi[:0]
				} else {
					esc = 0
				}
			case esc == 2:
				if (c >= 'A' && c <= 'Z') || c == '~' {
					esc = 0
					emit(csiKey(string(csi), c))
				} else {
					csi = append(csi, c)
				}
			case c == 'q' || c == 'Q' || c == 3:
				emit("quit")
			default:
				emit(string(c))
			}
		}
		if err != nil {
			close(out)
			return
		}
	}
}

// hdPortalUnder reports the portal destination under the player's footprint.
func hdPortalUnder(gen *worldgen.Generator, x, y int) (string, bool) {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			if c := gen.At(x+dx, y+dy); c.Portal != "" {
				return c.Portal, true
			}
		}
	}
	return "", false
}

// hdMove walks the footprint per the shared movement mapping (WASD/arrows, YUBN
// diagonals, uppercase to run), respecting terrain.
func hdMove(gen *worldgen.Generator, px, py *int, key string) bool {
	dx, dy, steps, ok := game.MoveKey(key)
	if !ok {
		return false
	}
	moved := false
	for i := 0; i < steps; i++ {
		nx, ny := *px+dx, *py+dy
		if !hdFootprint(gen, nx, ny) {
			break
		}
		*px, *py = nx, ny
		moved = true
	}
	return moved
}

// csiKey turns a parsed arrow escape into a MoveKey name, prefixing "shift+"
// when the Shift modifier (parameter ";2") is present so it runs.
func csiKey(params string, final byte) string {
	dir := arrowKey(final)
	if dir == "" {
		return ""
	}
	if strings.Contains(params, ";2") { // Shift modifier
		return "shift+" + dir
	}
	return dir
}

// arrowKey maps a CSI/SS3 final byte to the movement key names MoveKey expects.
func arrowKey(b byte) string {
	switch b {
	case 'A':
		return "up"
	case 'B':
		return "down"
	case 'C':
		return "right"
	case 'D':
		return "left"
	}
	return ""
}

func hdSpawn(gen *worldgen.Generator) (int, int) {
	for _, off := range [][2]int{{2, 2}, {-3, 2}, {2, -3}, {-3, -3}, {3, 0}, {0, 3}} {
		x, y := worldgen.GateX+off[0], worldgen.GateY+off[1]
		if hdFootprint(gen, x, y) {
			return x, y
		}
	}
	return worldgen.GateX + 2, worldgen.GateY + 2
}

func hdFootprint(gen *worldgen.Generator, x, y int) bool {
	for dy := 0; dy < game.PlayerH; dy++ {
		for dx := 0; dx < game.PlayerW; dx++ {
			if !gen.Walkable(x+dx, y+dy) {
				return false
			}
		}
	}
	return true
}

// hdWindow samples a vw×vh tile window with its top-left at world (ox,oy).
func hdWindow(gen *worldgen.Generator, ox, oy, vw, vh int) *game.TileMap {
	tiles := make([][]game.Tile, vh)
	for ly := 0; ly < vh; ly++ {
		row := make([]game.Tile, vw)
		for lx := 0; lx < vw; lx++ {
			row[lx] = wilds.CellTile(gen.At(ox+lx, oy+ly))
		}
		tiles[ly] = row
	}
	return &game.TileMap{W: vw, H: vh, Tiles: tiles}
}

// hdCellSize derives the terminal's pixel cell size from the PTY window (if the
// client reports pixel dimensions), falling back to Windows Terminal's 10×20.
func hdCellSize(win ssh.Window) (int, int) {
	if win.WidthPixels > 0 && win.HeightPixels > 0 && win.Width > 0 && win.Height > 0 {
		if cw, ch := win.WidthPixels/win.Width, win.HeightPixels/win.Height; cw > 0 && ch > 0 {
			return cw, ch
		}
	}
	return 10, 20
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
