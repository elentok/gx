package server

import (
	"path/filepath"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/devcheckout"
	"github.com/elentok/gx/server/devvite"
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
	on := next.WebDevFrontend && root != ""
	if !on {
		// The supervisor reports off only after Vite has stopped; until then
		// the proxy would answer 502.
		s.web.UseBuiltUI()
	}
	s.vite.Set(on, root)
}

func (h reloadHost) ApplyDevMode(_, next Settings) { h.s.applyDevMode(next) }
