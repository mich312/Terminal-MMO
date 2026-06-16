// Durst World — an SSH terminal MUD for the Durst HQ.
//
//	go run ./cmd/durstworld
//	ssh -p 2222 anna@localhost
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"

	"github.com/durst-group/durstworld/internal/game"
	"github.com/durst-group/durstworld/internal/store"
	"github.com/durst-group/durstworld/internal/ui"
	"github.com/durst-group/durstworld/internal/world"

	// areas register themselves with the game registry
	_ "github.com/durst-group/durstworld/internal/areas/democenter"
	_ "github.com/durst-group/durstworld/internal/areas/kraftwerk"
	_ "github.com/durst-group/durstworld/internal/areas/lobby"
	_ "github.com/durst-group/durstworld/internal/areas/presentation"
	_ "github.com/durst-group/durstworld/internal/areas/stub"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "2222"
	}

	w := world.New()
	st := store.Open("./data/durstworld.db")

	srv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort("0.0.0.0", port)),
		// persistent Ed25519 host key, generated on first run
		wish.WithHostKeyPath(".ssh/host_key"),
		// no auth options at all = accept any username, no password
		wish.WithMiddleware(
			bm.Middleware(teaHandler(w, st)),
			activeterm.Middleware(), // require a TTY
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalf("could not create server: %v", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Durst World listening on :%s — ssh -p %s you@localhost", port, port)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Printf("server error: %v", err)
			done <- syscall.SIGTERM
		}
	}()

	<-done
	log.Println("shutting down Durst World…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	w.Close()
	_ = st.Close()
}

// teaHandler creates one bubbletea program per SSH session. The session's
// username becomes the player name (deduplicated by the world); a watchdog
// goroutine cleans up world presence however the session ends.
func teaHandler(w *world.World, st store.Store) bm.Handler {
	return func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		name, events := w.Join(s.User())
		visit := st.RecordVisit(name)
		log.Printf("%s connected (visit #%d)", name, visit.VisitCount)

		go func() {
			<-s.Context().Done()
			w.Leave(name)
			st.RecordDisconnect(name)
			log.Printf("%s disconnected", name)
		}()

		ctx := &game.Ctx{
			World: w,
			Store: st,
			Name:  name,
			// per-session renderer: auto-detects the client's terminal and
			// downsamples truecolor → 256 → 16 as needed.
			Theme: ui.NewTheme(bm.MakeRenderer(s)),
		}
		m := game.NewModel(ctx, events, visit)
		return m, []tea.ProgramOption{tea.WithAltScreen()}
	}
}
