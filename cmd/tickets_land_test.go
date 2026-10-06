package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

type landFixture struct {
	in         landInput
	deps       ralphloop.Deps
	ticketPath string
	lockDir    string
	picked     *bool
	pickErr    *error
	pickActive *bool
}

func ticketWith(status, extra string) string {
	return "---\nid: \"01\"\nstatus: " + status + "\ntype: implement\n" + extra + "---\n# A\n"
}

func newLandFixture(t *testing.T, content string) *landFixture {
	t.Helper()
	epicPath, ticketPath := unparkFixture(t, content)
	picked, pickActive := false, false
	var pickErr error
	d := ralphloop.Deps{
		WorktreeDir: func(string) (string, error) { return t.TempDir(), nil },
		RevParse: func(_, ref string) (string, error) {
			return "sha-" + ref, nil
		},
		MergeBase:      func(_, _, _ string) (string, error) { return "base", nil },
		IsAncestor:     func(_, _, _ string) (bool, error) { return false, nil },
		PatchesApplied: func(_, _, _, _ string) (bool, error) { return false, nil },
		CherryPickRange: func(_, _, _ string) error {
			picked = true
			if pickErr != nil {
				pickActive = true
			}
			return pickErr
		},
		CherryPickInProgress: func(string) (bool, error) { return pickActive, nil },
		AppendTrailers:       func(string, ...git.Trailer) error { return nil },
		FindWorkspace:        func(string) (string, error) { return "ws", nil },
		TabList:              func(string) ([]herdr.Tab, error) { return nil, nil },
	}
	return &landFixture{
		in:         landInput{EpicPath: epicPath, ID: "01", Getwd: func() (string, error) { return t.TempDir(), nil }, Cwd: t.TempDir(), JSON: true},
		deps:       d,
		ticketPath: ticketPath,
		lockDir:    epicPath,
		picked:     &picked,
		pickErr:    &pickErr,
		pickActive: &pickActive,
	}
}

func (f *landFixture) run(t *testing.T) (stdout string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = runTicketsLand(f.in, f.deps, &out, &errOut)
	return out.String(), err
}

func (f *landFixture) refusal(t *testing.T) RefusalEnvelope {
	t.Helper()
	out, err := f.run(t)
	assertExit1(t, err)
	var env RefusalEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("stdout %q: %v", out, jerr)
	}
	if !env.Refused {
		t.Fatalf("envelope = %+v", env)
	}
	return env
}

func (f *landFixture) ticketText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(f.ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunTicketsLand_StatusRules(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"done", "claimed", "needs-repair"} {
		t.Run("accepts "+status, func(t *testing.T) {
			t.Parallel()
			f := newLandFixture(t, ticketWith(status, ""))
			out, err := f.run(t)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			var res map[string]any
			if err := json.Unmarshal([]byte(out), &res); err != nil {
				t.Fatal(err)
			}
			if res["outcome"] != "landed" || !strings.HasSuffix(res["trailer_value"].(string), ":widget-epic/01") {
				t.Errorf("result = %v", res)
			}
			if !strings.Contains(f.ticketText(t), "status: done") {
				t.Errorf("ticket not done:\n%s", f.ticketText(t))
			}
		})
	}
	for _, status := range []string{"draft", "open", "needs-answer"} {
		t.Run("refuses "+status, func(t *testing.T) {
			t.Parallel()
			f := newLandFixture(t, ticketWith(status, ""))
			if env := f.refusal(t); env.Reason != ReasonStatusRefused {
				t.Errorf("reason = %s", env.Reason)
			}
			if *f.picked {
				t.Error("cherry-pick ran on a refused ticket")
			}
		})
	}
}

func TestRunTicketsLand_RefusesExplicitCommitlessOnly(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", "commitless: true\n"))
	if env := f.refusal(t); env.Reason != ReasonCommitless {
		t.Errorf("reason = %s", env.Reason)
	}

	// A research ticket is commitless by type, not by flag: it still lands.
	g := newLandFixture(t, strings.Replace(ticketWith("claimed", ""), "type: implement", "type: research", 1))
	if _, err := g.run(t); err != nil {
		t.Errorf("type-derived commitless refused: %v", err)
	}
}

