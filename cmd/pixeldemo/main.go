// Command pixeldemo is a harness for the "real pixel" renderer experiment: it
// rasterizes a slice of the Wilds with game.RenderRGBA and pushes it to the
// terminal as a graphic — via the kitty graphics protocol or sixel — instead
// of half-block glyphs. It answers two questions before we commit to the idea:
// what does it look like, and what does it cost (encode time + bytes on the
// wire, which over SSH is the real limit)?
//
// It runs an efficient delta loop: it probes the terminal's cell size, then
// each frame transmits only the changed (dirty) region, aligned to the text
// cell grid, and transmits nothing at all when the scene is static. A periodic
// full refresh bounds kitty's placement memory. It deliberately does NOT touch
// bubbletea/wish — image escapes fight bubbletea's frame diffing, so we isolate
// the renderer here to measure it cleanly.
//
//	go run ./cmd/pixeldemo -proto kitty   # or sixel, or auto
//	go run ./cmd/pixeldemo -probe         # report capabilities + cell size
//
// Defaults are the config we'd actually ship: auto-detected protocol, viewport
// auto-fit to the window, flat shading (compresses), delta on. Flags: -scale
// (px/tile), -w/-h (viewport tiles, 0 = auto-fit), -frames, -fps, -seed,
// -still, -smooth (pretty but heavy), -res WxH (kitty-only fixed resolution),
// -refresh (frames between full repaints), -motion (pan = camera scrolls,
// worst-case churn; walk = camera still, one avatar walks). Each run reports
// the naive full-frame cost vs what the delta loop actually sent.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/pixel"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

