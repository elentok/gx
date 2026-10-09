package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/nativerunner"
)

// liveProbe asks the real machine. herdr.Ping has no timeout of its own, so it
// is raced against one; a ping that loses keeps running until herdr answers.
var liveProbe = agentrunner.Probe{
	LookPath: exec.LookPath,
	PingHerdr: func(timeout time.Duration) error {
		done := make(chan error, 1)
		go func() { done <- herdr.Ping() }()
		select {
		case err := <-done:
			return err
		case <-time.After(timeout):
			return fmt.Errorf("herdr did not answer within %v", timeout)
		}
	},
}

// preflightAgentRunner resolves the agent-runner setting before the server
// starts. The server log is only opened inside server.New, so the outcome is
// appended to logPath directly; on error the caller exits without serving.
func preflightAgentRunner(setting string, p agentrunner.Probe, logPath string) (agentrunner.Choice, error) {
	choice, err := agentrunner.Select(setting, p)
	_ = os.MkdirAll(filepath.Dir(logPath), 0o700)
	if f, ferr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); ferr == nil {
		log := slog.New(slog.NewJSONHandler(f, nil))
		if err != nil {
			log.Error("agent-runner preflight failed", "setting", setting, "err", err)
		} else {
			log.Info("agent-runner selected", "setting", setting, "runner", string(choice))
		}
		_ = f.Close()
	}
	if err != nil {
		return "", fmt.Errorf("gx server: %w", err)
	}
	return choice, nil
}

// serverRunner is the runner the server reports host health for. Launches
// still go through herdr directly, so a native runner needs no setup yet.
func serverRunner(c agentrunner.Choice) agentrunner.Runner {
	if c == agentrunner.ChoiceHerdr {
		return herdrrunner.New()
	}
	return &nativerunner.Headless{}
}
