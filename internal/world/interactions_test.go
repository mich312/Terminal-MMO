package world

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// waitFor drains ch (past periodic ticks) until an event of the given type
// arrives, or fails on timeout.
func waitFor(t *testing.T, ch <-chan Event, typ EventType) Event {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event %d", typ)
		}
	}
}

func TestWhisperReachesTargetOnly(t *testing.T) {
	w := New()
	defer w.Close()
	_, _ = w.Join("alice")
	_, bob := w.Join("bob")

	if !w.Whisper("alice", "bob", "psst") {
		t.Fatal("whisper to online player should succeed")
	}
	ev := waitFor(t, bob, EventWhisper)
	if ev.Player != "alice" || ev.Target != "bob" || ev.Detail != "psst" {
		t.Fatalf("unexpected whisper event: %+v", ev)
	}
	if w.Whisper("alice", "ghost", "anyone?") {
		t.Fatal("whisper to offline player should fail")
	}
}

func TestEmoteIsProximityScoped(t *testing.T) {
	w := New()
	defer w.Close()
	_, _ = w.Join("alice")
	_, bob := w.Join("bob")
	w.EnterArea("alice", "room", 0, 0, "Room")
	w.EnterArea("bob", "room", 1, 1, "Room")

	w.Emote("alice", "waves")
	ev := waitFor(t, bob, EventEmote)
	if ev.Player != "alice" || ev.Detail != "waves" {
		t.Fatalf("unexpected emote event: %+v", ev)
	}
}

func TestSetColor(t *testing.T) {
	w := New()
	defer w.Close()
	_, _ = w.Join("alice")
	want := lipgloss.Color("#123456")
	if !w.SetColor("alice", want) {
		t.Fatal("SetColor on present player should succeed")
	}
	if self, _ := w.Self("alice"); self.Color != want {
		t.Fatalf("color = %v, want %v", self.Color, want)
	}
	if w.SetColor("ghost", want) {
		t.Fatal("SetColor on absent player should fail")
	}
}