func main() {
	proto := flag.String("proto", "auto", "image protocol: kitty | sixel | auto")
	scale := flag.Int("scale", 12, "pixels per tile (sprite pixel = scale/2)")
	vw := flag.Int("w", 0, "viewport width in tiles (0 = auto-fit the terminal)")
	vh := flag.Int("h", 0, "viewport height in tiles (0 = auto-fit the terminal)")
	frames := flag.Int("frames", 60, "number of frames to render")
	fps := flag.Int("fps", 12, "target frames per second")
	seed := flag.Uint64("seed", 1, "worldgen seed")
	motion := flag.String("motion", "pan", "scene motion: pan (camera scrolls) | walk (camera still, one avatar walks)")
	refresh := flag.Int("refresh", 48, "frames between full repaints (bounds kitty placement memory)")
	smooth := flag.Bool("smooth", false, "bilinear terrain + vignette (pretty but ~15x more bytes); default flat tiles compress far better")
	res := flag.String("res", "", "fixed internal render resolution WxH (e.g. 480x270); empty = native tiles×scale. kitty scales it to fill the window")
	still := flag.Bool("still", false, "render a single frame and exit")
	probe := flag.Bool("probe", false, "report terminal graphics support + cell size and exit")
	flag.Parse()

	caps := probeTerminal(!*still || *probe)
	if *probe {
		fmt.Printf("kitty graphics: %v\nsixel:          %v\ncell size:      %dx%d px\n",
			caps.kitty, caps.sixel, caps.cellW, caps.cellH)
		return
	}

	chosen := *proto
	if chosen == "auto" {
		switch {
		case caps.kitty:
			chosen = "kitty"
		case caps.sixel:
			chosen = "sixel"
		default:
			fmt.Fprintln(os.Stderr, "no kitty/sixel support detected; pass -proto explicitly to force it")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "auto-detected: %s\n", chosen)
		time.Sleep(500 * time.Millisecond)
	}

	var encode func(*image.RGBA) []byte
	switch chosen {
	case "kitty":
		encode = func(im *image.RGBA) []byte { return pixel.EncodeKitty(im, 0, 0) }
	case "sixel":
		encode = func(im *image.RGBA) []byte { return pixel.EncodeSixel(im, *smooth) }
	default:
		fmt.Fprintf(os.Stderr, "unknown proto %q\n", chosen)
		os.Exit(1)
	}

	if *still {
		*frames = 1
		*motion = "still"
	}

	// Auto-fit the viewport to the window: fill cols×(rows-1) cells (one row
	// spare so sixel never scrolls off the bottom) given the probed cell size.
	if *vw <= 0 || *vh <= 0 {
		aw, ah := autoViewport(*scale, caps.cellW, caps.cellH)
		if *vw <= 0 {
			*vw = aw
		}
		if *vh <= 0 {
			*vh = ah
		}
	}

	// Fixed internal resolution: render native, then box-downscale to a constant
	// buffer we transmit; kitty stretches it (c=,r=) to fill the window so bytes
	// stop scaling with terminal size. Sixel can't display-scale, so it shows at
	// the buffer's own pixel size.
	resW, resH := 0, 0
	if *res != "" {
		fmt.Sscanf(*res, "%dx%d", &resW, &resH)
	}
	resMode := resW > 0 && resH > 0

	nativeW, nativeH := *vw**scale, *vh**scale
	cols, rows := 0, 0
	if resMode && caps.cellW > 0 && caps.cellH > 0 {
		cols, rows = nativeW/caps.cellW, nativeH/caps.cellH
	}

	// Delta needs the terminal's cell pixel size to align a dirty sub-image to a
	// text cell; without it (or in fixed-res mode) we send full frames.
	deltaOK := !*still && !resMode && caps.cellW > 0 && caps.cellH > 0

	players := demoPlayers(*seed)
	out := bufio.NewWriterSize(os.Stdout, 1<<20)
	restore := func() {
		out.WriteString("\x1b[?25h\x1b[0m\n")
		out.Flush()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; restore(); os.Exit(0) }()
	out.WriteString("\x1b[2J\x1b[?25l")

	gen := worldgen.New(*seed)
	wx, wy := worldgen.GateX, worldgen.GateY
	walk := *motion == "walk"

	var (
		rasterNS, encodeNS   int64
		baseBytes, sentBytes int
		dirtySum             float64
		started              = time.Now()
		framePeriod          = time.Second / time.Duration(maxInt(*fps, 1))
		prev                 []byte
		w, h                 = nativeW, nativeH
	)
	if resMode {
		w, h = resW, resH // dirty-diff / buffer dims = transmitted resolution
	}

	for f := 0; f < *frames; f++ {
		frameStart := time.Now()
		if walk && f%3 == 0 { // step "you" east to exercise sprite churn
			players[0].X++
			players[0].LastMoved = time.Now()
		}

		tm, ox, oy := wildsWindow(gen, wx, wy, *vw, *vh)
		cam := game.Camera{X: 0, Y: 0, W: *vw, H: *vh}

		t0 := time.Now()
		img := game.RenderRGBA(nil, tm, players, "you", f, cam, game.Light{}, ox, oy, *scale, *smooth)
		if resMode {
			img = downscale(img, resW, resH)
		}
		t1 := time.Now()
		var full []byte // baseline + used on full frames
		if chosen == "kitty" {
			full = pixel.EncodeKitty(img, cols, rows)
		} else {
			full = pixel.EncodeSixel(img, *smooth)
		}
		t2 := time.Now()
		rasterNS += t1.Sub(t0).Nanoseconds()
		encodeNS += t2.Sub(t1).Nanoseconds()
		baseBytes += len(full)

		doFull := f == 0 || !deltaOK || (*refresh > 0 && f%*refresh == 0)
		dirty := 1.0
		if !doFull {
			r, changed := pixel.DirtyRect(prev, img.Pix, w, h)
			switch {
			case !changed:
				dirty = 0 // static: send nothing
			case float64(r.Dx()*r.Dy()) > 0.5*float64(w*h):
				doFull = true // mostly changed: a full frame is cheaper than the overhead
			default:
				sr := pixel.SnapToCells(r, caps.cellW, caps.cellH, w, h)
				sub := encode(pixel.Crop(img, sr))
				emit(out, sub, sr.Min.X/caps.cellW, sr.Min.Y/caps.cellH)
				sentBytes += len(sub)
				dirty = float64(sr.Dx()*sr.Dy()) / float64(w*h)
			}
		}
		if doFull {
			if chosen == "kitty" {
				out.WriteString("\x1b7\x1b[H\x1b_Ga=d\x1b\\") // reclaim old placements
				out.Write(full)
				out.WriteString("\x1b8")
			} else {
				emit(out, full, 0, 0)
			}
			sentBytes += len(full)
			dirty = 1
		}
		out.Flush()
		dirtySum += dirty

		if prev == nil {
			prev = make([]byte, len(img.Pix))
		}
		copy(prev, img.Pix)

		wx += panStep(walk)
		if d := framePeriod - time.Since(frameStart); d > 0 && !*still {
			time.Sleep(d)
		}
	}

	elapsed := time.Since(started)
	restore()
	report(reportData{
		proto: chosen, motion: *motion, delta: deltaOK, smooth: *smooth, fixed: resMode,
		frames: *frames, tilesW: *vw, tilesH: *vh, scale: *scale,
		pxW: w, pxH: h, fps: *fps,
		rasterMS:  msPerFrame(rasterNS, *frames),
		encodeMS:  msPerFrame(encodeNS, *frames),
		baseBytes: baseBytes / maxInt(*frames, 1),
		sentBytes: sentBytes / maxInt(*frames, 1),
		dirtyPct:  100 * dirtySum / float64(maxInt(*frames, 1)),
		gotFPS:    float64(*frames) / elapsed.Seconds(),
	})
}

