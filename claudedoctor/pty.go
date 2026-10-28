package claudedoctor

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner"
)

var ptyChecks = []Check{
	{Runner: "pty", Name: "pid-status", Mode: Fixture, Session: "pty-status", Run: checkPidStatus},
	{Runner: "pty", Name: "trust-screen", Mode: Fixture, Session: "pty-trust", Run: checkTrustScreen},
	{Runner: "pty", Name: "permission-dialog", Mode: Fixture, Session: "pty-dialog", Run: checkPermissionDialog},
	// No recorded session has hit a rate limit, so there is no banner to
	// write a screen rule against; PTY RateLimit reports nothing until then.
	{Runner: "pty", Name: "rate-limit-screen", Mode: Fixture, SkipReason: "banner never captured"},
	{Runner: "pty", Name: "holder-survives-kickstart", Mode: Live, Run: checkHolderSurvivesKickstart},
}

// checkHolderSurvivesKickstart starts `gx agent-holder` detached from a
// launchd job that is then kickstarted; holder.sock must still answer. The
// holder runs sleep, not claude: only the holder's survival is in question.
func checkHolderSurvivesKickstart(in Input) (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return err.Error(), false
	}
	// Short dir: holder.sock lives in it and socket paths are length-capped.
	dir, err := os.MkdirTemp("", "gxd")
	if err != nil {
		return err.Error(), false
	}
	defer os.RemoveAll(dir)

	start := fmt.Sprintf(`%s %q agent-holder --session-id doctor --dir %q -- sleep 600 </dev/null >/dev/null 2>&1`,
		setsid, exe, dir)
	return holderSurvives(dir, func() (int, error) { return kickstartJob(in.Env, dir, start) })
}

// holderSurvives runs kickstart, which starts a holder serving dir and
// kickstarts its launchd job, then checks dir's holder.sock still answers.
func holderSurvives(dir string, kickstart func() (int, error)) (string, bool) {
	pid, err := kickstart()
	if pid > 0 {
		defer syscall.Kill(pid, syscall.SIGKILL)
	}
	if err != nil {
		return err.Error(), false
	}
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return fmt.Sprintf("holder %d after kickstart -k: %v", pid, err), false
	}
	defer c.Stop()
	if _, _, err := c.Status(); err != nil {
		return fmt.Sprintf("holder %d status after kickstart -k: %v", pid, err), false
	}
	return fmt.Sprintf("holder %d survived kickstart -k and answers status", pid), true
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

// checkPermissionDialog parses recorded frames with the parser the web UI
// approves through: a release that moves the dialog markers leaves no
// approvable dialog, and gx would silently stop offering approval.
func checkPermissionDialog(in Input) (string, bool) {
	for i, frame := range in.Frames {
		d := claudeui.ParseDialog(strings.Split(frame, "\n"))
		if d.Approvable && d.ID != "" {
			return fmt.Sprintf("frame %d parses as %q dialog", i, d.Header), true
		}
	}
	return fmt.Sprintf("no approvable permission dialog in %d frames", len(in.Frames)), false
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
