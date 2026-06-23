// Durst World — an SSH terminal MUD for the Durst HQ.
//
//	go run ./cmd/durstworld
//	ssh -p 2222 anna@localhost
package main

import (
	"context"
	"errors"
	"flag"
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
	"github.com/durst-group/durstworld/internal/wildlife"
	"github.com/durst-group/durstworld/internal/world"
	"github.com/durst-group/durstworld/internal/worldgen"

	// areas register themselves with the game registry
	_ "github.com/durst-group/durstworld/internal/areas/arcade"
	_ "github.com/durst-group/durstworld/internal/areas/bomberman"
	_ "github.com/durst-group/durstworld/internal/areas/breakout"
	_ "github.com/durst-group/durstworld/internal/areas/cave"
	_ "github.com/durst-group/durstworld/internal/areas/chess"
	_ "github.com/durst-group/durstworld/internal/areas/democenter"
	_ "github.com/durst-group/durstworld/internal/areas/doom"
	_ "github.com/durst-group/durstworld/internal/areas/grove"
	_ "github.com/durst-group/durstworld/internal/areas/kraftwerk"
	_ "github.com/durst-group/durstworld/internal/areas/lobby"
	_ "github.com/durst-group/durstworld/internal/areas/maze"
	_ "github.com/durst-group/durstworld/internal/areas/pong"
	_ "github.com/durst-group/durstworld/internal/areas/presentation"
	_ "github.com/durst-group/durstworld/internal/areas/snake"
	_ "github.com/durst-group/durstworld/internal/areas/sokoban"
	_ "github.com/durst-group/durstworld/internal/areas/tetris"
	_ "github.com/durst-group/durstworld/internal/areas/twenty48"
	_ "github.com/durst-group/durstworld/internal/areas/vault"
	"github.com/durst-group/durstworld/internal/areas/wilds"
)

func main() {
	styleName := flag.String("style", "default", "HD art style: default | gameboy | neon")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "2222"
	}

	// One server-wide art style for the HD/sixel renderer, resolved at startup.
	style := game.StyleByName(*styleName)
	log.Printf("HD art style: %s", style.Palette.Name)

	w := world.New()
	st := store.Open("./data/durstworld.db")

	// Restore user-authored presentation decks and keep them saved across edits.
	for _, d := range st.LoadDecks() {
		w.LoadDeck(d.ID, d.Owner, d.Title, d.Source, time.Unix(d.Created, 0))
	}
	w.SetDeckPersist(func(d world.Deck) {
		st.SaveDeck(d.ID, d.Owner, d.Title, d.Source, d.Created.Unix())
	})
	w.SetDeckRemove(st.DeleteDeck)

	// Restore shared co-op gate progress (contribution pools + which are open).
	w.LoadGates(st.LoadGateWorld())
	w.SetGatePersist(st.SaveGateWorld)

	// Restore the shared placements layer (player-built structures) and keep it
	// saved as people build and demolish.
	for _, p := range st.LoadPlacements() {
		w.LoadPlacements([]world.Placement{{X: p.X, Y: p.Y, Kind: p.Kind, Owner: p.Owner, State: p.State}})
	}
	w.SetPlacementPersist(
		func(p world.Placement) {
			st.AddPlacement(store.Placement{X: p.X, Y: p.Y, Kind: p.Kind, Owner: p.Owner,
				State: p.State, Created: time.Now().Unix()})
		},
		st.RemovePlacement,
	)

	// Restore the shared land claims (deeded settlement plots) and keep them saved
	// as people claim, refresh and release.
	claims := make([]world.Claim, 0)
	for _, c := range st.LoadClaims() {
		claims = append(claims, world.Claim{PlotID: c.PlotID, Owner: c.Owner,
			MinX: c.MinX, MinY: c.MinY, MaxX: c.MaxX, MaxY: c.MaxY, LastTouch: c.LastTouch})
	}
	w.LoadClaims(claims)
	w.SetClaimPersist(
		func(c world.Claim) {
			st.SaveClaim(store.Claim{PlotID: c.PlotID, Owner: c.Owner,
				MinX: c.MinX, MinY: c.MinY, MaxX: c.MaxX, MaxY: c.MaxY, LastTouch: c.LastTouch})
		},
		st.RemoveClaim,
	)

	// Restore cleared terrain (felled/quarried cells) and keep it saved as people
	// clear ground and as it regrows.
	cleared := make([]world.Cleared, 0)
	for _, c := range st.LoadCleared() {
		cleared = append(cleared, world.Cleared{X: c.X, Y: c.Y, Owner: c.Owner, LastTouch: c.LastTouch})
	}
	w.LoadCleared(cleared)
	w.SetClearPersist(
		func(c world.Cleared) {
			st.SaveCleared(store.Cleared{X: c.X, Y: c.Y, Owner: c.Owner, LastTouch: c.LastTouch})
		},
		st.RemoveCleared,
	)

	// Combat: a knocked-out player revives at the hub spawn beside the HQ gate
	// (docs/WEAPON_PLAN.md). The world's tick loop does the revive; it only needs
	// to know where, which is area geography the game layer owns.
	w.SetRespawn(func() (string, int, int) {
		return "wilds", worldgen.GateX + 2, worldgen.GateY + 2
	})

	// Legends: the shared registry of which unique weapons have been claimed, so a
	// found artifact never reappears and persists across restarts.
	w.LoadArtifacts(st.LoadArtifacts())
	w.SetArtifactPersist(st.SaveArtifact)

	// Wildlife: one server-side stepper drives the live herd in the Wilds, on the
	// same overworld seed every session sees. It spawns near online players and
	// reclaims animals when nobody is around, so the population tracks who's
	// connected rather than the size of the infinite map.
	wlStop := make(chan struct{})
	go wildlife.New(w, worldgen.New(wilds.Seed), st).Run(wlStop)

	srv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort("0.0.0.0", port)),
		// persistent Ed25519 host key, generated on first run
		wish.WithHostKeyPath(".ssh/host_key"),
		// no auth options at all = accept any username, no password
		wish.WithMiddleware(
			bm.Middleware(teaHandler(w, st)),
			// HD ("real pixel" sixel/kitty) mode is the default; `ssh -t … glyph`
			// opts back into the bubbletea client. Sits inside activeterm so it has
			// a PTY.
			hdMiddleware(w, st, style),
			activeterm.Middleware(), // require a TTY
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalf("could not create server: %v", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Durst World listening on :%s — ssh -p %s you@localhost (HD by default; classic glyph client: ssh -t -p %s you@localhost glyph)", port, port, port)
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
	close(wlStop)
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
		setupAvatar(w, st, name)
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
			Theme:      ui.NewTheme(bm.MakeRenderer(s)),
			Inventory:  st.LoadInventory(name),
			Hats:       st.LoadHats(name),
			Compendium: st.LoadCompendium(name),
			FixedGates: st.LoadPersonalGates(name),
		}
		m := game.NewModel(ctx, events, visit)
		return m, []tea.ProgramOption{tea.WithAltScreen()}
	}
}