// autoViewport sizes the tile viewport to fill the terminal: image pixels =
// cols×cellW by (rows-1)×cellH, divided by scale into tiles. Falls back to a
// sane default when the size or cell size is unknown (e.g. no TTY).
func autoViewport(scale, cellW, cellH int) (w, h int) {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 || rows <= 0 || cellW <= 0 || cellH <= 0 || scale <= 0 {
		return 64, 32
	}
	w = cols * cellW / scale
	h = (rows - 1) * cellH / scale
	if w < 8 {
		w = 8
	}
	if h < 8 {
		h = 8
	}
	return w, h
}

func panStep(walk bool) int {
	if walk {
		return 0
	}
	return 1
}

// emit positions the cursor at a text cell (1-based) and writes the image,
// bracketed by cursor save/restore so the placement never scrolls the screen.
func emit(out *bufio.Writer, payload []byte, col, row int) {
	out.WriteString("\x1b7")
	fmt.Fprintf(out, "\x1b[%d;%dH", row+1, col+1)
	out.Write(payload)
	out.WriteString("\x1b8")
}

// wildsWindow generates a width×height window of the overworld centered on
// (wx,wy), mirroring how the Wilds area samples the generator each frame.
func wildsWindow(gen *worldgen.Generator, wx, wy, width, height int) (*game.TileMap, int, int) {
	cx, cy := width/2, height/2
	tiles := make([][]game.Tile, height)
	for ly := 0; ly < height; ly++ {
		row := make([]game.Tile, width)
		for lx := 0; lx < width; lx++ {
			row[lx] = cellToTile(gen.At(wx+lx-cx, wy+ly-cy))
		}
		tiles[ly] = row
	}
	return &game.TileMap{W: width, H: height, Tiles: tiles}, wx - cx, wy - cy
}

func cellToTile(c worldgen.Cell) game.Tile {
	kind := game.TileFloor
	switch {
	case c.Portal != "":
		kind = game.TilePortal
	case c.Object:
		kind = game.TileObject
	case !c.Walkable:
		kind = game.TileDecor
	}
	t := game.Tile{Kind: kind, Ch: c.Glyph, Walkable: c.Walkable, Color: c.Color, Portal: c.Portal}
	if c.AnimA != "" && c.AnimB != "" {
		t.Anim = &game.TileAnim{Frames: c.Frames, ColorA: c.AnimA, ColorB: c.AnimB, Speed: 3}
	}
	return t
}

func demoPlayers(seed uint64) []world.Player {
	now := time.Now()
	colors := []string{"#7DF0FF", "#FFC861", "#C792EA", "#7BE08A", "#FF6E6E"}
	names := []string{"you", "anna", "markus", "lena", "tobias"}
	spots := [][2]int{{0, 0}, {3, -2}, {-3, 1}, {2, 4}, {-4, -3}}
	out := make([]world.Player, len(names))
	for i := range names {
		out[i] = world.Player{
			Name:      names[i],
			Area:      "wilds",
			X:         worldgen.GateX + spots[i][0],
			Y:         worldgen.GateY + spots[i][1],
			Color:     lipgloss.Color(colors[i]),
			LastMoved: now.Add(time.Duration(i) * time.Second),
		}
	}
	return out
}

// downscale area-averages src into a new outW×outH RGBA (a supersampling box
// filter): the fixed-resolution buffer we actually transmit.
func downscale(src *image.RGBA, outW, outH int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, outW, outH))
	for oy := 0; oy < outH; oy++ {
		sy0, sy1 := oy*sh/outH, (oy+1)*sh/outH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for ox := 0; ox < outW; ox++ {
			sx0, sx1 := ox*sw/outW, (ox+1)*sw/outW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, b, n int
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					o := src.PixOffset(xx, yy)
					r += int(src.Pix[o])
					g += int(src.Pix[o+1])
					b += int(src.Pix[o+2])
					n++
				}
			}
			out.SetRGBA(ox, oy, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255})
		}
	}
	return out
}

// --- capability detection ----------------------------------------------------

type termCaps struct {
	kitty, sixel bool
	cellW, cellH int
}

