package ralphloop

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

const resolvedSharedContent = "shared resolved for D\n"

var firstTimestampRe = regexp.MustCompile(`"timestamp":"([^"]+)"`)

// recordLaunchPrompt prepends text as the transcript's first user record, the
// way a live claude records a delivered launch command. Without it the launch
// proof (claudeui.CheckLaunchDelivered) never sees the command arrive and
// resends the prompt until its grace runs out.
func recordLaunchPrompt(home, cwd, sessionID, text string) error {
	path := transcript.PathIn(home, cwd, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content, err := json.Marshal(text)
	if err != nil {
		return err
	}
	// Share the first existing record's timestamp so the record doesn't stretch
	// the session's measured elapsed time.
	ts := "2026-08-14T08:59:59Z"
	if m := firstTimestampRe.FindSubmatch(existing); m != nil {
		ts = string(m[1])
	}
	line := `{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":` + string(content) + "}}\n"
	return os.WriteFile(path, append([]byte(line), existing...), 0644)
}

// ticketIDFromImplementPrompt extracts a ticket identifier ("01", "02", ...)
// from a "/<skill> <ticket-path>" (Claude) or "$<skill> <ticket-path>"
// (Codex, see skillPrompt) prompt's ticket path, or ok=false if text isn't a
// skill-launch prompt — the fake agent's cue for which iteration worktree to
// do (simulated) work in. The skill name itself isn't checked: production
// launches type: code-review tickets under "gx-code-review" rather than the
// run's configured Skill (see launchSkill), and the fake only cares which
// ticket the prompt is for.
func ticketIDFromImplementPrompt(text string) (id string, ok bool) {
	if !strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "$") {
		return "", false
	}
	_, addr, found := strings.Cut(text, " ")
	if !found {
		return "", false
	}
	_, id, found = strings.Cut(addr, "/")
	if !found || len(id) < 2 {
		return "", false
	}
	return id[:2], true
}

// commitIterationWork simulates a finished agent turn: it writes and commits
// one file (named after id, so A/B/C never touch the same path and never
// conflict on cherry-pick) directly in dir via real git commands — dir is an
// iteration worktree AddWorktree already created for real.
func commitIterationWork(dir, id string) error {
	if err := os.WriteFile(filepath.Join(dir, id+".txt"), []byte("work for "+id+"\n"), 0644); err != nil {
		return err
	}
	sharedContent := map[string]string{
		"01": "shared base\n",
		"04": resolvedSharedContent,
		"05": "shared from E\n",
	}[id]
	if sharedContent != "" {
		if err := os.WriteFile(filepath.Join(dir, "shared.txt"), []byte(sharedContent), 0644); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "iteration " + id}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w\n%s", args, err, out)
		}
	}
	return nil
}

// testWorktreeDir resolves repoDir's linked-worktree directory the way
// production code does (d.WorktreeDir -> LinkedWorktreeDir), so tests that
// predict iteration/feature worktree paths agree with where Run actually
// creates them (repoDir itself for .bare clones, repoDir/.worktrees for
// standard clones).
func testWorktreeDir(t *testing.T, repoDir string) string {
	t.Helper()
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo(%s): %v", repoDir, err)
	}
	return repo.LinkedWorktreeDir()
}

// realGitFlagValue returns the value following the first occurrence of name in
// args, or "" if absent.
func realGitFlagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// agentJSON builds a herdr `agent` envelope's success output, matching the
// shape runAgentJSON parses (see herdr/agent.go).
func agentJSON(pane, status, session string) ([]byte, int) {
	return herdrfake.Result(map[string]any{
		"agent": map[string]any{
			"pane_id":       pane,
			"agent_status":  status,
			"agent_session": map[string]any{"value": session},
		},
	})
}

type compactRecoverySink struct {
	noopEventSink
	mu     sync.Mutex
	phases []string
}

func (s *compactRecoverySink) record(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phases = append(s.phases, phase)
}

func (s *compactRecoverySink) SmartZoneCompactStarted(string) { s.record("compact-started") }
func (s *compactRecoverySink) SmartZoneFinishingUp(string)    { s.record("finishing-up") }
func (s *compactRecoverySink) SmartZoneRecovered(string)      { s.record("recovered") }
func (s *compactRecoverySink) VerifyCapStopped(string)        { s.record("verify-cap-stopped") }

