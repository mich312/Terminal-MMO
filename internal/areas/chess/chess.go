// Package chess is the arcade's chessboard: you (White) versus a simple house
// AI (Black). Full legal moves — including castling, en passant and promotion
// (auto-queen) — with check, checkmate and stalemate. Keypress-driven and a
// board game (camera-framed, no avatar): the glyph client draws Unicode pieces,
// the HD client the piece sprites. Move the cursor with WASD/arrows, 'e' selects
// then moves; 'r' new game, 'x' leaves.
package chess

import (
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"
)

// Piece codes: sign is the side (+white, −black), magnitude the kind.
const (
	pawn = iota + 1
	knight
	bishop
	rook
	queen
	king
)

const white, black = 1, -1

type move struct {
	fx, fy, tx, ty int
	promo          bool
	castle         int // +1 king-side, −1 queen-side, 0 none
	enpas          bool
}

type castleRights struct{ wk, wq, bk, bq bool }

func init() {
	game.Register("chess", "Chess", func(ctx *game.Ctx) game.Area {
		return &area{Walker: game.Walker{Ctx: ctx, AreaID: "chess"}}
	})
}

type area struct {
	game.Walker
	board    [8][8]int // board[y][x]; y=0 is Black's back rank (top), y=7 White's
	turn     int
	cr       castleRights
	ep       [2]int // en-passant target square, or {-1,-1}
	cursor   [2]int
	sel      [2]int // selected square, or {-1,-1}
	targets  map[[2]int]bool
	over     bool
	result   string
	rng      *rand.Rand
	toast    string
	toastUnt time.Time
}

func (a *area) Name() string      { return "Chess" }
func (a *area) HideAvatars() bool { return true }

func (a *area) Init(p *world.Player) tea.Cmd {
	a.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	a.reset()
	return nil
}

func (a *area) reset() {
	a.board = [8][8]int{}
	back := []int{rook, knight, bishop, queen, king, bishop, knight, rook}
	for x := 0; x < 8; x++ {
		a.board[0][x] = -back[x]
		a.board[1][x] = -pawn
		a.board[6][x] = pawn
		a.board[7][x] = back[x]
	}
	a.turn = white
	a.cr = castleRights{true, true, true, true}
	a.ep = [2]int{-1, -1}
	a.cursor = [2]int{4, 6}
	a.sel = [2]int{-1, -1}
	a.targets = nil
	a.over, a.result = false, ""
	a.X, a.Y = 4, 4
	a.rebuild()
	a.Ctx.World.EnterArea(a.Ctx.Name, a.AreaID, a.X, a.Y, a.Name())
}

func sign(p int) int {
	switch {
	case p > 0:
		return white
	case p < 0:
		return black
	}
	return 0
}
func kind(p int) int {
	if p < 0 {
		return -p
	}
	return p
}
func inBoard(x, y int) bool { return x >= 0 && x < 8 && y >= 0 && y < 8 }

// ── Move generation ────────────────────────────────────────────────────────

var knightJumps = [8][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
var diag = [4][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
var orth = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// pseudoMoves lists a side's moves ignoring whether they leave the king in check
// (castling legality through-check is verified in legalMoves).
func (a *area) pseudoMoves(side int) []move {
	var ms []move
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			p := a.board[y][x]
			if sign(p) != side {
				continue
			}
			switch kind(p) {
			case pawn:
				ms = a.pawnMoves(ms, x, y, side)
			case knight:
				for _, d := range knightJumps {
					nx, ny := x+d[0], y+d[1]
					if inBoard(nx, ny) && sign(a.board[ny][nx]) != side {
						ms = append(ms, move{x, y, nx, ny, false, 0, false})
					}
				}
			case bishop:
				ms = a.slide(ms, x, y, side, diag[:])
			case rook:
				ms = a.slide(ms, x, y, side, orth[:])
			case queen:
				ms = a.slide(ms, x, y, side, append(append([][2]int{}, diag[:]...), orth[:]...))
			case king:
				for _, d := range append(append([][2]int{}, diag[:]...), orth[:]...) {
					nx, ny := x+d[0], y+d[1]
					if inBoard(nx, ny) && sign(a.board[ny][nx]) != side {
						ms = append(ms, move{x, y, nx, ny, false, 0, false})
					}
				}
				ms = a.castleMoves(ms, x, y, side)
			}
		}
	}
	return ms
}

