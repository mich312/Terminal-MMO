package world

import (
	"testing"
	"time"
)

func drain(ch <-chan Event) []Event {
	var out []Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestJoinNamesAreUnique(t *testing.T) {
	w := New()
	defer w.Close()

	n1, _ := w.Join("anna")
	n2, _ := w.Join("anna")
	n3, _ := w.Join("")

	if n1 != "anna" {
		t.Errorf("first join: got %q", n1)
	}
	if n2 != "anna-2" {
		t.Errorf("duplicate join: got %q", n2)
	}
	if n3 != "guest-1" {
		t.Errorf("empty name: got %q", n3)
	}
}

func TestPresenceAndMovement(t *testing.T) {
	w := New()
	defer w.Close()

	anna, annaCh := w.Join("anna")
	bert, bertCh := w.Join("bert")

	w.EnterArea(anna, "lobby", 5, 5, "Lobby")
	w.EnterArea(bert, "lobby", 6, 5, "Lobby")
	drain(annaCh)
	drain(bertCh)

	w.Move(anna, 5, 6)
	found := false
	for _, ev := range drain(bertCh) {
		if ev.Type == EventMoved && ev.Player == anna && ev.X == 5 && ev.Y == 6 {
			found = true
		}
	}
	if !found {
		t.Error("bert did not see anna move")
	}

	players := w.PlayersInArea("lobby")
	if len(players) != 2 {
		t.Fatalf("expected 2 players in lobby, got %d", len(players))
	}

	w.Leave(anna)
	if got := len(w.PlayersInArea("lobby")); got != 1 {
		t.Errorf("after leave: expected 1 player, got %d", got)
	}
	if _, ok := <-annaCh; ok {
		// channel should be closed (after pending events drain)
		for range annaCh {
		}
	}
}

func TestPollerSkipsMoveAndTickButKeepsChat(t *testing.T) {
	w := New()
	defer w.Close()

	anna, annaCh := w.Join("anna") // HD-style poller
	w.MarkPoller(anna)
	bert, bertCh := w.Join("bert") // glyph-style event listener

	w.EnterArea(anna, "lobby", 5, 5, "Lobby")
	w.EnterArea(bert, "lobby", 6, 5, "Lobby")
	drain(annaCh)
	drain(bertCh)

	// A move from bert: the poller (anna) must not receive an EventMoved, but the
	// event listener (bert sees his own area's traffic) still does.
	w.Move(bert, 6, 6)
	for _, ev := range drain(annaCh) {
		if ev.Type == EventMoved {
			t.Error("poller received an EventMoved it does not need")
		}
	}

	// Chat is important and must still reach the poller.
	w.Chat(bert, "hello")
	gotChat := false
	for _, ev := range drain(annaCh) {
		if ev.Type == EventChat && ev.Detail == "hello" {
			gotChat = true
		}
	}
	if !gotChat {
		t.Error("poller missed a chat event")
	}

	// And ticks are filtered for the poller but delivered to the listener.
	time.Sleep(700 * time.Millisecond) // span at least one 2 Hz tick
	for _, ev := range drain(annaCh) {
		if ev.Type == EventTick {
			t.Error("poller received an EventTick it does not need")
		}
	}
	sawTick := false
	for _, ev := range drain(bertCh) {
		if ev.Type == EventTick {
			sawTick = true
		}
	}
	if !sawTick {
		t.Error("non-poller missed the world tick")
	}
}

func TestChatIsProximityFiltered(t *testing.T) {
	w := New()
	defer w.Close()

	anna, annaCh := w.Join("anna")
	bert, bertCh := w.Join("bert")
	carl, carlCh := w.Join("carl")
	dora, doraCh := w.Join("dora")

	w.EnterArea(anna, "lobby", 10, 10, "Lobby")
	w.EnterArea(bert, "lobby", 18, 10, "Lobby") // Chebyshev 8 → hears
	w.EnterArea(carl, "lobby", 19, 10, "Lobby") // Chebyshev 9 → silent
	w.EnterArea(dora, "kraftwerk", 10, 10, "Kraftwerk")
	drain(annaCh)
	drain(bertCh)
	drain(carlCh)
	drain(doraCh)

	w.Chat(anna, "hello")

	if !hasChat(drain(annaCh)) {
		t.Error("sender should hear their own message")
	}
	if !hasChat(drain(bertCh)) {
		t.Error("bert (distance 8) should hear the message")
	}
	if hasChat(drain(carlCh)) {
		t.Error("carl (distance 9) should NOT hear the message")
	}
	if hasChat(drain(doraCh)) {
		t.Error("dora (other area) should NOT hear the message")
	}
}

func hasChat(evs []Event) bool {
	for _, ev := range evs {
		if ev.Type == EventChat {
			return true
		}
	}
	return false
}

func TestEventChannelNeverBlocks(t *testing.T) {
	w := New()
	defer w.Close()

	anna, _ := w.Join("anna")
	bert, _ := w.Join("bert")
	w.EnterArea(anna, "lobby", 1, 1, "Lobby")
	w.EnterArea(bert, "lobby", 2, 1, "Lobby")

	// flood far beyond the buffer; must not deadlock
	done := make(chan struct{})
	go func() {
		for i := 0; i < eventBuffer*4; i++ {
			w.Move(anna, 1, 1)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("broadcast blocked on a full subscriber channel")
	}
}
