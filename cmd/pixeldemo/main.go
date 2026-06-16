// Command pixeldemo is a throwaway harness for the "real pixel" renderer
// experiment: it rasterizes a slice of the Wilds with game.RenderRGBA and
// pushes it to the terminal as a graphic — via the kitty graphics protocol or
// sixel — instead of half-block glyphs. It exists to answer two questions
// before we commit to the idea: what does it actually look like, and what does
// it cost (encode time + bytes on the wire, which over SSH is the real limit)?
//
// It deliberately does NOT touch bubbletea/wish: image protocols are
// out-of-band escapes that fight bubbletea's frame diffing, so we isolate the
// renderer here to measure it cleanly.
//
//	go run ./cmd/pixeldemo -proto kitty   # or sixel, or auto
//	go run ./cmd/pixeldemo -probe         # just report terminal capabilities
//
// Flags: -scale (px per tile), -w/-h (viewport in tiles), -frames, -fps,
// -seed, -still, and -motion (pan = camera scrolls, worst-case full-frame
// churn; walk = camera still, one avatar walks while tiles animate). Each run
// also reports a delta projection: the cost of re-sending only the dirty
// bounding box, which is where the real bandwidth win lives.
package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	"os"
	"os/signal"
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
	pan := flag.Int("pan", 1, "tiles to pan the camera per frame (worst-case churn)")
	motion := flag.String("motion", "pan", "scene motion: pan (camera scrolls, worst case) | walk (camera still, one avatar walks + tiles animate)")
	still := flag.Bool("still", false, "render a single frame and exit")
	probe := flag.Bool("probe", false, "detect terminal graphics support and exit")
	flag.Parse()

	if *probe {
		k, s := detect()
		fmt.Printf("kitty graphics: %v\nsixel:          %v\n", k, s)
		return
	}

	chosen := *proto
	if chosen == "auto" {
		k, s := detect()
		switch {
		case k:
			chosen = "kitty"
		case s:
			chosen = "sixel"
		default:
			fmt.Fprintln(os.Stderr, "no kitty/sixel support detected; pass -proto explicitly to force it")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "auto-detected: %s\n", chosen)
		time.Sleep(600 * time.Millisecond)
	}

	var encode func(*image.RGBA) []byte
	switch chosen {
	case "kitty":
		encode = encodeKitty
	case "sixel":
		encode = encodeSixel
	default:
		fmt.Fprintf(os.Stderr, "unknown proto %q\n", chosen)
		os.Exit(1)
	}

	if *still {
		*frames = 1
		*pan = 0
	}

	players := demoPlayers(*seed)
	out := bufio.NewWriterSize(os.Stdout, 1<<20)

	// Tidy the screen and hide the cursor; restore on exit / Ctrl-C.
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
	if walk {
		*pan = 0
	}

	var (
		rasterNS, encodeNS     int64
		totalBytes, deltaBytes int
		dirtySum               float64
		minB, maxB             = int(^uint(0) >> 1), 0
		started                = time.Now()
		framePeriod            = time.Second / time.Duration(maxInt(*fps, 1))
		prev                   []byte
	)

	for f := 0; f < *frames; f++ {
		frameStart := time.Now()

		if walk && f%3 == 0 { // step "you" east a tile to exercise sprite churn
			players[0].X++
			players[0].LastMoved = time.Now()
		}

		tm, ox, oy := wildsWindow(gen, wx, wy, *vw, *vh)
		cam := game.Camera{X: 0, Y: 0, W: *vw, H: *vh}

		t0 := time.Now()
		img := game.RenderRGBA(nil, tm, players, "you", f, cam, game.Light{}, ox, oy, *scale)
		t1 := time.Now()
		payload := encode(img)
		t2 := time.Now()

		// Delta accounting: what would it cost to re-send only the changed
		// region? zlib/sixel give no temporal delta for free, so the saving
		// comes from encoding just the dirty bounding box and placing it.
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		if prev == nil {
			deltaBytes += len(payload) // first frame is a full send
			dirtySum += 1
		} else if r, ok := dirtyRect(prev, img.Pix, w, h); ok {
			deltaBytes += len(encode(crop(img, r)))
			dirtySum += float64(r.Dx()*r.Dy()) / float64(w*h)
		}
		prev = append(prev[:0], img.Pix...)

		out.WriteString("\x1b[H")
		if chosen == "kitty" {
			out.WriteString("\x1b_Ga=d\x1b\\") // clear prior placements so frames don't pile up
		}
		out.Write(payload)
		out.Flush()

		rasterNS += t1.Sub(t0).Nanoseconds()
		encodeNS += t2.Sub(t1).Nanoseconds()
		totalBytes += len(payload)
		minB, maxB = minInt(minB, len(payload)), maxInt(maxB, len(payload))

		wx += *pan
		if d := framePeriod - time.Since(frameStart); d > 0 && !*still {
			time.Sleep(d)
		}
	}

	elapsed := time.Since(started)
	restore()
	report(reportData{
		proto:      chosen,
		motion:     *motion,
		frames:     *frames,
		tilesW:     *vw,
		tilesH:     *vh,
		scale:      *scale,
		pxW:        *vw * *scale,
		pxH:        *vh * *scale,
		fps:        *fps,
		rasterMS:   msPerFrame(rasterNS, *frames),
		encodeMS:   msPerFrame(encodeNS, *frames),
		avgBytes:   totalBytes / maxInt(*frames, 1),
		minBytes:   minB,
		maxBytes:   maxB,
		deltaBytes: deltaBytes / maxInt(*frames, 1),
		dirtyPct:   100 * dirtySum / float64(maxInt(*frames, 1)),
		gotFPS:     float64(*frames) / elapsed.Seconds(),
	})
}

