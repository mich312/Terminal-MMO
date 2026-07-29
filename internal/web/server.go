package web

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/coder/websocket"

	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/world"
)

//go:embed static
var staticFS embed.FS

// Server serves the browser client and its websocket, alongside (never instead
// of) the SSH server. It holds no game state of its own: every session it
// accepts joins the same world the terminal clients are already in.
type Server struct {
	world  *world.World
	store  store.Store
	mux    *http.ServeMux
	origin []string
}

// New builds the HTTP handler. origins restricts which page origins may open a
// websocket; empty means same-origin only, which is what you want unless the
// client is served from somewhere else.
func New(w *world.World, st store.Store, origins []string) *Server {
	s := &Server{world: w, store: st, mux: http.NewServeMux(), origin: origins}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// The assets are embedded at build time, so this can only fail if the
		// binary was built wrong — worth being loud about.
		panic("web: embedded assets missing: " + err.Error())
	}
	files := http.FileServer(http.FS(sub))
	s.mux.Handle("/", cacheControl(files))
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.HandleFunc("/healthz", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("ok"))
	})
	return s
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(rw, r) }

// cacheControl keeps the vendored renderer cacheable while the game's own code
// stays fresh, so a redeploy doesn't leave players on a stale client.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/vendor/"):
			rw.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			rw.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(rw, r)
	})
}

// handleWS upgrades a connection and runs one browser session on it.
func (s *Server) handleWS(rw http.ResponseWriter, r *http.Request) {
	name, ok := CleanName(r.URL.Query().Get("name"))
	if !ok {
		http.Error(rw, "a name of 1–16 letters, digits, - or _ is required", http.StatusBadRequest)
		return
	}
	opts := &websocket.AcceptOptions{
		OriginPatterns:     s.origin,
		CompressionMode:    websocket.CompressionContextTakeover,
		InsecureSkipVerify: len(s.origin) == 1 && s.origin[0] == "*",
	}
	conn, err := websocket.Accept(rw, r, opts)
	if err != nil {
		log.Printf("web: websocket accept failed: %v", err)
		return
	}
	// The scene messages are small but frequent; the default read limit only
	// bounds what the *client* may send us, which is tiny.
	conn.SetReadLimit(64 * 1024)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	defer func() {
		if rec := recover(); rec != nil {
			// One player's session must never take the server — and everyone
			// else's world — down with it.
			log.Printf("web: session for %q panicked: %v", name, rec)
			_ = conn.Close(websocket.StatusInternalError, "internal error")
		}
	}()

	runSession(ctx, conn, s.world, s.store, name)
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

// CleanName validates a browser-supplied player name. SSH gets identity for
// free — the username you connect with is who you are. The browser has no such
// thing, so a name is typed in, and this is the only gate on it: it must look
// like a name, and it must not be long enough to wreck a chat log or a floating
// label. The world's own Join then deduplicates it, exactly as it does for two
// SSH clients claiming the same username.
func CleanName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 16 {
		return "", false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return "", false
		}
	}
	return name, true
}

// ListenAndServe runs the browser server until the context is cancelled, then
// shuts it down gracefully so in-flight sessions get a chance to close cleanly.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		// No write timeout: a websocket session is a long-lived connection and
		// any deadline here would sever players mid-game.
	}
	errc := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errc <- err
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	}
}
