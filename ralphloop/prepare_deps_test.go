package ralphloop

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// installRecorder is a fake InstallDeps that records the directories it was
// asked about and fails when err is set.
type installRecorder struct {
	paths []string
	err   error
	// setup and hook are what the last call got; installTime and setupTime are
	// what it reports back.
	setup       []string
	hook        func()
	installTime time.Duration
	setupTime   time.Duration
}

func (r *installRecorder) install(_ context.Context, path, _ string, setup []string, hook func()) ([]string, time.Duration, time.Duration, error) {
	r.paths = append(r.paths, path)
	r.setup, r.hook = setup, hook
	commands := []string{"fake install"}
	for _, s := range setup {
		commands = append(commands, "setup: "+s)
	}
	return commands, r.installTime, r.setupTime, r.err
}

func depsInstalledEvent(t *testing.T, o OneIteration) runlog.Event {
	t.Helper()
	evs, _, err := runlog.ReadEvents(o.ScratchDir, o.Epic)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == string(events.DepsInstalled) {
			return e
		}
	}
	t.Fatalf("no deps-installed event in %v", evs)
	return runlog.Event{}
}

func TestInstallDepsIn_PassesTheGivenListToTheInstall(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)
	hooked := false

	if err := installDepsIn(d, o, t.TempDir(), []string{"make gen"}, func() { hooked = true }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rec.setup, []string{"make gen"}) {
		t.Errorf("install got list %v, want [make gen]", rec.setup)
	}
	rec.hook()
	if !hooked {
		t.Error("install got a different setup-start hook")
	}
}

func TestInstallDepsIn_LogsDepsInstalledWithSetupCommandsAndDurations(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)
	rec.installTime, rec.setupTime = 2*time.Second, 3*time.Second

	if err := installDepsIn(d, o, t.TempDir(), []string{"make gen"}, nil); err != nil {
		t.Fatal(err)
	}
	e := depsInstalledEvent(t, o)
	if want := "fake install; setup: make gen"; e.Reason != want {
		t.Errorf("Reason = %q, want %q", e.Reason, want)
	}
	if e.DepsStep != 2*time.Second || e.SetupStep != 3*time.Second {
		t.Errorf("durations = %v, %v; want 2s, 3s", e.DepsStep, e.SetupStep)
	}
}

func TestInstallDepsIn_NoList_LogsOnlyTheInstallsAndNoSetupTime(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)
	rec.installTime = 2 * time.Second

	if err := installDepsIn(d, o, t.TempDir(), nil, nil); err != nil {
		t.Fatal(err)
	}
	e := depsInstalledEvent(t, o)
	if e.Reason != "fake install" || e.SetupStep != 0 || e.DepsStep != 2*time.Second {
		t.Errorf("event = %+v, want only the install and no setup time", e)
	}
}

func TestSetupFor_LoadsThroughTheDepsFunc(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	d.LoadSetup = func(worktree, projectDir string) ([]string, error) {
		return []string{worktree, projectDir}, nil
	}

	got, err := setupFor(d, o, "/wt")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/wt", o.ScratchDir}; !slices.Equal(got, want) {
		t.Errorf("setupFor = %v, want %v", got, want)
	}
}

func TestSetupFor_NilLoaderMeansNoList(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	d.LoadSetup = nil

	got, err := setupFor(d, o, "/wt")
	if err != nil || len(got) != 0 {
		t.Errorf("setupFor = %v, %v; want empty, nil", got, err)
	}
}