// dirtyRect returns the bounding box of pixels that differ between two RGBA
// buffers of the same w×h, and whether anything changed at all.
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

// crop copies a rectangle out of img into a new tightly-packed RGBA (kitty/
// sixel both want contiguous pixels for the sub-image).
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
// (a=T). zlib is the difference between ~170 KB and a few KB for flat terrain,
// so it's the only fair way to weigh kitty against sixel on the wire.
func encodeKitty(img *image.RGBA) []byte {
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
		n := chunk
		if n > len(b64) {
			n = len(b64)
		}
		part := b64[:n]
		b64 = b64[n:]
		more := 0
		if len(b64) > 0 {
			more = 1
		}
		buf.WriteString("\x1b_G")
		if first {
			fmt.Fprintf(&buf, "f=32,o=z,s=%d,v=%d,a=T,", bounds.Dx(), bounds.Dy())
			first = false
		}
		fmt.Fprintf(&buf, "m=%d;%s\x1b\\", more, part)
	}
	return buf.Bytes()
}

// --- sixel -------------------------------------------------------------------

// encodeSixel quantizes the image to a 6×6×6 (216) color cube and emits sixel
// bands. Cheap and dependency-free; gradients will show some banding, which is
// fine for judging the look and — more importantly — the byte cost.
func encodeSixel(img *image.RGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	idx := make([]uint8, w*h) // palette index per pixel
	used := make([]bool, 216)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := img.PixOffset(x, y)
			p := q6(img.Pix[o])*36 + q6(img.Pix[o+1])*6 + q6(img.Pix[o+2])
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
		r := ((p / 36) % 6) * 20
		g := ((p / 6) % 6) * 20
		b := (p % 6) * 20
		fmt.Fprintf(&buf, "#%d;2;%d;%d;%d", p, r, g, b)
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
			buf.WriteByte('$') // carriage return: overstrike band with next color
		}
		buf.WriteByte('-') // next band
	}
	buf.WriteString("\x1b\\")
	return buf.Bytes()
}

func q6(v byte) int { return int(v) * 5 / 255 }

// --- capability detection ----------------------------------------------------

// detect probes the terminal: it asks kitty whether it understands a graphics
// query and reads the DA1 reply for the sixel feature flag (;4). Best-effort,
// with a short timeout; falls back to env hints if stdin isn't a TTY.
func detect() (kitty, sixel bool) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return envKitty(), false
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return envKitty(), false
	}
	defer term.Restore(fd, old)

	os.Stdout.WriteString("\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c")

	ch := make(chan byte, 512)
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
			// DA1 reply ends in 'c'; once seen we have everything.
			if c == 'c' && bytes.Contains(buf, []byte("\x1b[?")) {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	s := string(buf)
	kitty = strings.Contains(s, "\x1b_Gi=31;OK") || envKitty()
	sixel = da1HasSixel(s)
	return
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

func envKitty() bool {
	return os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("TERM") == "xterm-kitty" ||
		os.Getenv("TERM_PROGRAM") == "ghostty"
}

// --- reporting ---------------------------------------------------------------

type reportData struct {
	proto, motion                 string
	frames, tilesW, tilesH, scale int
	pxW, pxH, fps                 int
	rasterMS, encodeMS            float64
	avgBytes, minBytes, maxBytes  int
	deltaBytes                    int
	dirtyPct                      float64
	gotFPS                        float64
}

func report(d reportData) {
	kbPerFrame := float64(d.avgBytes) / 1024
	kbPerSec := kbPerFrame * float64(d.fps)
	mbitPerSec := kbPerSec * 8 / 1024
	deltaKB := float64(d.deltaBytes) / 1024
	fmt.Fprintf(os.Stderr, `
─── pixeldemo: %s (%s motion) ───────────────────
viewport     %d×%d tiles  →  %d×%d px  (scale %d)
frames       %d  @  target %d fps  (achieved %.1f fps)
rasterize    %.2f ms/frame
encode       %.2f ms/frame
full frame   avg %.1f KB/frame  (min %.1f, max %.1f)
             %.0f KB/s  ≈  %.2f Mbit/s  at %d fps
delta        %.1f%% of frame dirty  →  %.1f KB/frame
             %.0f KB/s  ≈  %.2f Mbit/s  at %d fps
─────────────────────────────────────────────────
`,
		d.proto, d.motion,
		d.tilesW, d.tilesH, d.pxW, d.pxH, d.scale,
		d.frames, d.fps, d.gotFPS,
		d.rasterMS,
		d.encodeMS,
		kbPerFrame, float64(d.minBytes)/1024, float64(d.maxBytes)/1024,
		kbPerSec, mbitPerSec, d.fps,
		d.dirtyPct, deltaKB,
		deltaKB*float64(d.fps), deltaKB*float64(d.fps)*8/1024, d.fps,
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