func TestRunTicketsLand_GuardRefusesFromRalphLoopBranch(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.in.Getwd = agentGetwd(t, "ralph-loop/widget-epic-item-01")
	if env := f.refusal(t); env.Reason != ReasonRalphLoopCwd {
		t.Errorf("reason = %s", env.Reason)
	}
}

func TestRunTicketsLand_LiveTabRefusalAndOverride(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.deps.TabList = func(string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab1", Label: "widget-epic-iter-01"}}, nil
	}
	if env := f.refusal(t); env.Reason != ReasonLiveAgentOnTab {
		t.Errorf("reason = %s", env.Reason)
	}

	f.in.IgnoreLiveTab = true
	if _, err := f.run(t); err != nil {
		t.Errorf("override: err = %v", err)
	}
}

func TestRunTicketsLand_MissingIterationBranch(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.deps.RevParse = func(_, ref string) (string, error) {
		if strings.HasPrefix(ref, "ralph-loop/") {
			return "", errors.New("unknown revision")
		}
		return "sha", nil
	}
	env := f.refusal(t)
	if env.Reason != ReasonIterationBranchMissing || !strings.Contains(env.Message, "--from") {
		t.Errorf("envelope = %+v", env)
	}

	f.in.From, f.in.To = "abc", "def"
	if _, err := f.run(t); err != nil {
		t.Errorf("explicit range: err = %v", err)
	}
}

func TestRunTicketsLand_AlreadyAppliedFixesStaleStatus(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.deps.PatchesApplied = func(_, _, _, _ string) (bool, error) { return true, nil }
	out, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	var res landResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Outcome != "already-applied" {
		t.Errorf("outcome = %s", res.Outcome)
	}
	if *f.picked {
		t.Error("already-applied ticket was cherry-picked")
	}
	if !strings.Contains(f.ticketText(t), "status: done") {
		t.Errorf("stale status not fixed:\n%s", f.ticketText(t))
	}
}

func TestRunTicketsLand_ConflictExitsZeroWithMarkerAndNoStatus(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	*f.pickErr = errors.New("conflict")
	out, err := f.run(t)
	if err != nil {
		t.Fatalf("conflict must exit 0, err = %v", err)
	}
	if !strings.Contains(out, `"conflicted"`) {
		t.Errorf("stdout = %s", out)
	}
	m, err := ralphloop.ReadLandMarker(f.lockDir)
	if err != nil || m == nil || m.Ticket != "01" || m.PrePickHead != "sha-HEAD" {
		t.Errorf("marker = %+v, err = %v", m, err)
	}
	if strings.Contains(f.ticketText(t), "status: done") {
		t.Error("status written on conflict")
	}
	if err := ralphloop.AcquireLandLock(f.lockDir); !errors.Is(err, ralphloop.ErrLandLocked) {
		t.Errorf("lock not held after conflict: %v", err)
	}
}

func TestRunTicketsLand_LockContention(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	if err := ralphloop.AcquireLandLock(f.lockDir); err != nil {
		t.Fatal(err)
	}
	if env := f.refusal(t); env.Reason != ReasonLandLocked {
		t.Errorf("reason = %s", env.Reason)
	}
}

func TestRunTicketsLand_ReleasesLockAfterLanding(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	if err := ralphloop.AcquireLandLock(f.lockDir); err != nil {
		t.Errorf("lock still held after a clean land: %v", err)
	}
}

func TestRunTicketsLand_RefusesPendingMarkerForOtherTicket(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	if err := ralphloop.WriteLandMarker(f.lockDir, ralphloop.LandMarker{Epic: "widget-epic", Ticket: "02"}); err != nil {
		t.Fatal(err)
	}
	if env := f.refusal(t); env.Reason != ReasonLandBlocked {
		t.Errorf("reason = %s", env.Reason)
	}
}