func TestSetupFor_LoadFailureIsReturned(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	boom := errors.New("bad checks.json")
	d.LoadSetup = func(string, string) ([]string, error) { return nil, boom }

	if _, err := setupFor(d, o, "/wt"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func prepareDepsFixture(t *testing.T) (deps.Deps, *installRecorder, OneIteration) {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	rec := &installRecorder{}
	d := deps.DefaultDeps()
	d.InstallDeps = rec.install
	wtDir := filepath.Join(t.TempDir(), "wt")
	d.WorktreeDir = func(string) (string, error) { return wtDir, nil }
	scratchDir := fixture.WriteEpic(t, "fix-it", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	o := OneIteration{
		RepoDir: repo, Epic: "fix-it", RootBase: "main", ScratchDir: scratchDir,
		Ticket: tickets.Ticket{Identifier: "01"},
	}
	return d, rec, o
}

func TestPrepareIteration_InstallsOnlyInTheIterationWorktree(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	w, err := PrepareIteration(d, o)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{w.Path}; !slices.Equal(rec.paths, want) {
		t.Errorf("installed in %v, want %v (not the feature worktree %s)", rec.paths, want, w.featureWorktree)
	}
}

func TestPrepareFeature_InstallsInTheFeatureWorktreeOnEveryPrepare(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	w, err := PrepareFeature(d, o)
	if err != nil {
		t.Fatal(err)
	}
	// The worktree already exists now; a real second add would be rejected.
	d.AddWorktree = func(string, string, string, string) error { return nil }
	if _, err := PrepareFeature(d, o); err != nil {
		t.Fatal(err)
	}
	if want := []string{w.Path, w.Path}; !slices.Equal(rec.paths, want) {
		t.Errorf("installed in %v, want %v", rec.paths, want)
	}
}

func TestPrepareCommitless_InstallsInADetachedWorktreeButNotAScratchDir(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	if _, err := PrepareCommitless(d, o, filepath.Join(t.TempDir(), "scratch"), ""); err != nil {
		t.Fatal(err)
	}
	if len(rec.paths) != 0 {
		t.Fatalf("scratch dir prepare installed in %v", rec.paths)
	}
	w, err := PrepareCommitless(d, o, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{w.Path}; !slices.Equal(rec.paths, want) {
		t.Errorf("installed in %v, want %v", rec.paths, want)
	}
}

func TestPrepareIteration_WorktreeAddCompletesWhileAnotherInstallIsBlocked(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	entered, unblock := make(chan struct{}), make(chan struct{})
	d.InstallDeps = func(ctx context.Context, _, _ string, _ []string, _ func()) ([]string, time.Duration, time.Duration, error) {
		close(entered)
		select {
		case <-unblock:
		case <-ctx.Done():
		}
		return nil, 0, 0, nil
	}
	done := make(chan error, 1)
	go func() { _, err := PrepareIteration(d, o); done <- err }()
	<-entered

	second := o
	second.Ticket.Identifier = "02"
	added := make(chan error, 1)
	go func() { _, err := addIterationWorktrees(d, second); added <- err }()
	select {
	case err := <-added:
		if err != nil {
			t.Fatalf("addIterationWorktrees() = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worktree add did not complete while another launch's install was blocked")
	}
	close(unblock)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestResume_NeverInstalls(t *testing.T) {
	d, rec, o := prepareDepsFixture(t)

	if _, err := ResumeIteration(d, o, "base"); err != nil {
		t.Fatal(err)
	}
	if _, err := ResumeFeature(d, o); err != nil {
		t.Fatal(err)
	}
	for _, scratch := range []string{"", t.TempDir()} {
		if _, err := ResumeCommitless(d, o, scratch); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.paths) != 0 {
		t.Errorf("a resume installed in %v", rec.paths)
	}
}

func TestPrepare_ReturnsTheInstallErrorUnwrapped(t *testing.T) {
	prepares := map[string]func(deps.Deps, OneIteration) error{
		"iteration": func(d deps.Deps, o OneIteration) error { _, err := PrepareIteration(d, o); return err },
		"feature":   func(d deps.Deps, o OneIteration) error { _, err := PrepareFeature(d, o); return err },
		"commitless": func(d deps.Deps, o OneIteration) error {
			_, err := PrepareCommitless(d, o, "", "main")
			return err
		},
	}
	for name, prepare := range prepares {
		t.Run(name, func(t *testing.T) {
			d, rec, o := prepareDepsFixture(t)
			rec.err = errors.New("installing dependencies in x: boom")

			if err := prepare(d, o); err != rec.err {
				t.Errorf("err = %v, want the install error itself", err)
			}
		})
	}
}

func TestPrepareIteration_LogsDepsInstalled(t *testing.T) {
	d, _, o := prepareDepsFixture(t)

	if _, err := PrepareIteration(d, o); err != nil {
		t.Fatal(err)
	}
	evs, _, err := runlog.ReadEvents(o.ScratchDir, o.Epic)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "deps-installed" || evs[0].Ticket != "01" {
		t.Errorf("events = %+v, want one deps-installed for ticket 01", evs)
	}
}

func TestPrepare_RebasesAnExistingEpicBranchOntoTheBaseOnFirstAttach(t *testing.T) {
	for name, prepare := range map[string]func(deps.Deps, OneIteration) error{
		"iteration": func(d deps.Deps, o OneIteration) error { _, err := PrepareIteration(d, o); return err },
		"feature":   func(d deps.Deps, o OneIteration) error { _, err := PrepareFeature(d, o); return err },
	} {
		t.Run(name, func(t *testing.T) {
			d, _, o := prepareDepsFixture(t)
			gitIn(t, o.RepoDir, "branch", o.Epic)
			gitIn(t, o.RepoDir, "commit", "--allow-empty", "-m", "newer trunk commit")

			if err := prepare(d, o); err != nil {
				t.Fatal(err)
			}
			if ok, err := d.IsAncestor(o.RepoDir, "main", o.Epic); err != nil || !ok {
				t.Errorf("main is not an ancestor of %s after the first attach (err %v)", o.Epic, err)
			}
		})
	}
}

func TestPrepareFeature_LaunchesOnTheOldTipWhenTheFirstRunRebaseConflicts(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	gitIn(t, o.RepoDir, "branch", o.Epic)
	gitIn(t, o.RepoDir, "commit", "--allow-empty", "-m", "newer trunk commit")
	d.RebaseOntoOrAbort = func(string, string) error { return errors.New("conflict") }

	if _, err := PrepareFeature(d, o); err != nil {
		t.Fatalf("PrepareFeature err = %v, want the launch to go on", err)
	}
	evs, _, err := runlog.ReadEvents(o.ScratchDir, o.Epic)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(evs, func(ev runlog.Event) bool { return ev.Type == string(events.EpicRebaseSkipped) })
	if i < 0 || evs[i].Ticket != "01" || !strings.Contains(evs[i].Reason, "conflict") {
		t.Errorf("events = %+v, want epic-rebase-skipped for 01 naming the conflict", evs)
	}
}

func TestPrepare_LeavesTheEpicBranchAloneOnceTheEpicHasRun(t *testing.T) {
	for name, prepare := range map[string]func(deps.Deps, OneIteration) error{
		"iteration": func(d deps.Deps, o OneIteration) error { _, err := PrepareIteration(d, o); return err },
		"feature":   func(d deps.Deps, o OneIteration) error { _, err := PrepareFeature(d, o); return err },
	} {
		t.Run(name, func(t *testing.T) {
			d, _, o := prepareDepsFixture(t)
			gitIn(t, o.RepoDir, "branch", o.Epic)
			if err := runlog.AppendEvent(o.ScratchDir, o.Epic, runlog.Event{Type: string(events.IterationStarted), Ticket: "01"}); err != nil {
				t.Fatal(err)
			}
			gitIn(t, o.RepoDir, "commit", "--allow-empty", "-m", "newer trunk commit")
			d.RebaseOntoOrAbort = func(string, string) error {
				t.Error("rebased an epic branch that has already run")
				return nil
			}

			if err := prepare(d, o); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPrepareFeature_RebasesWhenTheWorktreeAlreadyExists(t *testing.T) {
	d, _, o := prepareDepsFixture(t)
	gitIn(t, o.RepoDir, "branch", o.Epic)
	if _, err := PrepareFeature(d, o); err != nil {
		t.Fatal(err)
	}
	gitIn(t, o.RepoDir, "commit", "--allow-empty", "-m", "newer trunk commit")

	if _, err := PrepareFeature(d, o); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.IsAncestor(o.RepoDir, "main", o.Epic); err != nil || !ok {
		t.Errorf("main is not an ancestor of %s after a second attach (err %v)", o.Epic, err)
	}
}
