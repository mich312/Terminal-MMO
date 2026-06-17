# Real-pixel renderer — experiment & decision

Status: **experimental prototype** (branch `claude/pixel-based-renderer`). Not
wired into the game. This records what we built, what we measured, and the
recommendation.

## The question

Could Durst World render with *real* pixels — terminal image protocols (kitty
graphics / sixel) — instead of, or alongside, the half-block glyph renderer?

## The hard constraint

Durst World's whole pitch is "ssh from any terminal and it works." Image
protocols only run on a minority of terminals (and not at all inside
tmux/screen, or in Terminal.app / default gnome-terminal / PuTTY / classic
conhost). **So a pixel renderer can never be the baseline** — the half-block
renderer stays the default, and any pixel path is an opt-in for detected
terminals. You maintain two renderers either way.

Reach note: kitty graphics is rare; **sixel is the wider target** and growing —
Windows Terminal 1.22+ (late 2024) added sixel (but not kitty), which covers a
large share of Windows users. Confirmed working: Windows Terminal reports
`sixel: true`, `cell size: 10×20`, `kitty: false`, and renders the Wilds with
walking avatars.

## What was built

- `internal/game/raster.go` — `RenderRGBA`: rasterizes the same scene as the
  glyph renderer (shares `buildGrid` for palette/tint/light). Flat tiles by
  default; optional bilinear-smoothed terrain + vignette; avatars upscaled with
  bilinear alpha (rounded, anti-aliased) over a soft contact shadow.
- `internal/pixel` — the `FrameWriter`: hand-rolled **kitty** (RGBA + zlib,
  optional `c=,r=` display scaling) and **sixel** (216-cube, ordered dither only
  when smoothing) encoders, plus the **delta loop** (transmit only the dirty
  cell-aligned region, nothing when static, periodic full refresh). Driven for
  live sessions by `cmd/durstworld/hd.go`, kept out of bubbletea/wish on purpose
  (image escapes are out-of-band and bubbletea would clobber them).

## What we measured

768×384 px viewport (64×32 tiles, scale 12), per frame:

| config | payload | @12 fps |
| --- | --- | --- |
| flat, sixel | **15.9 KB** | 1.5 Mbit/s |
| flat, kitty (zlib) | 20.1 KB | 1.9 Mbit/s |
| smooth, sixel | 213.7 KB | 20 Mbit/s |
| smooth, kitty | 216.8 KB | 20 Mbit/s |

Key findings:

- **Compressibility dominates, not pixel count.** Smooth per-pixel gradients are
  ~10–15× heavier than flat tiles, which keep long runs that zlib / sixel-RLE
  crush. (Sixel dithering must be *off* for flat — its noise bloats a flat frame
  ~10×.) "Pretty" is a luxury for fast links only.
- **Delta is the big lever, and only when the camera is still.** Codecs give no
  temporal delta for free; sending only the changed region does. Walking with a
  static camera is ~10–15% dirty → roughly that fraction of the bandwidth; an
  idle screen sends nothing. **Panning is 100% dirty and defeats deltas.**
- **Fixed internal resolution** decouples bytes from window size (kitty scales
  the buffer to fill the window). Sixel can't display-scale, so this is
  kitty-only; for sixel, render at the size you want.
- **CPU is per-session.** Rasterize ~2–5 ms (flat) / 5–10 ms (smooth); encode
  ~6 ms (kitty flat) up to ~20 ms (smooth) per connected player. Flat + delta +
  idle-skip keeps it sane; smooth at scale is a real server-CPU cost the
  half-block renderer doesn't have.

## Recommendation

1. **Keep half-blocks as the renderer.** Universal, colorful, free frame-diffing
   from bubbletea, snappy on bad links.
2. **If we add pixels, ship it as an opt-in `/hd` mode**, in exactly this shape:
   sixel-first (kitty too where present), **flat shading**, **delta +
   event-driven** (no continuous loop), low fps (~10–12), auto-fit viewport,
   with the **half-block renderer as the fallback**. That config is bounded
   (~16 KB/frame ceiling), predictable, fair (fixed FOV), and ~0 when idle.
3. **For a universal fidelity bump instead**, sextants (2×3) / braille (2×4)
   sub-cell glyphs give most of the resolution gain with none of the new
   constraints — likely the better bet for broad impact.

Two costs make `/hd` a real project, not a toggle: it must live **outside
bubbletea's `View()`-string loop** (image escapes are out-of-band and bubbletea
will clobber them), and it adds **per-session encode CPU** on the server.

## Run it

HD shipped as the default renderer — just connect (a plain interactive `ssh`
gets a PTY, so no `-t` is needed):

```sh
go run ./cmd/durstworld
ssh -p 2222 you@localhost            # HD (sixel/kitty, auto-detected from TERM)
ssh -t -p 2222 you@localhost glyph   # classic half-block glyph client
```
