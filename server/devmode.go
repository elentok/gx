package server

import (
	"net/http"
	"path/filepath"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/devcheckout"
	"github.com/elentok/gx/server/devvite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/version"
)

// newDevVite builds the Vite supervisor, off until the settings ask for it. The
// web UI follows its state: Vite's proxy while it runs, the built files otherwise.
func (s *Server) newDevVite() error {
	sup, err := devvite.New(devvite.Config{
		LogDir:   filepath.Join(s.cfg.StateDir, "logs", "vite"),
		Env:      s.cfg.Env,
		Launcher: s.cfg.ViteLauncher,
		Observer: func(snap devvite.Snapshot) {
			if snap.State == devvite.StateRunning {
				s.web.UseViteProxy(snap.Port)
			} else {
				s.web.UseBuiltUI()
			}
		},
	})
	if err != nil {
		return err
	}
	s.vite = sup
	s.applyDevMode(*s.settings.Load())
	return nil
}

// applyDevMode tells the supervisor what is wanted. It returns at once, so it
// is safe under the reload lock.
func (s *Server) applyDevMode(next Settings) {
	home := harness.LookupEnv(s.cfg.Env, "HOME")
	root := devcheckout.Resolve(next.WebDevDir, version.DevDir, home)
	on := next.WebDevFrontend && root != "" && s.web.Listener() != nil
	if !on {
		// The supervisor reports off only after Vite has stopped; until then
		// the proxy would answer 502.
		s.web.UseBuiltUI()
	}
	s.vite.Set(on, root)
}

// devRoutes is the Vite restart, which only the web UI offers (ADR 0028).
func (s *Server) devRoutes() []wire.Route {
	return []wire.Route{{Pattern: "POST /v1/dev/restart", Web: wire.WebWrite, WebHandler: s.devRestart}}
}

func (s *Server) devRestart(w http.ResponseWriter, _ *http.Request) {
	if err := s.vite.Restart(); err != nil {
		wire.WriteJSON(w, wire.DevRestartResult{Refused: true, Reason: wire.ReasonDevNotDown,
			Message: "Vite can only be restarted while it is down"})
		return
	}
	wire.WriteJSON(w, wire.DevRestartResult{})
}

func (h reloadHost) ApplyDevMode(_, next Settings) { h.s.applyDevMode(next) }
