package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/server/wire"
)

const restartCheckTimeout = 2 * time.Minute

func newInstanceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// restartBinary is the binary a restart would start.
func (s *Server) restartBinary() string {
	if s.cfg.RestartBinary != nil {
		return s.cfg.RestartBinary()
	}
	exe, _ := os.Executable()
	return exe
}

// runningAgents counts loop agents and live interactive sessions.
func (s *Server) runningAgents() int {
	n := s.registry.count()
	sessions, _ := s.listSessions()
	for _, si := range sessions {
		if si.Status.State != SessionExited {
			n++
		}
	}
	return n
}

func (s *Server) restartCheckRead(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.restartCheck(r.Context()))
}

// restartCheck runs the restart check from the binary a restart would start.
// It never touches this server.
func (s *Server) restartCheck(ctx context.Context) wire.RestartCheckReport {
	rep := wire.RestartCheckReport{
		Binary:         s.restartBinary(),
		RunningVersion: s.cfg.Build,
		Projects:       []wire.RestartCheckProject{},
		Agents:         s.runningAgents(),
		Instance:       s.instanceID,
		RestartLog:     migratefail.RestartLogPath(s.cfg.StateDir),
	}
	out, status, detail := runRestartCheckBinary(ctx, rep.Binary, s.cfg.Env)
	rep.Status, rep.Error = status, detail
	switch status {
	case wire.RestartCheckOK:
		rep.NewVersion, rep.Config = out.Version, out.Config
		if out.Projects != nil {
			rep.Projects = out.Projects
		}
	case wire.RestartCheckTooOld:
		rep.Hint = "that binary has no restart check; use `gx server restart` instead"
	}
	return rep
}

func runRestartCheckBinary(ctx context.Context, bin string, env []string) (wire.RestartCheckOutput, string, string) {
	var out wire.RestartCheckOutput
	if bin == "" {
		return out, wire.RestartCheckUnreadable, "no binary to restart with"
	}
	if _, err := os.Stat(bin); err != nil {
		return out, wire.RestartCheckUnreadable, err.Error()
	}
	ctx, cancel := context.WithTimeout(ctx, restartCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "restart-check")
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		var exit *exec.ExitError
		if errors.As(err, &exit) && strings.Contains(msg, "unknown command") {
			return out, wire.RestartCheckTooOld, msg
		}
		if msg == "" {
			msg = err.Error()
		}
		return out, wire.RestartCheckUnreadable, msg
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return out, wire.RestartCheckUnreadable, "unreadable output: " + err.Error()
	}
	return out, wire.RestartCheckOK, ""
}
