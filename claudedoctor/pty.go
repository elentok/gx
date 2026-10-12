package claudedoctor

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner"
)

var ptyChecks = []Check{
	{Runner: "pty", Name: "pid-status", Mode: Fixture, Session: "pty-status", Run: checkPidStatus},
	{Runner: "pty", Name: "trust-screen", Mode: Fixture, Session: "pty-trust", Run: checkTrustScreen},
	// No recorded session has hit a rate limit, so there is no banner to
	// write a screen rule against; PTY RateLimit reports nothing until then.
	{Runner: "pty", Name: "rate-limit-screen", Mode: Fixture, SkipReason: "banner never captured"},
}

// checkPidStatus reads the pid status file snapshots the PTY runner relies
// on: each has a status, and waitingFor is set exactly while waiting.
func checkPidStatus(in Input) (string, bool) {
	if len(in.Snapshots) == 0 {
		return "no pid status snapshots", false
	}
	var waitingFor []string
	for i, snap := range in.Snapshots {
		s, err := nativerunner.ParsePidStatus(snap)
		if err != nil {
			return fmt.Sprintf("snapshot %d: %v", i, err), false
		}
		if s.Status == nativerunner.ClaudeStarting {
			return fmt.Sprintf("snapshot %d has no status", i), false
		}
		if (s.Status == nativerunner.ClaudeWaiting) != (s.WaitingFor != "") {
			return fmt.Sprintf("snapshot %d: status %q with waitingFor %q", i, s.Status, s.WaitingFor), false
		}
		if s.WaitingFor != "" {
			waitingFor = append(waitingFor, s.WaitingFor)
		}
	}
	if len(waitingFor) == 0 {
		return "no waiting snapshot", false
	}
	return "waitingFor: " + strings.Join(waitingFor, ", "), true
}

// checkTrustScreen matches the trust dialog screen rule on recorded frames.
func checkTrustScreen(in Input) (string, bool) {
	for i, frame := range in.Frames {
		if claudeui.ClassifyStartup(claudeui.Normalize(strings.Split(frame, "\n")...)) == claudeui.StartupTrust {
			return fmt.Sprintf("frame %d is the trust dialog", i), true
		}
	}
	return fmt.Sprintf("no trust dialog in %d frames", len(in.Frames)), false
}