// contextPromptResponse implements pane01's "agent prompt" behavior for
// TestRun_ProductionRealGit_CodexContextAndQuotaConcurrentlyResolve, mirroring
// herdrfake.CodexRollout.prompt's phase transitions.
func contextPromptResponse(pane string, phase *string, text, sessionID string) ([]byte, int) {
	status := "working"
	switch {
	case text == "/compact":
		*phase = "compact-confirmation"
		status = "blocked"
	case strings.Contains(strings.ToLower(text), "finish up"):
		*phase = "continuation"
	default:
		*phase = "running"
	}
	return agentJSON(pane, status, sessionID)
}

// contextWaitResponse implements pane01's "agent wait" behavior, mirroring
// herdrfake.CodexRollout.wait's phase transitions, but writing usage lines
// straight to a synthetic rollout JSONL (see appendCodexRolloutLine) instead
// of going through herdrfake.State/RegisterCodexRollout.
func contextWaitResponse(pane string, phase *string, path string, line *int, sessionID string, compactedContext, compactedTotal, finalContext, finalTotal int) ([]byte, int) {
	switch *phase {
	case "running":
		return herdrfake.CommandError("timed out waiting for agent status")
	case "compact-confirmation":
		*phase = "compact-continuation"
		return agentJSON(pane, "working", sessionID)
	case "compact-continuation":
		if err := fixture.WriteCodexUsageLine(path, *line, compactedContext, compactedTotal); err != nil {
			return herdrfake.CommandError(err.Error())
		}
		*line++
		*phase = "idle"
		return agentJSON(pane, "idle", sessionID)
	case "continuation":
		if err := fixture.WriteCodexUsageLine(path, *line, finalContext, finalTotal); err != nil {
			return herdrfake.CommandError(err.Error())
		}
		*line++
		*phase = "final-pending"
		return herdrfake.CommandError("timed out waiting for agent status")
	case "final-pending":
		*phase = "final"
		return agentJSON(pane, "idle", sessionID)
	case "idle", "final":
		return agentJSON(pane, "idle", sessionID)
	default:
		return herdrfake.CommandError("unexpected context phase " + *phase)
	}
}

// pathExcluding returns the current process PATH with any directory holding
// an executable named one of names removed — so a test can simulate a tool
// being completely absent even when it happens to be installed on the host
// running `go test` (e.g. a developer machine with a real `codex`/`herdr` on
// PATH), without also losing `git` and everything else production code and
// the test harness itself need to run.
func pathExcluding(names ...string) string {
	var kept []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		excluded := false
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				excluded = true
				break
			}
		}
		if !excluded {
			kept = append(kept, dir)
		}
	}
	return strings.Join(kept, string(os.PathListSeparator))
}

// writeFakeExecutable creates an executable shell script named name in a
// fresh temp dir under t, with body as its script body (after the shebang),
// and returns that dir so the caller can prepend it to PATH.
func writeFakeExecutable(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return dir
}

// assertNoLaunchTrace asserts the outcomes ticket 32 requires of every
// launch-environment failure that's caught before a ticket is claimed:
// repository (no feature worktree ever created), tracker (ticket left open,
// never claimed, so no done/paused (answer)/paused (repair) residue), and
// diagnostic-trace (no run-log.jsonl at all — logEvent is never reached this
// early) outcomes. A failure this early also never opens a Herdr tab, since
// FindOrCreateWorkspace/AddWorktree/TabCreate all run strictly after these
// checks in Run (see loop.go) — verified structurally by the same feature-
// worktree assertion rather than separately, since no tab can exist without
// the workspace/worktree that would have to precede it.
func assertNoLaunchTrace(t *testing.T, repoDir, epicName, scratchDir, ticketFilename string, sink *recordingEventSink) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(testWorktreeDir(t, repoDir), epicName)); !os.IsNotExist(err) {
		t.Errorf("feature worktree exists after launch failure, want none created: %v", err)
	}

	ticketPath := filepath.Join(scratchDir, epicName, "issues", ticketFilename)
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile ticket: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("ticket after launch failure = %s, want status to remain open (never claimed)", raw)
	}
	for _, unwanted := range []string{"status: done", "status: paused"} {
		if strings.Contains(string(raw), unwanted) {
			t.Errorf("ticket after launch failure = %s, must not contain %q", raw, unwanted)
		}
	}

	if _, ok, readErr := ReadEvents(scratchDir, epicName); readErr != nil || ok {
		t.Errorf("ReadEvents = ok:%v err:%v, want no run-log written for a failure caught before claim", ok, readErr)
	}

	if hasEvent(sink, LiveEventIterationFinished, func(LiveEvent) bool { return true }) ||
		hasEvent(sink, LiveEventTicketNeedsHuman, func(ev LiveEvent) bool { return ev.Status == string(schema.StatusLegacyAnswer) }) {
		t.Errorf("launch failure events = %+v, must not report successful/generic completion", sink.Events())
	}
}
