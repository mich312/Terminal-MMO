package world

import (
	"fmt"
	"testing"
)

// benchArea joins n players into one area, optionally as pollers, draining their
// channels in the background so deliver() never blocks on a full buffer.
func benchArea(b *testing.B, n int, pollers bool) *World {
	w := New()
	b.Cleanup(w.Close)
	for i := 0; i < n; i++ {
		name, ch := w.Join(fmt.Sprintf("p%d", i))
		if pollers {
			w.MarkPoller(name)
		}
		w.EnterArea(name, "lobby", i%20, i/20, "Lobby")
		go func() {
			for range ch {
			}
		}()
	}
	return w
}

// BenchmarkMoveBroadcast measures one Move in an N-player area: the global-lock
// hold plus the O(area) fan-out. This is the per-step server cost, paid up to
// hdMoveHz times/sec per moving player.
func BenchmarkMoveBroadcast(b *testing.B) {
	for _, n := range []int{2, 10, 50, 200} {
		b.Run(fmt.Sprintf("listeners_%d", n), func(b *testing.B) {
			w := benchArea(b, n, false)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.Move("p0", i%20, (i/20)%20)
			}
		})
		b.Run(fmt.Sprintf("pollers_%d", n), func(b *testing.B) {
			w := benchArea(b, n, true) // EventMoved skipped for all → just the scan + lock
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.Move("p0", i%20, (i/20)%20)
			}
		})
	}
}

// BenchmarkPlayersInArea measures the per-frame snapshot every HD session takes
// each render (O(all players) under the global lock).
func BenchmarkPlayersInArea(b *testing.B) {
	for _, n := range []int{2, 10, 50, 200} {
		b.Run(fmt.Sprintf("n_%d", n), func(b *testing.B) {
			w := benchArea(b, n, true)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = w.PlayersInArea("lobby")
			}
		})
	}
}