func (a *area) slide(ms []move, x, y, side int, dirs [][2]int) []move {
	for _, d := range dirs {
		for nx, ny := x+d[0], y+d[1]; inBoard(nx, ny); nx, ny = nx+d[0], ny+d[1] {
			t := a.board[ny][nx]
			if sign(t) == side {
				break
			}
			ms = append(ms, move{x, y, nx, ny, false, 0, false})
			if t != 0 {
				break // capture stops the ray
			}
		}
	}
	return ms
}

func (a *area) pawnMoves(ms []move, x, y, side int) []move {
	dy := -side // White (+1) marches up the board toward y=0
	start := 6
	last := 0
	if side == black {
		start, last = 1, 7
	}
	// forward one / two
	if inBoard(x, y+dy) && a.board[y+dy][x] == 0 {
		ms = appendPawn(ms, x, y, x, y+dy, y+dy == last)
		if y == start && a.board[y+2*dy][x] == 0 {
			ms = append(ms, move{x, y, x, y + 2*dy, false, 0, false})
		}
	}
	// captures + en passant
	for _, dx := range []int{-1, 1} {
		nx, ny := x+dx, y+dy
		if !inBoard(nx, ny) {
			continue
		}
		if t := a.board[ny][nx]; t != 0 && sign(t) != side {
			ms = appendPawn(ms, x, y, nx, ny, ny == last)
		} else if a.ep[0] == nx && a.ep[1] == ny {
			ms = append(ms, move{x, y, nx, ny, false, 0, true})
		}
	}
	return ms
}

func appendPawn(ms []move, fx, fy, tx, ty int, promo bool) []move {
	return append(ms, move{fx, fy, tx, ty, promo, 0, false})
}

func (a *area) castleMoves(ms []move, x, y, side int) []move {
	if a.attacked(x, y, -side) {
		return ms // can't castle out of check
	}
	homeY := 7
	kSide, qSide := a.cr.wk, a.cr.wq
	if side == black {
		homeY, kSide, qSide = 0, a.cr.bk, a.cr.bq
	}
	if y != homeY || x != 4 {
		return ms
	}
	// king-side: f,g empty and not attacked, rook on h
	if kSide && a.board[homeY][5] == 0 && a.board[homeY][6] == 0 &&
		!a.attacked(5, homeY, -side) && !a.attacked(6, homeY, -side) && kind(a.board[homeY][7]) == rook {
		ms = append(ms, move{4, homeY, 6, homeY, false, 1, false})
	}
	// queen-side: b,c,d empty (b need not be unattacked), c,d not attacked, rook on a
	if qSide && a.board[homeY][1] == 0 && a.board[homeY][2] == 0 && a.board[homeY][3] == 0 &&
		!a.attacked(3, homeY, -side) && !a.attacked(2, homeY, -side) && kind(a.board[homeY][0]) == rook {
		ms = append(ms, move{4, homeY, 2, homeY, false, -1, false})
	}
	return ms
}

// attacked reports whether (x,y) is attacked by any piece of side `by`.
func (a *area) attacked(x, y, by int) bool {
	// pawns (they attack toward −by's marching direction)
	pdy := by // a `by` pawn sits at (x±1, y+by) attacking (x,y)
	for _, dx := range []int{-1, 1} {
		px, py := x+dx, y+pdy
		if inBoard(px, py) && a.board[py][px] == by*pawn {
			return true
		}
	}
	for _, d := range knightJumps {
		nx, ny := x+d[0], y+d[1]
		if inBoard(nx, ny) && a.board[ny][nx] == by*knight {
			return true
		}
	}
	for _, d := range append(append([][2]int{}, diag[:]...), orth[:]...) {
		nx, ny := x+d[0], y+d[1]
		if inBoard(nx, ny) && a.board[ny][nx] == by*king {
			return true
		}
	}
	for _, d := range diag {
		if a.rayHits(x, y, d, by, bishop) {
			return true
		}
	}
	for _, d := range orth {
		if a.rayHits(x, y, d, by, rook) {
			return true
		}
	}
	return false
}