// probeTerminal asks the terminal, in one raw-mode round-trip: whether kitty
// graphics work (APC query → "OK"), whether sixel is advertised (DA1 ;4), and
// the cell pixel size (CSI 16 t → CSI 6 ; h ; w t). Best-effort with a short
// timeout; returns zeroes if stdin isn't a TTY or nothing answers.
func probeTerminal(enabled bool) termCaps {
	var caps termCaps
	fd := int(os.Stdin.Fd())
	if !enabled || !term.IsTerminal(fd) {
		caps.kitty = envKitty()
		return caps
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		caps.kitty = envKitty()
		return caps
	}
	defer term.Restore(fd, old)

	os.Stdout.WriteString("\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[16t\x1b[c")

	ch := make(chan byte, 1024)
	go func() {
		b := make([]byte, 1)
		for {
			n, e := os.Stdin.Read(b)
			if n > 0 {
				ch <- b[0]
			}
			if e != nil {
				return
			}
		}
	}()

	var buf []byte
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case c := <-ch:
			buf = append(buf, c)
			if c == 'c' && bytes.Contains(buf, []byte("\x1b[?")) {
				break loop // DA1 is the last reply we sent
			}
		case <-timeout:
			break loop
		}
	}

	s := string(buf)
	caps.kitty = strings.Contains(s, "\x1b_Gi=31;OK") || envKitty()
	caps.sixel = da1HasSixel(s)
	caps.cellW, caps.cellH = parseCellSize(s)
	return caps
}

func da1HasSixel(s string) bool {
	i := strings.Index(s, "\x1b[?")
	if i < 0 {
		return false
	}
	j := strings.IndexByte(s[i:], 'c')
	if j < 0 {
		return false
	}
	for _, p := range strings.Split(s[i+3:i+j], ";") {
		if p == "4" {
			return true
		}
	}
	return false
}

// parseCellSize reads a CSI 6 ; height ; width t window report into (w,h).
func parseCellSize(s string) (w, h int) {
	i := strings.Index(s, "\x1b[6;")
	if i < 0 {
		return 0, 0
	}
	j := strings.IndexByte(s[i:], 't')
	if j < 0 {
		return 0, 0
	}
	parts := strings.Split(s[i+4:i+j], ";")
	if len(parts) < 2 {
		return 0, 0
	}
	h, _ = strconv.Atoi(parts[0])
	w, _ = strconv.Atoi(parts[1])
	return w, h
}

func envKitty() bool {
	return os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("TERM") == "xterm-kitty" ||
		os.Getenv("TERM_PROGRAM") == "ghostty"
}

// --- reporting ---------------------------------------------------------------

type reportData struct {
	proto, motion                 string
	delta, smooth, fixed          bool
	frames, tilesW, tilesH, scale int
	pxW, pxH, fps                 int
	rasterMS, encodeMS            float64
	baseBytes, sentBytes          int
	dirtyPct                      float64
	gotFPS                        float64
}

func report(d reportData) {
	mbit := func(bytesPerFrame int) float64 {
		return float64(bytesPerFrame) / 1024 * float64(d.fps) * 8 / 1024
	}
	deltaNote := "off (no cell size / -still)"
	if d.delta {
		deltaNote = fmt.Sprintf("on, %.1f%% of frame dirty", d.dirtyPct)
	}
	look := "flat tiles"
	if d.smooth {
		look = "smooth (bilinear+vignette)"
	}
	if d.fixed {
		look += " · fixed-res"
	}
	fmt.Fprintf(os.Stderr, `
─── pixeldemo: %s (%s motion, %s) ───────────────
viewport     %d×%d tiles  →  %d×%d px  (scale %d)
frames       %d  @  target %d fps  (achieved %.1f fps)
rasterize    %.2f ms/frame
encode       %.2f ms/frame
full frame   %.1f KB  →  %.0f KB/s  ≈  %.2f Mbit/s  at %d fps
delta        %s
             %.1f KB/frame sent  →  %.0f KB/s  ≈  %.2f Mbit/s
─────────────────────────────────────────────────
`,
		d.proto, d.motion, look,
		d.tilesW, d.tilesH, d.pxW, d.pxH, d.scale,
		d.frames, d.fps, d.gotFPS,
		d.rasterMS, d.encodeMS,
		float64(d.baseBytes)/1024, float64(d.baseBytes)/1024*float64(d.fps), mbit(d.baseBytes), d.fps,
		deltaNote,
		float64(d.sentBytes)/1024, float64(d.sentBytes)/1024*float64(d.fps), mbit(d.sentBytes),
	)
}

func msPerFrame(totalNS int64, frames int) float64 {
	return float64(totalNS) / float64(maxInt(frames, 1)) / 1e6
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
