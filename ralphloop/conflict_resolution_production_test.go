package ralphloop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// conflictResolver is a fake stand-in for the real "/resolving-merge-
// conflicts" agent production Ralph-loop launches on a cherry-pick conflict
// (see resolveCherryPickConflict in iteration.go): hooked into fakeDeps'
// Runner, it recognizes the same skill prompt, then — instead of an LLM turn
// — does the deterministic real-git equivalent of what that skill instructs
// an agent to do: verify it's running in the feature worktree with a
// cherry-pick actually stopped on a conflict, resolve every conflicted file
// with fixed content, stage it, and run `git cherry-pick --continue`. Any of
// those preconditions failing (wrong cwd, no cherry-pick in progress) or an
// unrecognized prompt fails the prompt immediately rather than guessing.
type conflictResolver struct {
	expectedCwd  string
	resolvedText string

	mu  sync.Mutex
	cwd map[string]string // by session label
}

func newConflictResolver(expectedCwd, resolvedText string) *conflictResolver {
	return &conflictResolver{
		expectedCwd:  expectedCwd,
		resolvedText: resolvedText,
		cwd:          map[string]string{},
	}
}

func (r *conflictResolver) onStart(opts agentrunner.StartOptions) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cwd[opts.Label] = opts.Cwd
	return nil
}

// onPrompt is the resolver's core: on the production
// "/gx-resolving-merge-conflicts" prompt, it validates its own preconditions
// then performs the real conflict resolution in s's cwd.
func (r *conflictResolver) onPrompt(s agentrunner.Session, text string) error {
	if text != "/gx-resolving-merge-conflicts" {
		return fmt.Errorf("conflict resolver received unexpected prompt %q", text)
	}

	r.mu.Lock()
	cwd := r.cwd[s.Label]
	r.mu.Unlock()

	if cwd == "" || cwd != r.expectedCwd {
		return fmt.Errorf("conflict resolver launched in wrong cwd %q, want %q", cwd, r.expectedCwd)
	}

	inProgress, err := git.CherryPickInProgress(cwd)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state in %s: %w", cwd, err)
	}
	if !inProgress {
		return fmt.Errorf("conflict resolver launched with no cherry-pick in progress in %s", cwd)
	}

	conflicted, err := conflictedFiles(cwd)
	if err != nil {
		return fmt.Errorf("listing conflicted files in %s: %w", cwd, err)
	}
	if len(conflicted) == 0 {
		return fmt.Errorf("cherry-pick in progress but no conflicted files found in %s", cwd)
	}

	for _, f := range conflicted {
		if err := os.WriteFile(filepath.Join(cwd, f), []byte(r.resolvedText), 0644); err != nil {
			return fmt.Errorf("writing resolved %s: %w", f, err)
		}
		if err := gitRun(cwd, "add", f); err != nil {
			return fmt.Errorf("staging resolved %s: %w", f, err)
		}
	}
	if err := gitContinueCherryPick(cwd); err != nil {
		return fmt.Errorf("git cherry-pick --continue in %s: %w", cwd, err)
	}
	return nil
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

// realLandingDeps is realGitDeps plus the rest of landing's git operations,
// for driving a genuine cherry-pick conflict.
func realLandingDeps() Deps {
	d := realGitDeps()
	real := DefaultDeps()
	d.CommitsAhead = real.CommitsAhead
	d.CommitSubjects = real.CommitSubjects
	d.CherryPickRange = real.CherryPickRange
	d.CherryPickInProgress = real.CherryPickInProgress
	d.AbortCherryPick = real.AbortCherryPick
	return d
}

// conflictRepo builds a real repo whose ralph-loop/main/iter-03 branch
// conflicts with main on shared.txt, returning the branch's base, its tip and
// its commit subject.
func conflictRepo(t *testing.T) (dir, base, iterTip, subject string) {
	t.Helper()
	dir = testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/main/iter-03", base)
	testutil.WriteFile(t, dir, "shared.txt", "iteration content\n")
	testutil.CommitAll(t, dir, "Add shared.txt from iteration")
	iterTip, err = git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	subject = gitSubject(t, dir, "HEAD")

	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "shared.txt", "main content\n")
	testutil.CommitAll(t, dir, "Add shared.txt from main")
	return dir, base, iterTip, subject
}

func conflictParams(t *testing.T, dir string) iterationParams {
	t.Helper()
	scratchDir := t.TempDir()
	testutil.EnsureEpicTicketMD(t, filepath.Join(scratchDir, "main"))
	return iterationParams{
		WorkspaceID:     "ws-1",
		FeatureWorktree: dir,
		FeatureBranch:   "main",
		Agent:           AgentClaude,
		Ticket:          tickets.Ticket{Identifier: "03"},
		ScratchDir:      scratchDir,
		SmartZone:       1_000_000,
		Gate:            NewGate(),
		Sink:            noopEventSink{},
	}
}

