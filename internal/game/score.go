package game

// Arcade leaderboards: each score-bearing cabinet keeps a persistent board
// (store `highscores`), and a run that beats every stored score is announced
// to everyone online — the bragging rights that make ten minigames a rivalry.

import "fmt"

// ScoreGame names one cabinet that keeps a leaderboard, with the unit its
// scores wear on the board. Pong, Chess, Maze and Doom have no meaningful
// number, so they stay off it.
type ScoreGame struct {
	ID   string
	Unit string
}

// ScoreGames is the hall's board order.
var ScoreGames = []ScoreGame{
	{"snake", "pts"},
	{"tetris", "pts"},
	{"2048", "pts"},
	{"breakout", "pts"},
	{"sokoban", "levels"},
}

// SubmitScore records a finished run on a cabinet's board and, when it sets a
// new hall record, announces it to everyone online. Zero scores stay off the
// board, and the degraded (no-op) store simply never reports a record.
func SubmitScore(ctx *Ctx, gameID string, score int) {
	if ctx == nil || ctx.Store == nil || score <= 0 {
		return
	}
	if ctx.Store.SaveScore(gameID, ctx.Name, score) && ctx.World != nil {
		ctx.World.Announce(fmt.Sprintf("★ %s set the arcade record on %s — %d", ctx.Name, DisplayName(gameID), score))
	}
}
