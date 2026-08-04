package ralphloop

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets"
)

// conflictResolver is a fake stand-in for the real "/resolving-merge-
// conflicts" agent production Ralph-loop launches on a cherry-pick conflict
// (see resolveCherryPickConflict in iteration.go): it recognizes the same
// skill prompt, then — instead of an LLM turn — does the deterministic
// real-git equivalent of what that skill instructs an agent to do: verify
// it's running in the feature worktree with a cherry-pick actually stopped on
// a conflict, resolve every conflicted file with fixed content, stage it, and
// run `git cherry-pick --continue`. Any of those preconditions failing (wrong
// cwd, no cherry-pick in progress) or an unrecognized prompt fails the
// command immediately rather than guessing.
type conflictResolver struct {
	t            *testing.T
	expectedCwd  string
	sessionID    string
	resolvedText string

	mu      sync.Mutex
	paneCwd map[string]string
	status  string
}

func newConflictResolver(t *testing.T, expectedCwd, sessionID, resolvedText string) *conflictResolver {
	return &conflictResolver{
		t:            t,
		expectedCwd:  expectedCwd,
		sessionID:    sessionID,
		resolvedText: resolvedText,
		paneCwd:      map[string]string{},
	}
}

func (r *conflictResolver) registerAll(s *herdrfake.State) {
	s.Register("tab", "create", r.handleTabCreate)
	s.Register("tab", "close", r.handleTabClose)
	s.Register("agent", "start", r.handleAgentStart)
	s.Register("agent", "wait", r.handleAgentWait)
	s.Register("agent", "prompt", r.handlePrompt)
	s.Register("agent", "read", r.handleAgentRead)
}

func (r *conflictResolver) handleTabClose(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
	return nil, herdrfake.Identities{}, nil
}

func (r *conflictResolver) handleTabCreate(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
	rest := argv[2:]
	cwd := flagValue(rest, "--cwd")
	label := flagValue(rest, "--label")
	workspace := flagValue(rest, "--workspace")
	tabID := "tab-" + label
	paneID := "pane-" + label

	r.mu.Lock()
	r.paneCwd[paneID] = cwd
	r.mu.Unlock()

	return map[string]any{
		"tab":       map[string]any{"tab_id": tabID, "workspace_id": workspace, "label": label},
		"root_pane": map[string]any{"pane_id": paneID},
	}, herdrfake.Identities{WorkspaceID: workspace, TabID: tabID, PaneID: paneID}, nil
}

func (r *conflictResolver) handleAgentStart(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
	pane := flagValue(argv[2:], "--pane")
	r.mu.Lock()
	r.status = "idle"
	r.mu.Unlock()
	return conflictAgentResult(pane, "idle", "")
}

func (r *conflictResolver) handleAgentWait(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
	target := argv[2]
	until := parseUntil(argv[3:])
	r.mu.Lock()
	cur := r.status
	r.mu.Unlock()
	if len(until) == 0 || slices.Contains(until, cur) {
		return conflictAgentResult(target, cur, "")
	}
	return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
}

func (r *conflictResolver) handleAgentRead(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
	return "", herdrfake.Identities{}, nil
}

// handlePrompt is the resolver's core: on the production
// "/resolving-merge-conflicts" prompt, it validates its own preconditions
// then performs the real conflict resolution against argv[2]'s pane cwd.
// Anything else fails immediately rather than producing a default response.
func (r *conflictResolver) handlePrompt(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
	target, text := argv[2], argv[3]
	if text != "/resolving-merge-conflicts" {
		return nil, herdrfake.Identities{}, fmt.Errorf("conflict resolver received unexpected prompt %q", text)
	}

	r.mu.Lock()
	cwd := r.paneCwd[target]
	r.mu.Unlock()

	if cwd == "" || cwd != r.expectedCwd {
		return nil, herdrfake.Identities{}, fmt.Errorf("conflict resolver launched in wrong cwd %q, want %q", cwd, r.expectedCwd)
	}

	inProgress, err := git.CherryPickInProgress(cwd)
	if err != nil {
		return nil, herdrfake.Identities{}, fmt.Errorf("checking cherry-pick state in %s: %w", cwd, err)
	}
	if !inProgress {
		return nil, herdrfake.Identities{}, fmt.Errorf("conflict resolver launched with no cherry-pick in progress in %s", cwd)
	}

	conflicted, err := conflictedFiles(cwd)
	if err != nil {
		return nil, herdrfake.Identities{}, fmt.Errorf("listing conflicted files in %s: %w", cwd, err)
	}
	if len(conflicted) == 0 {
		return nil, herdrfake.Identities{}, fmt.Errorf("cherry-pick in progress but no conflicted files found in %s", cwd)
	}

	for _, f := range conflicted {
		if err := os.WriteFile(filepath.Join(cwd, f), []byte(r.resolvedText), 0644); err != nil {
			return nil, herdrfake.Identities{}, fmt.Errorf("writing resolved %s: %w", f, err)
		}
		if err := gitRun(cwd, "add", f); err != nil {
			return nil, herdrfake.Identities{}, fmt.Errorf("staging resolved %s: %w", f, err)
		}
	}
	if err := gitContinueCherryPick(cwd); err != nil {
		return nil, herdrfake.Identities{}, fmt.Errorf("git cherry-pick --continue in %s: %w", cwd, err)
	}

	r.mu.Lock()
	r.status = "done"
	r.mu.Unlock()

	return conflictAgentResult(target, "working", r.sessionID)
}