// TestCherryPickWithConflictResolution_ProductionRealConflict drives
// cherryPickWithConflictResolution — the real production function, not a
// stand-in — against a real git repo with a genuine cherry-pick conflict,
// resolved by a fake "/gx-resolving-merge-conflicts" agent hosted on the
// Runner.
func TestCherryPickWithConflictResolution_ProductionRealConflict(t *testing.T) {
	t.Parallel()
	dir, base, iterTip, wantSubject := conflictRepo(t)

	const resolutionSessionID = "resolver-session-1"
	const iterationSessionID = "iteration-session-1"
	const resolvedText = "resolved by fake conflict resolver\n"

	resolver := newConflictResolver(dir, resolvedText)
	d := realLandingDeps()
	d.Sleep = func(time.Duration) {}
	d.Now = func() time.Time { return time.Unix(0, 0) }
	onRunnerStart(d, resolver.onStart)
	onRunnerPrompt(d, resolver.onPrompt)
	runner := fakeRunner(d)
	runner.IDs = func(label string) (string, string) { return "pane-" + label, resolutionSessionID }

	p := conflictParams(t, dir)
	label := conflictLabel(p.Ticket.Identifier)

	res, gotResolutionSessionID, err := cherryPickWithConflictResolution(d, p, base, iterTip, iterationSessionID, "iter-pane")
	if err != nil {
		t.Fatalf("cherryPickWithConflictResolution: %v", err)
	}
	if res.Outcome != Landed {
		t.Errorf("outcome = %q, want %q", res.Outcome, Landed)
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

	evs, ok, err := ReadEvents(p.ScratchDir, "main")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var conflictHit *Event
	for i, e := range evs {
		if e.Type == string(events.ConflictHit) {
			conflictHit = &evs[i]
		}
	}
	if conflictHit == nil {
		t.Fatalf("events = %+v, want %q", evs, string(events.ConflictHit))
	}
	if conflictHit.AgentSession != iterationSessionID {
		t.Errorf("conflict-hit session = %q, want the iteration agent's session %q", conflictHit.AgentSession, iterationSessionID)
	}

	if got, want := runner.Prompts(label), []string{"/gx-resolving-merge-conflicts"}; !slices.Equal(got, want) {
		t.Errorf("resolver prompts = %v, want %v", got, want)
	}
	// The resolver's session must actually end once it finishes — the gap
	// tickets-queue-polish/04 found when the tab was never closed.
	if _, live, _ := runner.Find(label); live {
		t.Errorf("conflict-resolution session %s still live after resolution", label)
	}
}

func TestResolveCherryPickConflict_StopFailure_FailsTheResolution(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	p := conflictParams(t, t.TempDir())
	label := conflictLabel(p.Ticket.Identifier)
	stopErr := errors.New("stop failed")
	onRunnerPrompt(d, func(agentrunner.Session, string) error {
		fakeRunner(d).SetStopErr(label, stopErr)
		return nil
	})

	if _, err := resolveCherryPickConflict(d, p); !errors.Is(err, stopErr) {
		t.Fatalf("resolveCherryPickConflict() error = %v, want it to wrap %v", err, stopErr)
	}
}

func TestConflictResolverOnPrompt_WrongCwd_FailsImmediately(t *testing.T) {
	t.Parallel()
	resolver := newConflictResolver(t.TempDir(), "resolved\n")
	resolver.cwd["x"] = t.TempDir()

	err := resolver.onPrompt(agentrunner.Session{Label: "x"}, "/gx-resolving-merge-conflicts")
	if err == nil {
		t.Fatal("onPrompt() error = nil, want a wrong-cwd failure")
	}
	if !strings.Contains(err.Error(), "wrong cwd") {
		t.Errorf("error = %v, want it to mention the cwd mismatch", err)
	}
}

func TestConflictResolverOnPrompt_NoCherryPickInProgress_FailsImmediately(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t) // clean repo, nothing in progress
	resolver := newConflictResolver(dir, "resolved\n")
	resolver.cwd["x"] = dir

	err := resolver.onPrompt(agentrunner.Session{Label: "x"}, "/gx-resolving-merge-conflicts")
	if err == nil {
		t.Fatal("onPrompt() error = nil, want a missing-cherry-pick-state failure")
	}
	if !strings.Contains(err.Error(), "cherry-pick") {
		t.Errorf("error = %v, want it to mention missing cherry-pick state", err)
	}
}

func TestConflictResolverOnPrompt_UnexpectedPrompt_FailsImmediately(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	resolver := newConflictResolver(dir, "resolved\n")
	resolver.cwd["x"] = dir

	if err := resolver.onPrompt(agentrunner.Session{Label: "x"}, "/some-other-skill"); err == nil {
		t.Fatal("onPrompt() error = nil, want failure for an unrecognized prompt")
	}
}
