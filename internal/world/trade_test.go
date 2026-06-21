package world

import (
	"testing"
	"time"
)

// nextTrade waits briefly for the next EventTrade on ch and returns its phase.
func nextTrade(t *testing.T, ch <-chan Event) string {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == EventTrade {
				return ev.Detail
			}
		case <-deadline:
			t.Fatal("timed out waiting for a trade event")
		}
	}
}

// twoTraders joins two players standing next to each other in the wilds.
func twoTraders(t *testing.T) (*World, <-chan Event, <-chan Event) {
	t.Helper()
	w := New()
	t.Cleanup(w.Close)
	_, ca := w.Join("a")
	_, cb := w.Join("b")
	w.EnterArea("a", "wilds", 0, 0, "")
	w.EnterArea("b", "wilds", 2, 0, "")
	return w, ca, cb
}

func TestTradeHappyPath(t *testing.T) {
	w, ca, cb := twoTraders(t)

	if err := w.RequestTrade("a", "b"); err != nil {
		t.Fatalf("request: %v", err)
	}
	if ph := nextTrade(t, cb); ph != TradeRequest {
		t.Fatalf("b should see a request, got %q", ph)
	}
	if err := w.AcceptTrade("b", "a"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if ph := nextTrade(t, ca); ph != TradeOpen {
		t.Fatalf("a should see the table open, got %q", ph)
	}
	_ = nextTrade(t, cb) // b's open

	if err := w.SetOffer("a", map[string]int{"crystal": 2}); err != nil {
		t.Fatalf("offer a: %v", err)
	}
	if err := w.SetOffer("b", map[string]int{"nugget": 3, "wood": 0}); err != nil {
		t.Fatalf("offer b: %v", err)
	}

	w.SetReady("a", true)
	if _, ok := w.TakeCompletedTrade("a"); ok {
		t.Fatal("trade completed with only one side ready")
	}
	w.SetReady("b", true)

	ca2, ok := w.TakeCompletedTrade("a")
	if !ok {
		t.Fatal("a has no completed trade")
	}
	if ca2.With != "b" || ca2.Gave["crystal"] != 2 || ca2.Got["nugget"] != 3 {
		t.Fatalf("a's delta wrong: %+v", ca2)
	}
	if _, zero := ca2.Got["wood"]; zero {
		t.Fatalf("zero-count offer should be dropped: %+v", ca2.Got)
	}
	cb2, ok := w.TakeCompletedTrade("b")
	if !ok || cb2.Gave["nugget"] != 3 || cb2.Got["crystal"] != 2 {
		t.Fatalf("b's delta wrong: %+v", cb2)
	}
	if w.Trading("a") || w.Trading("b") {
		t.Fatal("table should be closed after the swap")
	}
}

func TestTradeReadyResetsOnChange(t *testing.T) {
	w, _, _ := twoTraders(t)
	w.RequestTrade("a", "b")
	w.AcceptTrade("b", "a")
	w.SetOffer("a", map[string]int{"crystal": 1})
	w.SetReady("a", true)

	snap, _ := w.TradeSnapshot("b")
	if !snap.ThemReady {
		t.Fatal("b should see a as ready")
	}
	// a changes the offer — both ready flags must clear.
	w.SetOffer("a", map[string]int{"crystal": 2})
	snap, _ = w.TradeSnapshot("b")
	if snap.ThemReady || snap.YouReady {
		t.Fatalf("changing an offer must clear ready flags: %+v", snap)
	}
}

func TestTradeProximityAndBusy(t *testing.T) {
	w := New()
	t.Cleanup(w.Close)
	w.Join("a")
	w.Join("b")
	w.Join("c")
	w.EnterArea("a", "wilds", 0, 0, "")
	w.EnterArea("b", "wilds", 40, 40, "") // far away
	w.EnterArea("c", "wilds", 1, 0, "")   // adjacent to a

	if err := w.RequestTrade("a", "b"); err != errFar {
		t.Fatalf("far request should be rejected, got %v", err)
	}
	if err := w.RequestTrade("a", "a"); err != errSelf {
		t.Fatalf("self request should be rejected, got %v", err)
	}
	// a trades with c; a is then busy.
	w.RequestTrade("a", "c")
	w.AcceptTrade("c", "a")
	w.EnterArea("b", "wilds", 1, 1, "") // bring b close
	if err := w.RequestTrade("b", "a"); err != errBusy {
		t.Fatalf("request to a busy trader should fail, got %v", err)
	}
}

func TestTradeAutoCancelOnLeave(t *testing.T) {
	w, ca, _ := twoTraders(t)
	w.RequestTrade("a", "b")
	w.AcceptTrade("b", "a")
	nextTrade(t, ca) // open

	w.Leave("b")
	if w.Trading("a") {
		t.Fatal("a's table should auto-cancel when b leaves")
	}
	if ph := nextTrade(t, ca); ph != TradeCancel {
		t.Fatalf("a should be told the trade cancelled, got %q", ph)
	}
}

func TestTradeDecline(t *testing.T) {
	w, ca, _ := twoTraders(t)
	if err := w.RequestTrade("a", "b"); err != nil {
		t.Fatal(err)
	}
	w.DeclineTrade("b", "a")
	if ph := nextTrade(t, ca); ph != TradeDeclined {
		t.Fatalf("a should hear the decline, got %q", ph)
	}
	if err := w.AcceptTrade("b", "a"); err != errNoOffer {
		t.Fatalf("a declined request can't be accepted, got %v", err)
	}
}
