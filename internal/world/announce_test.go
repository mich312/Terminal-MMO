package world

import "testing"

// Announce is the town crier: every session hears it, whatever area they're
// in — unlike proximity chat, which stays local.
func TestAnnounceReachesEveryArea(t *testing.T) {
	w := New()
	defer w.Close()

	anna, annaCh := w.Join("anna")
	bert, bertCh := w.Join("bert")
	w.EnterArea(anna, "arcade", 3, 3, "Arcade")
	w.EnterArea(bert, "wilds", 100, -40, "The Wilds")
	drain(annaCh)
	drain(bertCh)

	w.Announce("★ anna set the arcade record on Snake — 12")

	for name, ch := range map[string]<-chan Event{"anna": annaCh, "bert": bertCh} {
		heard := false
		for _, ev := range drain(ch) {
			if ev.Type == EventAnnounce && ev.Detail != "" {
				heard = true
			}
		}
		if !heard {
			t.Fatalf("%s did not hear the announcement", name)
		}
	}
}
