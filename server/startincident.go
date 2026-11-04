package server

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// StartIncidentPath is the note `gx server start` leaves when launchd refused
// to start the server or it never answered. The server is down at that point,
// so the next server to come up files the ticket.
func StartIncidentPath(stateDir string) string {
	return filepath.Join(stateDir, "server-start-incident.md")
}

// fileStartIncident files a diagnose ticket for a start incident left by the
// previous run, then removes the note. Like a CPU spike it needs evolve
// maintainer mode and a gx project; without them the note stays for a later
// start.
func (s *Server) fileStartIncident(now time.Time) {
	path := StartIncidentPath(s.cfg.StateDir)
	note, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		s.log.Warn("start incident: read note", "err", err)
		return
	}
	if s.settings.Load().Evolve != config.EvolveMaintainer {
		s.log.Info("start incident: not filed, needs evolve maintainer mode", "note", path)
		return
	}
	if _, err := s.projects.Dir(projects.OutboxProject); err != nil {
		s.log.Info("start incident: not filed, needs a gx project", "err", err)
		return
	}
	res, err := s.oneOff(wire.OneOffRequest{
		Prompt:  startIncidentPrompt(string(note), path),
		Project: projects.OutboxProject,
		Type:    string(schema.TypeDiagnose),
		Name:    "server-start-" + now.Format("20060102-1504"),
		Unique:  "server-start-incident",
		Front:   true,
		Notify:  true,
	})
	switch {
	case err != nil:
		s.log.Warn("start incident: file ticket", "err", err)
		return
	case res.Refused:
		s.log.Info("start incident: ticket refused", "reason", res.Reason, "message", res.Message)
	default:
		s.log.Info("start incident: ticket filed", "address", res.Address)
	}
	if err := os.Remove(path); err != nil {
		s.log.Warn("start incident: remove note", "err", err)
	}
}

func startIncidentPrompt(note, path string) string {
	return fmt.Sprintf(`The gx server failed to start under launchd. Find out why launchd refused to spawn it, and why a restart that had worked for weeks stopped working.

What gx recorded (from %s):

%s

Also see server-start-failure.log and server.stderr in the same directory. Known so far: "gx server install" (bootout + bootstrap) fixed it by hand, rebuilding the binary with a new cdhash did not reproduce it, and gx now reloads the job itself when it sees "spawn failed". Look for what changed in launchd's view of the job (launch constraints, the plist, a macOS update) and whether the reload fallback is enough.`, path, note)
}
