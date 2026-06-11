package game

import (
	"sort"
	"strings"
	"unicode"

	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// RenderMap draws a tilemap with players on top. Players are drawn in
// LastMoved order (most recent wins a contested tile); the local player is
// always drawn last and highlighted.
func RenderMap(tm *TileMap, players []world.Player, self string, pulse bool) string {
	// glyph layer: start from tile display runes, then stamp texts & players
	type cell struct {
		ch    rune
		style int // index into styles below
	}
	const (
		stWall = iota
		stFloor
		stDecor
		stPortal
		stObject
		stLabel
		stVoid
	)

	grid := make([][]cell, tm.H)
	for y := 0; y < tm.H; y++ {
		grid[y] = make([]cell, tm.W)
		for x := 0; x < tm.W; x++ {
			t := tm.Tiles[y][x]
			c := cell{ch: t.Ch}
			switch t.Kind {
			case TileWall:
				c.style = stWall
			case TileFloor:
				c.style = stFloor
			case TileDecor:
				c.style = stDecor
			case TilePortal:
				c.style = stPortal
			case TileObject:
				c.style = stObject
			default:
				c.style = stVoid
			}
			grid[y][x] = c
		}
	}

	for _, txt := range tm.Texts {
		for i, r := range []rune(txt.S) {
			x := txt.X + i
			if txt.Y < 0 || txt.Y >= tm.H || x < 0 || x >= tm.W {
				continue
			}
			grid[txt.Y][x] = cell{ch: r, style: stLabel}
		}
	}

	// players: oldest movers first so recent movers draw on top
	sorted := make([]world.Player, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name == self {
			return false // self last
		}
		if sorted[j].Name == self {
			return true
		}
		return sorted[i].LastMoved.Before(sorted[j].LastMoved)
	})

	playerAt := make(map[[2]int]world.Player)
	for _, p := range sorted {
		playerAt[[2]int{p.X, p.Y}] = p
	}

	var b strings.Builder
	for y := 0; y < tm.H; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := 0; x < tm.W; x++ {
			if p, ok := playerAt[[2]int{x, y}]; ok {
				b.WriteString(playerGlyph(p, p.Name == self))
				continue
			}
			c := grid[y][x]
			s := string(c.ch)
			switch c.style {
			case stWall:
				b.WriteString(ui.WallStyle.Render(s))
			case stFloor:
				b.WriteString(ui.FloorStyle.Render(s))
			case stDecor:
				b.WriteString(ui.DecorStyle.Render(s))
			case stPortal:
				if pulse {
					b.WriteString(ui.PortalStyleA.Render(s))
				} else {
					b.WriteString(ui.PortalStyleB.Render(s))
				}
			case stObject:
				b.WriteString(ui.ObjectStyle.Render(s))
			case stLabel:
				b.WriteString(ui.LabelStyle.Render(s))
			default:
				b.WriteString(s)
			}
		}
	}
	return b.String()
}

// playerGlyph renders a player as the first letter of their name in their
// avatar color; the local player is bold + inverse.
func playerGlyph(p world.Player, isSelf bool) string {
	r := '☺'
	for _, c := range p.Name {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			r = unicode.ToUpper(c)
			break
		}
	}
	st := ui.ChatNameStyle.Foreground(p.Color)
	if isSelf {
		st = st.Reverse(true)
	}
	return st.Render(string(r))
}
