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
)

// RestartCheckOutput is what `gx restart-check` prints. A binary without the
// command exits non-zero with an unknown-command error; one that ran exits zero
// even when the config or a dry run failed (the failure is in the JSON).
type RestartCheckOutput struct {
	Version  string                `json:"version"`
	Config   RestartCheckConfig    `json:"config"`
	Projects []RestartCheckProject `json:"projects"`
}

type RestartCheckConfig struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type RestartCheckProject struct {
	Name  string   `json:"name"`
	OK    bool     `json:"ok"`
	Error string   `json:"error,omitempty"`
	Files []string `json:"files"`
}

// Restart check statuses.
const (
	RestartCheckOK         = "ok"
	RestartCheckUnreadable = "unreadable"
	RestartCheckTooOld     = "too_old"
)

// RestartCheckReport is the restart check route's answer. Config and Projects
// are set only when Status is "ok".
type RestartCheckReport struct {
	Status         string                `json:"status"`
	Error          string                `json:"error,omitempty"`
	Hint           string                `json:"hint,omitempty"`
	Binary         string                `json:"binary"`
	RunningVersion string                `json:"running_version"`
	NewVersion     string                `json:"new_version,omitempty"`
	Config         RestartCheckConfig    `json:"config"`
	Projects       []RestartCheckProject `json:"projects"`
	Agents         int                   `json:"agents"`
	Instance       string                `json:"instance"`
	RestartLog     string                `json:"restart_log"`
}

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
func (s *Server) restartCheck(ctx context.Context) RestartCheckReport {
	rep := RestartCheckReport{
		Binary:         s.restartBinary(),
		RunningVersion: s.cfg.Build,
		Projects:       []RestartCheckProject{},
		Agents:         s.runningAgents(),
		Instance:       s.instanceID,
		RestartLog:     migratefail.RestartLogPath(s.cfg.StateDir),
	}
	out, status, detail := runRestartCheckBinary(ctx, rep.Binary, s.cfg.Env)
	rep.Status, rep.Error = status, detail
	switch status {
	case RestartCheckOK:
		rep.NewVersion, rep.Config = out.Version, out.Config
		if out.Projects != nil {
			rep.Projects = out.Projects
		}
	case RestartCheckTooOld:
		rep.Hint = "that binary has no restart check; use `gx server restart` instead"
	}
	return rep
}

func runRestartCheckBinary(ctx context.Context, bin string, env []string) (RestartCheckOutput, string, string) {
	var out RestartCheckOutput
	if bin == "" {
		return out, RestartCheckUnreadable, "no binary to restart with"
	}
	if _, err := os.Stat(bin); err != nil {
		return out, RestartCheckUnreadable, err.Error()
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
			return out, RestartCheckTooOld, msg
		}
		if msg == "" {
			msg = err.Error()
		}
		return out, RestartCheckUnreadable, msg
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return out, RestartCheckUnreadable, "unreadable output: " + err.Error()
	}
	return out, RestartCheckOK, ""
}
