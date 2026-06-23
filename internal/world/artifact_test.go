package world

import "testing"

func TestClaimArtifactFirstWins(t *testing.T) {
	w := New()
	defer w.Close()
	var saved [][2]string
	w.SetArtifactPersist(func(id, owner string) { saved = append(saved, [2]string{id, owner}) })

	if _, claimed := w.ArtifactClaimed("durstbane"); claimed {
		t.Fatal("a fresh world should have nothing claimed")
	}
	if !w.ClaimArtifact("durstbane", "ada") {
		t.Fatal("the first claim should win")
	}
	if w.ClaimArtifact("durstbane", "bob") {
		t.Fatal("a second claim on the same legend must lose")
	}
	owner, claimed := w.ArtifactClaimed("durstbane")
	if !claimed || owner != "ada" {
		t.Fatalf("after claim: owner=%q claimed=%v, want ada/true", owner, claimed)
	}
	if len(saved) != 1 || saved[0] != [2]string{"durstbane", "ada"} {
		t.Fatalf("persist callback fired %v, want one (durstbane, ada)", saved)
	}
}

func TestLoadAndListArtifacts(t *testing.T) {
	w := New()
	defer w.Close()
	w.LoadArtifacts(map[string]string{"skypiercer": "zoe"})

	owner, claimed := w.ArtifactClaimed("skypiercer")
	if !claimed || owner != "zoe" {
		t.Fatalf("loaded owner=%q claimed=%v, want zoe/true", owner, claimed)
	}
	if h := w.ArtifactHolders(); h["skypiercer"] != "zoe" {
		t.Fatalf("holders = %v, want skypiercer→zoe", h)
	}
}