// rayHits walks a direction until it meets a piece, returning true if that piece
// is a `by`-side slider of the given kind (or a queen).
func (a *area) rayHits(x, y int, d [2]int, by, slider int) bool {
	for nx, ny := x+d[0], y+d[1]; inBoard(nx, ny); nx, ny = nx+d[0], ny+d[1] {
		p := a.board[ny][nx]
		if p == 0 {
			continue
		}
		return sign(p) == by && (kind(p) == slider || kind(p) == queen)
	}
	return false
}

func (a *area) kingPos(side int) (int, int) {
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if a.board[y][x] == side*king {
				return x, y
			}
		}
	}
	return -1, -1
}

func (a *area) inCheck(side int) bool {
	kx, ky := a.kingPos(side)
	return kx >= 0 && a.attacked(kx, ky, -side)
}

// legalMoves filters pseudo-moves to those that don't leave the mover in check.
func (a *area) legalMoves(side int) []move {
	var out []move
	for _, m := range a.pseudoMoves(side) {
		undo := a.apply(m)
		if !a.inCheck(side) {
			out = append(out, m)
		}
		a.undo(undo)
	}
	return out
}

// undoState captures everything apply mutates, for a cheap rollback.
type undoState struct {
	m                move
	captured         int
	capAt            [2]int
	cr               castleRights
	ep               [2]int
	movedPiece       int
	rookFrom, rookTo [2]int
	hadRook          bool
}

// apply makes a move on the board and returns an undo token. It does not switch
// turn (callers manage that) so it can be used both for real moves and for the
// legality probe in legalMoves.
func (a *area) apply(m move) undoState {
	u := undoState{m: m, cr: a.cr, ep: a.ep, capAt: [2]int{m.tx, m.ty}}
	p := a.board[m.fy][m.fx]
	u.movedPiece = p
	side := sign(p)

	u.captured = a.board[m.ty][m.tx]
	if m.enpas { // captured pawn sits beside the destination, not on it
		cy := m.ty + side // behind the destination from the mover's view
		u.captured = a.board[cy][m.tx]
		u.capAt = [2]int{m.tx, cy}
		a.board[cy][m.tx] = 0
	}
	a.board[m.fy][m.fx] = 0
	if m.promo {
		a.board[m.ty][m.tx] = side * queen
	} else {
		a.board[m.ty][m.tx] = p
	}
	// castling: shift the rook
	if m.castle != 0 {
		u.hadRook = true
		if m.castle == 1 {
			u.rookFrom, u.rookTo = [2]int{7, m.fy}, [2]int{5, m.fy}
		} else {
			u.rookFrom, u.rookTo = [2]int{0, m.fy}, [2]int{3, m.fy}
		}
		a.board[u.rookTo[1]][u.rookTo[0]] = a.board[u.rookFrom[1]][u.rookFrom[0]]
		a.board[u.rookFrom[1]][u.rookFrom[0]] = 0
	}
	a.updateRights(m, p)
	// set en-passant target if a pawn double-pushed
	a.ep = [2]int{-1, -1}
	if kind(p) == pawn && abs(m.ty-m.fy) == 2 {
		a.ep = [2]int{m.fx, (m.fy + m.ty) / 2}
	}
	return u
}

func (a *area) undo(u undoState) {
	m := u.m
	a.board[m.fy][m.fx] = u.movedPiece
	a.board[m.ty][m.tx] = 0
	if u.captured != 0 || u.capAt != [2]int{m.tx, m.ty} {
		a.board[u.capAt[1]][u.capAt[0]] = u.captured
	}
	if u.hadRook {
		a.board[u.rookFrom[1]][u.rookFrom[0]] = a.board[u.rookTo[1]][u.rookTo[0]]
		a.board[u.rookTo[1]][u.rookTo[0]] = 0
	}
	a.cr, a.ep = u.cr, u.ep
}

