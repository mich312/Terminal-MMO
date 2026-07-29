package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

// A browser has no username to hand us, so the name is typed in — which makes
// it the one piece of player-supplied identity on the server. It has to be
// boring: no spaces to wreck a chat line, no markup to end up in a label, and
// short enough to float over a head.
func TestCleanName(t *testing.T) {
	ok := []string{"anna", "Bob", "markus_1", "a", "sixteen-chars-ok", "ÅsaK"}
	for _, in := range ok {
		if _, valid := CleanName(in); !valid {
			t.Errorf("CleanName(%q) rejected a reasonable name", in)
		}
	}
	bad := []string{
		"", "   ", "way-too-long-a-name-for-a-label",
		"has space", "sql'inject", "<script>", "semi;colon", "slash/es", "emoji🙂?",
	}
	for _, in := range bad {
		if got, valid := CleanName(in); valid {
			t.Errorf("CleanName(%q) accepted it as %q", in, got)
		}
	}
	if got, _ := CleanName("  anna  "); got != "anna" {
		t.Errorf("CleanName should trim surrounding space, got %q", got)
	}
}

// The client is embedded in the binary — there is no asset directory to forget
// to deploy. If the embed ever stops covering the JS, the page loads and then
// silently fails to start, so assert the whole set is served.
func TestServesTheEmbeddedClient(t *testing.T) {
	w := world.New()
	defer w.Close()
	srv := New(w, store.Open(""), nil)

	for _, path := range []string{
		"/", "/style.css", "/js/main.js", "/js/scene.js", "/js/field.js",
		"/js/props.js", "/js/actors.js", "/js/ui.js", "/js/input.js", "/js/net.js",
		"/vendor/three.module.min.js",
	} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s served an empty body", path)
		}
	}
}

// A websocket request without a usable name is refused before it ever reaches
// the world, so a blank or hostile name can't join.
func TestWebSocketRequiresAName(t *testing.T) {
	w := world.New()
	defer w.Close()
	srv := New(w, store.Open(""), nil)

	for _, q := range []string{"", "?name=", "?name=has%20space", "?name=<b>"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET /ws%s = %d, want 400", q, rec.Code)
		}
	}
}

// The vendored renderer never changes under a given URL, so it is cached hard;
// the game's own code must not be, or a redeploy leaves players on a stale
// client talking a protocol the server no longer speaks.
func TestCachePolicy(t *testing.T) {
	w := world.New()
	defer w.Close()
	srv := New(w, store.Open(""), nil)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/vendor/three.module.min.js", nil))
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("vendored asset Cache-Control = %q, want immutable", cc)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/js/main.js", nil))
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("game client Cache-Control = %q, want no-cache", cc)
	}
}

// The browser may only send keys the areas already act on. This is the same
// gate the HD client applies: a client that invents a key gets ignored rather
// than reaching into an area with something it never expected.
func TestOnlyMovementKeysBecomeKeyMessages(t *testing.T) {
	for _, key := range []string{"w", "a", "s", "d", "up", "down", "left", "right",
		"y", "u", "b", "n", "W", "shift+up", "shift+left"} {
		if _, ok := moveKeyMsg(key); !ok {
			t.Errorf("moveKeyMsg(%q) should be a movement key", key)
		}
	}
	for _, key := range []string{"", "q", "ctrl+c", "F1", "zzz", "e"} {
		if _, ok := moveKeyMsg(key); ok {
			t.Errorf("moveKeyMsg(%q) should not be treated as movement", key)
		}
	}
}

// The build palette the browser shows has to be the same list, in the same
// order, that the terminal build panel offers — the index is what gets sent
// back as a selection.
func TestBuildPaletteMatchesTheGameCatalog(t *testing.T) {
	names := placeableNames()
	if len(names) != len(game.Placeables) {
		t.Fatalf("build palette has %d entries, the catalog has %d",
			len(names), len(game.Placeables))
	}
	for i, p := range game.Placeables {
		if names[i] != p.Name {
			t.Errorf("build palette[%d] = %q, want %q", i, names[i], p.Name)
		}
	}
}