func TestRunTicketsLand_WritesManualLandEvent(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	events, ok, err := ralphloop.ReadEvents(filepath.Dir(f.lockDir), "widget-epic")
	if err != nil || !ok || len(events) != 1 {
		t.Fatalf("events = %+v ok=%v err=%v", events, ok, err)
	}
	ev := events[0]
	if ev.Type != ralphloop.EventManualLand || ev.Ticket != "01" || ev.Outcome != "landed" || ev.Reason == "" || ev.AgentSession != "" {
		t.Errorf("event = %+v", ev)
	}
}

// Not parallel: transcript lookup resolves $HOME process-wide.
func TestRunTicketsLand_StampsMetricsFromRecoveredSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	f := newLandFixture(t, ticketWith("claimed", ""))
	const cwd, session = "/repo/iter-01", "sess-1"
	path := transcript.PathIn(home, cwd, session)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"type":"assistant","timestamp":"2026-08-01T10:00:00Z","message":{"model":"claude-sonnet-5","usage":{"input_tokens":5,"cache_read_input_tokens":100,"output_tokens":50}}}
{"type":"assistant","timestamp":"2026-08-01T10:01:00Z","message":{"model":"claude-sonnet-5","usage":{"input_tokens":5,"cache_read_input_tokens":200,"output_tokens":50}}}
`
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Dir(f.lockDir)
	started := ralphloop.Event{Type: "iteration-started", Ticket: "01", AgentSession: session, Cwd: cwd}
	if err := ralphloop.AppendEvent(scratch, "widget-epic", started); err != nil {
		t.Fatal(err)
	}

	out, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"metrics_stamped":true`) {
		t.Errorf("stdout = %s", out)
	}
	got := f.ticketText(t)
	if !strings.Contains(got, "elapsed_time: 60") || strings.Contains(got, "actual_cost: 0\n") {
		t.Errorf("ticket = %s", got)
	}
	events, _, _ := ralphloop.ReadEvents(scratch, "widget-epic")
	ev := events[len(events)-1]
	if ev.Type != ralphloop.EventManualLand || ev.AgentSession != session || ev.Reason != "" {
		t.Errorf("event = %+v", ev)
	}
}