func (a *area) updateRights(m move, p int) {
	switch p {
	case white * king:
		a.cr.wk, a.cr.wq = false, false
	case black * king:
		a.cr.bk, a.cr.bq = false, false
	}
	lose := func(x, y int) {
		switch {
		case x == 7 && y == 7:
			a.cr.wk = false
		case x == 0 && y == 7:
			a.cr.wq = false
		case x == 7 && y == 0:
			a.cr.bk = false
		case x == 0 && y == 0:
			a.cr.bq = false
		}
	}
	lose(m.fx, m.fy) // a rook leaving its corner
	lose(m.tx, m.ty) // …or being captured on it
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ── Play ────────────────────────────────────────────────────────────────────

func (a *area) Update(msg tea.Msg) (game.Area, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	switch key.String() {
	case "x":
		return game.Transition{To: "arcade"}, nil
	case "r":
		a.reset()
		return a, nil
	case "e", " ":
		if !a.over && a.turn == white {
			a.click()
		}
		return a, nil
	}
	if dx, dy, _, ok := game.MoveKey(key.String()); ok && (dx == 0) != (dy == 0) {
		a.cursor[0] = clamp(a.cursor[0]+dx, 0, 7)
		a.cursor[1] = clamp(a.cursor[1]+dy, 0, 7)
		a.rebuild()
	}
	return a, nil
}

// click selects your piece, or moves the selected piece to the cursor if that's
// a legal destination.
func (a *area) click() {
	c := a.cursor
	if a.sel[0] >= 0 && a.targets[c] {
		a.playerMove(a.sel, c)
		return
	}
	if sign(a.board[c[1]][c[0]]) == white { // (re)select your own piece
		a.sel = c
		a.targets = map[[2]int]bool{}
		for _, m := range a.legalMoves(white) {
			if m.fx == c[0] && m.fy == c[1] {
				a.targets[[2]int{m.tx, m.ty}] = true
			}
		}
	} else {
		a.sel = [2]int{-1, -1}
		a.targets = nil
	}
	a.rebuild()
}

func (a *area) playerMove(from, to [2]int) {
	for _, m := range a.legalMoves(white) {
		if m.fx == from[0] && m.fy == from[1] && m.tx == to[0] && m.ty == to[1] {
			a.apply(m)
			a.turn = black
			a.sel = [2]int{-1, -1}
			a.targets = nil
			if a.finishIfDone(black) {
				a.rebuild()
				return
			}
			a.aiMove()
			a.finishIfDone(white)
			a.rebuild()
			return
		}
	}
}

// finishIfDone ends the game if `side` (the side to move) has no legal reply.
func (a *area) finishIfDone(side int) bool {
	if len(a.legalMoves(side)) > 0 {
		return false
	}
	a.over = true
	switch {
	case a.inCheck(side) && side == black:
		a.result, a.toast = "checkmate — you win!", "🏆 checkmate — you win! · r new · x leave"
	case a.inCheck(side):
		a.result, a.toast = "checkmate — the house wins", "checkmate — you lose · r new · x leave"
	default:
		a.result, a.toast = "stalemate — a draw", "stalemate · r new · x leave"
	}
	a.toastUnt = time.Now().Add(8 * time.Second)
	return true
}

var valOf = map[int]int{pawn: 1, knight: 3, bishop: 3, rook: 5, queen: 9, king: 0}

// aiMove plays Black: greedily grab the most valuable capture, else a random
// legal move — beatable, but it won't hang pieces for free.
func (a *area) aiMove() {
	moves := a.legalMoves(black)
	if len(moves) == 0 {
		return
	}
	best, bestVal := -1, -1
	for i, m := range moves {
		v := 0
		if t := a.board[m.ty][m.tx]; t != 0 {
			v = valOf[kind(t)]
		}
		if m.enpas {
			v = 1
		}
		if v > bestVal || (v == bestVal && a.rng.Intn(3) == 0) {
			best, bestVal = i, v
		}
	}
	if bestVal <= 0 { // nothing worth taking → play any legal move
		best = a.rng.Intn(len(moves))
	}
	a.apply(moves[best])
	a.turn = white
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

// ── Rendering ────────────────────────────────────────────────────────────────

var glyphs = map[int]rune{
	white * pawn: '♙', white * knight: '♘', white * bishop: '♗', white * rook: '♖', white * queen: '♕', white * king: '♔',
	black * pawn: '♟', black * knight: '♞', black * bishop: '♝', black * rook: '♜', black * queen: '♛', black * king: '♚',
}
var props = map[int]game.TileProp{
	pawn: game.PropChessPawn, knight: game.PropChessKnight, bishop: game.PropChessBishop,
	rook: game.PropChessRook, queen: game.PropChessQueen, king: game.PropChessKing,
}

func (a *area) rebuild() {
	tiles := make([][]game.Tile, 8)
	for y := 0; y < 8; y++ {
		row := make([]game.Tile, 8)
		for x := 0; x < 8; x++ {
			ground := "#B9A06A" // light square
			if (x+y)%2 == 1 {
				ground = "#7E6A46" // dark square
			}
			switch {
			case a.sel == [2]int{x, y}:
				ground = "#4F8A57" // selected
			case a.targets[[2]int{x, y}]:
				ground = "#5A7A4A" // legal destination
			case a.cursor == [2]int{x, y}:
				ground = "#3C6EA5" // cursor
			}
			t := game.Tile{Kind: game.TileFloor, Ch: ' ', Walkable: true, Tex: game.TexFloor, Ground: ground}
			if p := a.board[y][x]; p != 0 {
				hex := "#F0EAD6"
				if sign(p) == black {
					hex = "#2E2E38"
				}
				t.Kind = game.TileDecor
				t.Ch = glyphs[p]
				t.Color = hex
				t.Prop = props[kind(p)]
				t.PropHex = hex
			} else if a.cursor == [2]int{x, y} {
				t.Ch, t.Color = '∙', "#9FD0FF"
			} else if a.targets[[2]int{x, y}] {
				t.Ch, t.Color = '•', "#BfE0A0"
			}
			row[x] = t
		}
		tiles[y] = row
	}
	a.Map = &game.TileMap{W: 8, H: 8, Tiles: tiles}
}

func (a *area) Toast() (string, bool) {
	if a.toast != "" && time.Now().Before(a.toastUnt) {
		return a.toast, true
	}
	return "", false
}

func (a *area) status(th *ui.Theme) string {
	switch {
	case a.over:
		return th.Warn.Render(a.result)
	case a.inCheck(white):
		return th.Warn.Render("check!")
	case a.turn == white:
		return th.Accent.Render("your move (White)")
	default:
		return th.Dim.Render("the house thinks…")
	}
}

func (a *area) Hint() string {
	if a.over {
		return a.result + " · r new game · x leave"
	}
	return "move cursor · e select/move · r new · x leave"
}

func (a *area) Prompt() (string, bool) {
	if a.over {
		return a.result + " · r new game · x leave", true
	}
	if a.sel[0] >= 0 {
		return "e — move here (or pick another piece) · x leave", true
	}
	return "move the cursor · e to pick a piece · x leave", true
}

func (a *area) View(width, height int) string {
	th := a.Ctx.Theme
	if th == nil {
		th = ui.Default
	}
	rows := []string{
		th.PanelTitle.Render("♛ Chess"), "",
		th.ChatText.Render("You are White vs the"),
		th.ChatText.Render("house AI."), "",
		th.Dim.Render("Turn   ") + a.status(th), "",
		th.Dim.Render("WASD / arrows  cursor"),
		th.Dim.Render("e              select/move"),
		th.Dim.Render("r              new game"),
		th.Dim.Render("x              leave"),
	}
	panel := th.Panel.Width(30).Render(strings.Join(rows, "\n"))

	const gap = 3
	mapW := width - lipgloss.Width(panel) - gap
	if mapW < 24 {
		mapW = 24
	}
	mapView := a.RenderBoard(mapW, height)
	return lipgloss.JoinHorizontal(lipgloss.Center, panel, "   ", mapView)
}