// conflictAgentResult builds the same {"agent": {...}} envelope agentResult
// does, plus an optional agent_session so the resolver's own session id
// (distinct from the iteration agent's) flows through to the caller.
func conflictAgentResult(pane, status, sessionID string) (any, herdrfake.Identities, error) {
	agent := map[string]any{"pane_id": pane, "agent_status": status}
	ids := herdrfake.Identities{PaneID: pane}
	if sessionID != "" {
		agent["agent_session"] = map[string]any{"value": sessionID}
		ids.SessionID = sessionID
	}
	return map[string]any{"agent": agent}, ids, nil
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func conflictedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %w\n%s", args, err, out)
	}
	return nil
}

func gitContinueCherryPick(dir string) error {
	cmd := exec.Command("git", "cherry-pick", "--continue")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}
	return nil
}

func gitSubject(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--format=%s", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log --format=%%s %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// TestCherryPickWithConflictResolution_ProductionRealConflict drives
// cherryPickWithConflictResolution — the real production function, not a
// stand-in — against a real git repo with a genuine cherry-pick conflict,
// resolved through the same herdr tab-create/agent-start/agent-wait/
// agent-prompt protocol production Ralph-loop uses, reaching a fake
// "/resolving-merge-conflicts" agent through a process-boundary fake herdr
// executable (testutil/herdrfake) instead of a mocked Deps field.
func TestCherryPickWithConflictResolution_ProductionRealConflict(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/main/iter-03", base)
	testutil.WriteFile(t, dir, "shared.txt", "iteration content\n")
	testutil.CommitAll(t, dir, "Add shared.txt from iteration")
	iterTip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	wantSubject := gitSubject(t, dir, "HEAD")

	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "shared.txt", "main content\n")
	testutil.CommitAll(t, dir, "Add shared.txt from main")

	const resolutionSessionID = "resolver-session-1"
	const iterationSessionID = "iteration-session-1"
	const resolvedText = "resolved by fake conflict resolver\n"

	resolver := newConflictResolver(t, dir, resolutionSessionID, resolvedText)
	s := herdrfake.NewState(t)
	resolver.registerAll(s)
	herdrfake.StartState(t, s)

	scratchDir := t.TempDir()
	d := DefaultDeps()
	d.Sleep = func(time.Duration) {}
	d.Now = func() time.Time { return time.Unix(0, 0) }

	var out bytes.Buffer
	p := iterationParams{
		WorkspaceID:     "ws-1",
		FeatureWorktree: dir,
		FeatureBranch:   "main",
		Agent:           AgentClaude,
		Ticket:          tickets.Ticket{Identifier: "03"},
		ScratchDir:      scratchDir,
		SmartZone:       1_000_000,
		Gate:            NewGate(),
		Sink:            NewTextEventSink(&out),
	}

	picked, gotResolutionSessionID, err := cherryPickWithConflictResolution(d, p, base, iterTip, iterationSessionID, "iter-pane", "iter-tab")
	if err != nil {
		t.Fatalf("cherryPickWithConflictResolution: %v", err)
	}
	if !picked {
		t.Error("picked = false, want true")
	}
	if gotResolutionSessionID != resolutionSessionID {
		t.Errorf("resolution session = %q, want %q", gotResolutionSessionID, resolutionSessionID)
	}

	inProgress, err := git.CherryPickInProgress(dir)
	if err != nil {
		t.Fatalf("CherryPickInProgress: %v", err)
	}
	if inProgress {
		t.Error("cherry-pick still in progress after resolution")
	}

	gotContent, err := os.ReadFile(filepath.Join(dir, "shared.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(gotContent) != resolvedText {
		t.Errorf("shared.txt = %q, want deterministic resolved content %q", gotContent, resolvedText)
	}

	if gotSubject := gitSubject(t, dir, "HEAD"); gotSubject != wantSubject {
		t.Errorf("landed commit subject = %q, want original cherry-picked subject %q preserved", gotSubject, wantSubject)
	}

	events, ok, err := readEvents(scratchDir, "main")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var conflictHit *Event
	for i, e := range events {
		if e.Type == eventConflictHit {
			conflictHit = &events[i]
		}
	}
	if conflictHit == nil {
		t.Fatalf("events = %+v, want %q", events, eventConflictHit)
	}
	if conflictHit.AgentSession != iterationSessionID {
		t.Errorf("conflict-hit session = %q, want the iteration agent's session %q", conflictHit.AgentSession, iterationSessionID)
	}

	trace := s.Trace()
	var verbs []string
	for _, e := range trace {
		if len(e.Argv) >= 2 {
			verbs = append(verbs, e.Argv[0]+" "+e.Argv[1])
		}
	}
	wantPrefix := []string{"tab create", "agent start", "agent wait", "agent prompt"}
	if len(verbs) < len(wantPrefix) {
		t.Fatalf("traced commands = %v, want at least %v", verbs, wantPrefix)
	}
	for i, want := range wantPrefix {
		if verbs[i] != want {
			t.Errorf("traced command #%d = %q, want %q (full order = %v)", i, verbs[i], want, verbs)
		}
	}

	// Nothing beyond tab-create/agent-start/agent-wait/agent-prompt/agent-read
	// is registered, so any other command against this same coordinator fails
	// immediately instead of silently no-op'ing.
	if err := herdr.AgentSendKeys("pane-"+conflictLabel(p.Ticket.Identifier), "ctrl+c"); err == nil {
		t.Error("AgentSendKeys against an unregistered command = nil error, want failure")
	}
}

func TestConflictResolverHandlePrompt_WrongCwd_FailsImmediately(t *testing.T) {
	expected := t.TempDir()
	wrong := t.TempDir()
	resolver := newConflictResolver(t, expected, "sess", "resolved\n")
	resolver.paneCwd["pane-x"] = wrong

	_, _, err := resolver.handlePrompt(nil, []string{"agent", "prompt", "pane-x", "/resolving-merge-conflicts"})
	if err == nil {
		t.Fatal("handlePrompt() error = nil, want a wrong-cwd failure")
	}
	if !strings.Contains(err.Error(), "wrong cwd") {
		t.Errorf("error = %v, want it to mention the cwd mismatch", err)
	}
}

func TestConflictResolverHandlePrompt_NoCherryPickInProgress_FailsImmediately(t *testing.T) {
	dir := testutil.TempRepo(t) // clean repo, nothing in progress
	resolver := newConflictResolver(t, dir, "sess", "resolved\n")
	resolver.paneCwd["pane-x"] = dir

	_, _, err := resolver.handlePrompt(nil, []string{"agent", "prompt", "pane-x", "/resolving-merge-conflicts"})
	if err == nil {
		t.Fatal("handlePrompt() error = nil, want a missing-cherry-pick-state failure")
	}
	if !strings.Contains(err.Error(), "cherry-pick") {
		t.Errorf("error = %v, want it to mention missing cherry-pick state", err)
	}
}

func TestConflictResolverHandlePrompt_UnexpectedPrompt_FailsImmediately(t *testing.T) {
	dir := testutil.TempRepo(t)
	resolver := newConflictResolver(t, dir, "sess", "resolved\n")
	resolver.paneCwd["pane-x"] = dir

	_, _, err := resolver.handlePrompt(nil, []string{"agent", "prompt", "pane-x", "/some-other-skill"})
	if err == nil {
		t.Fatal("handlePrompt() error = nil, want failure for an unrecognized prompt")
	}
}