func TestTicketsLandHelp_CarriesBothCaveats(t *testing.T) {
	t.Parallel()
	long := strings.Join(strings.Fields(newTicketsLandCmd(deps{}).Long), " ")
	for _, want := range []string{"guard rail, not a boundary", "least trustworthy of the three metrics"} {
		if !strings.Contains(long, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}

func TestRunTicketsLand_HumanModeConflictText(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.in.JSON = false
	*f.pickErr = errors.New("conflict")
	var out, errOut bytes.Buffer
	if err := runTicketsLand(f.in, f.deps, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--continue") {
		t.Errorf("stdout = %q", out.String())
	}
}

// pendingConflict leaves the fixture as a conflicted land would: marker and
// lock written, cherry-pick active, HEAD still at the pre-pick SHA.
func pendingConflict(t *testing.T, f *landFixture) {
	t.Helper()
	if err := ralphloop.WriteLandMarker(f.lockDir, ralphloop.LandMarker{Epic: "widget-epic", Ticket: "01", PrePickHead: "pre"}); err != nil {
		t.Fatal(err)
	}
	if err := ralphloop.AcquireLandLock(f.lockDir); err != nil {
		t.Fatal(err)
	}
	*f.pickActive = true
	f.deps.RevParse = func(_, _ string) (string, error) { return "pre", nil }
}

func TestRunTicketsLand_ContinueRefusedWhileInProgress(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	pendingConflict(t, f)
	f.in.Continue = true
	if env := f.refusal(t); env.Reason != ReasonLandConflictPending {
		t.Errorf("reason = %s", env.Reason)
	}
}

func TestRunTicketsLand_ContinueRefusedWhenHeadDidNotMove(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	pendingConflict(t, f)
	*f.pickActive = false
	f.in.Continue = true
	if env := f.refusal(t); env.Reason != ReasonLandNotResolved {
		t.Errorf("reason = %s", env.Reason)
	}
	if strings.Contains(f.ticketText(t), "status: done") {
		t.Error("status written on refused continue")
	}
}

func TestRunTicketsLand_ContinueStampsMarksDoneAndClears(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	pendingConflict(t, f)
	*f.pickActive = false
	f.deps.RevParse = func(_, _ string) (string, error) { return "post", nil }
	stamped := false
	f.deps.AppendTrailers = func(string, ...git.Trailer) error { stamped = true; return nil }
	f.in.Continue = true
	out, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if !stamped || !strings.Contains(out, `"post"`) {
		t.Errorf("stamped = %v, stdout = %s", stamped, out)
	}
	if !strings.Contains(f.ticketText(t), "status: done") {
		t.Errorf("status not done:\n%s", f.ticketText(t))
	}
	if m, _ := ralphloop.ReadLandMarker(f.lockDir); m != nil {
		t.Errorf("marker not cleared: %+v", m)
	}
	if err := ralphloop.AcquireLandLock(f.lockDir); err != nil {
		t.Errorf("lock not cleared: %v", err)
	}
}

func TestRunTicketsLand_ContinueWithoutPendingLandRefused(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.in.Continue = true
	if env := f.refusal(t); env.Reason != ReasonNoPendingLand {
		t.Errorf("reason = %s", env.Reason)
	}
}

func TestRunTicketsLand_AbortClearsMarkerAndLockOnly(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	pendingConflict(t, f)
	before := f.ticketText(t)
	aborted, stamped := false, false
	f.deps.AbortCherryPick = func(string) error { aborted = true; *f.pickActive = false; return nil }
	f.deps.AppendTrailers = func(string, ...git.Trailer) error { stamped = true; return nil }
	f.in.Abort = true
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	if !aborted || stamped {
		t.Errorf("aborted = %v, stamped = %v", aborted, stamped)
	}
	if f.ticketText(t) != before {
		t.Errorf("ticket changed:\n%s", f.ticketText(t))
	}
	if m, _ := ralphloop.ReadLandMarker(f.lockDir); m != nil {
		t.Errorf("marker not cleared: %+v", m)
	}
	if err := ralphloop.AcquireLandLock(f.lockDir); err != nil {
		t.Errorf("lock not cleared: %v", err)
	}
}

func TestRunTicketsLand_ContinueAndAbortExclusive(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.in.Continue, f.in.Abort = true, true
	if _, err := f.run(t); err == nil {
		t.Fatal("expected error")
	}
}

// writeDeadLock leaves a lock owned by a pid that cannot be running.
func writeDeadLock(t *testing.T, dir string) {
	t.Helper()
	body := `{"pid":2147483646,"time":"2026-01-02T03:04:05Z","epic":"widget-epic","ticket":"01"}`
	if err := os.WriteFile(filepath.Join(dir, "land.lock"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunTicketsLand_OrphanLockRefusalShowsOwnerAndRemedy(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	writeDeadLock(t, f.lockDir)
	env := f.refusal(t)
	if env.Reason != ReasonLandLocked || !strings.Contains(env.Message, "pid 2147483646") || !strings.Contains(env.Message, "--abort") {
		t.Errorf("envelope = %+v", env)
	}
}

func TestRunTicketsLand_AbortClearsOrphanLock(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	writeDeadLock(t, f.lockDir)
	f.in.Abort = true
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	if err := ralphloop.AcquireLandLock(f.lockDir); err != nil {
		t.Errorf("lock still held after --abort: %v", err)
	}
}

func TestRunTicketsLand_AbortRefusesLiveOrphanLock(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	if err := ralphloop.AcquireLandLock(f.lockDir); err != nil { // owned by this test process
		t.Fatal(err)
	}
	f.in.Abort = true
	if env := f.refusal(t); env.Reason != ReasonLandLocked {
		t.Errorf("reason = %s", env.Reason)
	}
}

func TestRunTicketsLand_AbortWithNoLockOrMarkerRefuses(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.in.Abort = true
	if env := f.refusal(t); env.Reason != ReasonNoPendingLand {
		t.Errorf("reason = %s", env.Reason)
	}
}
