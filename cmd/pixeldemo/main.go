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
// Flags: -scale (px/tile), -w/-h (viewport tiles), -frames, -fps, -seed,
// -still, -refresh (frames between full repaints), and -motion (pan = camera
// scrolls, worst-case full-frame churn; walk = camera still, one avatar walks
// while tiles animate). Each run reports the naive full-frame cost vs what the
// delta loop actually sent.
package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
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
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"
)

func main() {
	proto := flag.String("proto", "auto", "image protocol: kitty | sixel | auto")
	scale := flag.Int("scale", 12, "pixels per tile (sprite pixel = scale/2)")
	vw := flag.Int("w", 64, "viewport width in tiles")
	vh := flag.Int("h", 32, "viewport height in tiles")
	frames := flag.Int("frames", 60, "number of frames to render")
	fps := flag.Int("fps", 12, "target frames per second")
	seed := flag.Uint64("seed", 1, "worldgen seed")
	motion := flag.String("motion", "pan", "scene motion: pan (camera scrolls) | walk (camera still, one avatar walks)")
	refresh := flag.Int("refresh", 48, "frames between full repaints (bounds kitty placement memory)")
	smooth := flag.Bool("smooth", true, "bilinear terrain + vignette (pretty but ~15x more bytes); false = flat tiles (compresses)")
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
		encode = func(im *image.RGBA) []byte { return encodeKitty(im, 0, 0) }
	case "sixel":
		encode = encodeSixel
	default:
		fmt.Fprintf(os.Stderr, "unknown proto %q\n", chosen)
		os.Exit(1)
	}

	if *still {
		*frames = 1
		*motion = "still"
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
			full = encodeKitty(img, cols, rows)
		} else {
			full = encodeSixel(img)
		}
		t2 := time.Now()
		rasterNS += t1.Sub(t0).Nanoseconds()
		encodeNS += t2.Sub(t1).Nanoseconds()
		baseBytes += len(full)

		doFull := f == 0 || !deltaOK || (*refresh > 0 && f%*refresh == 0)
		dirty := 1.0
		if !doFull {
			r, changed := dirtyRect(prev, img.Pix, w, h)
			switch {
			case !changed:
				dirty = 0 // static: send nothing
			case float64(r.Dx()*r.Dy()) > 0.5*float64(w*h):
				doFull = true // mostly changed: a full frame is cheaper than the overhead
			default:
				sr := snapToCells(r, caps.cellW, caps.cellH, w, h)
				sub := encode(crop(img, sr))
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

// snapToCells expands a pixel rect outward to the text-cell grid so a kitty/
// sixel sub-image placed at the cell aligns exactly with the dirty region.
func snapToCells(r image.Rectangle, cw, ch, w, h int) image.Rectangle {
	x0 := (r.Min.X / cw) * cw
	y0 := (r.Min.Y / ch) * ch
	x1 := minInt(((r.Max.X+cw-1)/cw)*cw, w)
	y1 := minInt(((r.Max.Y+ch-1)/ch)*ch, h)
	return image.Rect(x0, y0, x1, y1)
}

// dirtyRect returns the bounding box of pixels that differ between two RGBA
// buffers of the same w×h, and whether anything changed.
func dirtyRect(prev, cur []byte, w, h int) (image.Rectangle, bool) {
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		base := y * w * 4
		for x := 0; x < w; x++ {
			o := base + x*4
			if prev[o] != cur[o] || prev[o+1] != cur[o+1] || prev[o+2] != cur[o+2] || prev[o+3] != cur[o+3] {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

// crop copies a rectangle out of img into a new tightly-packed RGBA.
func crop(img *image.RGBA, r image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		so := img.PixOffset(r.Min.X, r.Min.Y+y)
		do := out.PixOffset(0, y)
		copy(out.Pix[do:do+r.Dx()*4], img.Pix[so:so+r.Dx()*4])
	}
	return out
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

// --- kitty graphics protocol -------------------------------------------------

// encodeKitty transmits the image as RGBA (f=32), zlib-compressed (o=z),
// base64'd and chunked at 4096 bytes per APC escape, displayed at the cursor
// without moving it (a=T, C=1). zlib is the difference between ~170 KB and a
// few KB for flat terrain, the only fair way to weigh kitty against sixel.
//
// When cols/rows > 0 the image is display-scaled to fill that many text cells
// (c=,r=) — the basis for a fixed internal render resolution: transmit a small
// constant buffer and let kitty stretch it to the window, so bytes/frame no
// longer grow with terminal size.
func encodeKitty(img *image.RGBA, cols, rows int) []byte {
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	zw.Write(img.Pix)
	zw.Close()

	b64 := base64.StdEncoding.EncodeToString(zb.Bytes())
	bounds := img.Bounds()
	var buf bytes.Buffer
	const chunk = 4096
	first := true
	for len(b64) > 0 {
		n := minInt(chunk, len(b64))
		part := b64[:n]
		b64 = b64[n:]
		more := 0
		if len(b64) > 0 {
			more = 1
		}
		buf.WriteString("\x1b_G")
		if first {
			fmt.Fprintf(&buf, "f=32,o=z,s=%d,v=%d,a=T,C=1,", bounds.Dx(), bounds.Dy())
			if cols > 0 && rows > 0 {
				fmt.Fprintf(&buf, "c=%d,r=%d,", cols, rows)
			}
			first = false
		}
		fmt.Fprintf(&buf, "m=%d;%s\x1b\\", more, part)
	}
	return buf.Bytes()
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

// --- sixel -------------------------------------------------------------------

var bayer4 = [4][4]int{
	{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5},
}

// encodeSixel quantizes to a 6×6×6 (216) color cube with ordered dithering to
// soften gradient banding, then emits sixel bands. Cheap and dependency-free.
func encodeSixel(img *image.RGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	idx := make([]uint8, w*h)
	used := make([]bool, 216)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := img.PixOffset(x, y)
			p := q6(img.Pix[o], x, y)*36 + q6(img.Pix[o+1], x, y)*6 + q6(img.Pix[o+2], x, y)
			idx[y*w+x] = uint8(p)
			used[p] = true
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "\x1bPq\"1;1;%d;%d", w, h)
	for p := 0; p < 216; p++ {
		if !used[p] {
			continue
		}
		fmt.Fprintf(&buf, "#%d;2;%d;%d;%d", p, ((p/36)%6)*20, ((p/6)%6)*20, (p%6)*20)
	}

	for band := 0; band < h; band += 6 {
		rows := minInt(6, h-band)
		present := map[uint8]bool{}
		for r := 0; r < rows; r++ {
			base := (band + r) * w
			for x := 0; x < w; x++ {
				present[idx[base+x]] = true
			}
		}
		for c := range present {
			fmt.Fprintf(&buf, "#%d", c)
			var prev byte
			var run int
			flush := func() {
				if run == 0 {
					return
				}
				if run > 3 {
					fmt.Fprintf(&buf, "!%d", run)
					buf.WriteByte(prev)
				} else {
					for i := 0; i < run; i++ {
						buf.WriteByte(prev)
					}
				}
			}
			for x := 0; x < w; x++ {
				var bits byte
				for r := 0; r < rows; r++ {
					if idx[(band+r)*w+x] == c {
						bits |= 1 << uint(r)
					}
				}
				ch := byte('?') + bits
				if ch == prev {
					run++
				} else {
					flush()
					prev, run = ch, 1
				}
			}
			flush()
			buf.WriteByte('$')
		}
		buf.WriteByte('-')
	}
	buf.WriteString("\x1b\\")
	return buf.Bytes()
}

// q6 maps a channel to a 0..5 level with ordered (Bayer) dithering.
func q6(v byte, x, y int) int {
	t := float64(v) / 255 * 5
	thr := float64(bayer4[y&3][x&3])/16 - 0.5
	lvl := int(t + thr + 0.5)
	if lvl < 0 {
		return 0
	}
	if lvl > 5 {
		return 5
	}
	return lvl
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
