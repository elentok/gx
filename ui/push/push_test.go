package push

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

const testPRURL = "https://github.com/owner/repo/pull/new/feature"

func newModelAtPRPrompt() Model {
	m := New()
	m.OpenAtPRPrompt(testPRURL)
	return m
}

func TestPRPromptAcceptReturnsOpenURLCmd(t *testing.T) {
	m := newModelAtPRPrompt()

	_, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !result.Done {
		t.Fatal("expected Done=true after accepting PR prompt")
	}
	if cmd == nil {
		t.Fatal("expected non-nil URL-opener cmd after accepting PR prompt")
	}
}

func TestPRPromptRejectReturnsDoneWithNoCmd(t *testing.T) {
	m := newModelAtPRPrompt()
	m.prYes = false

	_, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !result.Done {
		t.Fatal("expected Done=true after rejecting PR prompt")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd after rejecting PR prompt")
	}
}

// phaseConfirm accept → starts fetch.
func TestConfirmAccept_StartsFetch(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseConfirm
	m.yes = true
	m.remote = "origin"
	next, cmd, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if result.Done {
		t.Fatal("expected Done=false while fetching")
	}
	if next.phase != phaseFetching {
		t.Fatalf("expected phaseFetching, got %d", next.phase)
	}
	if cmd == nil {
		t.Fatal("expected non-nil fetch command")
	}
}

// phaseConfirm decline → closes.
func TestConfirmDecline_Closes(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseConfirm
	m.yes = true
	next, _, result := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after decline")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

// runnerDoneMsg error → phaseFailed.
func TestRunnerDoneError_Fails(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseFetching
	next, _, _ := m.Update(runnerDoneMsg{phase: phaseFetching, err: fakeErr("network error")})
	if next.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %d", next.phase)
	}
}

// phaseFailed esc → closes with error.
func TestFailedEsc_Closes(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseFailed
	m.failErr = fakeErr("oops")
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false")
	}
	if !result.Done || result.Err == nil {
		t.Fatal("expected Done=true with error")
	}
}

// humanizeOrUnknown returns "unknown time" for zero time.
func TestHumanizeOrUnknown_Zero(t *testing.T) {
	got := humanizeOrUnknown(time.Time{})
	if got != "unknown time" {
		t.Errorf("got %q, want 'unknown time'", got)
	}
}

// humanizeOrUnknown returns a relative string for a non-zero time.
func TestHumanizeOrUnknown_NonZero(t *testing.T) {
	got := humanizeOrUnknown(time.Now().Add(-1 * time.Hour))
	if got == "unknown time" {
		t.Error("expected relative time, got 'unknown time'")
	}
}

// stepPushTag uses the tag field.
func TestStepPushTag_UsesTagField(t *testing.T) {
	m := New()
	m.tag = "v1.2.3"
	step := m.stepPushTag()
	if step.TitleBefore != "push tag v1.2.3" {
		t.Errorf("unexpected TitleBefore: %q", step.TitleBefore)
	}
}

func newModelWithLog() Model {
	m := New()
	m.IsOpen = true
	m.log = ui.NewCommandOutputLog()
	return m
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestPRPromptTransitionFromPushOutput(t *testing.T) {
	m := New()
	m.IsOpen = true
	m.log = ui.NewCommandOutputLog()
	m.phase = phasePushing

	next, cmd, result := m.Update(runnerDoneMsg{
		phase:  phasePushing,
		output: "remote:   " + testPRURL + "\n",
	})

	if result.Done {
		t.Fatal("expected not Done: should show PR prompt first")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd at PR prompt transition, got non-nil")
	}
	if next.phase != phasePRPrompt {
		t.Fatalf("expected phasePRPrompt, got phase=%d", next.phase)
	}
	if next.prURL != testPRURL {
		t.Fatalf("prURL=%q, want %q", next.prURL, testPRURL)
	}
}

func TestModalWidth(t *testing.T) {
	// min clamp
	if got := modalWidth(0); got != 56 {
		t.Errorf("modalWidth(0) = %d, want 56", got)
	}
	// max clamp
	if got := modalWidth(300); got != 100 {
		t.Errorf("modalWidth(300) = %d, want 100", got)
	}
	// half of 120 = 60, within [56, 100]
	if got := modalWidth(120); got != 60 {
		t.Errorf("modalWidth(120) = %d, want 60", got)
	}
}

func TestConfirmPrompt_NoTag(t *testing.T) {
	m := New()
	m.branch = "main"
	m.remote = "origin"
	got := m.confirmPrompt()
	if got == "" {
		t.Error("expected non-empty confirmPrompt")
	}
}

func TestConfirmPrompt_WithTag(t *testing.T) {
	m := New()
	m.branch = "main"
	m.remote = "origin"
	m.tag = "v1.0.0"
	got := m.confirmPrompt()
	if got == "" {
		t.Error("expected non-empty confirmPrompt with tag")
	}
}

func TestSelectedMenuValue_Empty(t *testing.T) {
	if got := selectedMenuValue(components.MenuState{}); got != "" {
		t.Errorf("selectedMenuValue empty = %q, want empty", got)
	}
}

func TestView_ConfirmPhase(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseConfirm
	m.branch = "main"
	m.remote = "origin"
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in confirm phase")
	}
}

func TestView_FailedPhase(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseFailed
	m.failErr = fakeErr("something failed")
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in failed phase")
	}
}

func TestHandlePoll_NilRunner(t *testing.T) {
	m := newModelWithLog()
	m.activeRunner = nil
	next, cmd, result := m.handlePoll()
	if cmd != nil || result.Done {
		t.Error("handlePoll with nil runner should return empty result")
	}
	_ = next
}

func TestOpen_InitializesModel(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := New()
	if err := m.Open(repo); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !m.IsOpen {
		t.Error("expected IsOpen=true")
	}
	if m.phase != phaseConfirm {
		t.Errorf("expected phaseConfirm, got %v", m.phase)
	}
	if m.branch == "" {
		t.Error("expected branch to be set")
	}
	if m.log == nil {
		t.Error("expected log to be initialized")
	}
}

func TestOpenWithTag_SetsTag(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := New()
	if err := m.OpenWithTag(repo, "v1.0.0"); err != nil {
		t.Fatalf("OpenWithTag: %v", err)
	}
	if m.tag != "v1.0.0" {
		t.Errorf("expected tag=v1.0.0, got %q", m.tag)
	}
	if !m.IsOpen {
		t.Error("expected IsOpen=true")
	}
}

func TestRunnerArgs_AllPhases(t *testing.T) {
	m := New()
	m.remote = "origin"
	m.branch = "main"
	m.tag = "v1.0.0"
	m.divergence = &git.PushDivergence{Upstream: "origin/main"}

	cases := []struct {
		phase phase
		want  []string
	}{
		{phaseFetching, []string{"fetch", "origin"}},
		{phaseRebasing, []string{"rebase", "origin/main"}},
		{phasePushing, []string{"push", "origin", "main"}},
		{phaseTagPushing, []string{"push", "origin", "v1.0.0"}},
		{phaseForcePushing, []string{"push", "--force", "origin", "main"}},
	}
	for _, tc := range cases {
		got := m.runnerArgs(tc.phase)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("runnerArgs(%v) = %v, want %v", tc.phase, got, tc.want)
		}
	}
}

func TestRunnerArgs_RebasingNoUpstream(t *testing.T) {
	m := New()
	m.remote = "origin"
	m.divergence = &git.PushDivergence{Upstream: "", Remote: "origin/main"}
	got := m.runnerArgs(phaseRebasing)
	if len(got) < 1 || got[0] != "rebase" {
		t.Errorf("runnerArgs phaseRebasing no upstream = %v, want rebase ...", got)
	}
}

func TestSelectedMenuValue_WithCursor(t *testing.T) {
	state := components.MenuState{
		Items: []components.MenuItem{
			{Label: "A", Value: "alpha"},
			{Label: "B", Value: "beta"},
		},
		Cursor: 1,
	}
	if got := selectedMenuValue(state); got != "beta" {
		t.Errorf("selectedMenuValue cursor=1 = %q, want 'beta'", got)
	}
}

func TestSelectedMenuValue_OutOfBounds(t *testing.T) {
	state := components.MenuState{
		Items:  []components.MenuItem{{Label: "A", Value: "alpha"}},
		Cursor: 5,
	}
	if got := selectedMenuValue(state); got != "" {
		t.Errorf("selectedMenuValue out-of-bounds = %q, want empty", got)
	}
}

func TestCompleteCurrentStep_WithSteps(t *testing.T) {
	m := newModelWithLog()
	m.steps = []components.Step{{TitleBefore: "fetch", IsRunning: true}}
	m.completeCurrentStep()
	if !m.steps[0].IsDone {
		t.Error("expected IsDone=true after completeCurrentStep")
	}
	if m.steps[0].IsRunning {
		t.Error("expected IsRunning=false after completeCurrentStep")
	}
}

func TestFailCurrentStep_WithSteps(t *testing.T) {
	m := newModelWithLog()
	m.steps = []components.Step{{TitleBefore: "push", IsRunning: true}}
	m.failCurrentStep()
	if !m.steps[0].HasFailed {
		t.Error("expected HasFailed=true after failCurrentStep")
	}
	if m.steps[0].IsRunning {
		t.Error("expected IsRunning=false after failCurrentStep")
	}
}

func TestHandleRunnerDone_Rebasing_StartsPush(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := newModelWithLog()
	m.root = repo
	m.remote = "origin"
	m.branch = "main"
	m.steps = []components.Step{{TitleBefore: "rebase", IsRunning: true}}
	next, cmd, _ := m.Update(runnerDoneMsg{phase: phaseRebasing})
	if next.phase != phasePushing {
		t.Fatalf("expected phasePushing after rebase done, got %v", next.phase)
	}
	if cmd == nil {
		t.Fatal("expected non-nil push command")
	}
}

func TestHandleRunnerDone_Pushing_NoTagNoPR_Closes(t *testing.T) {
	m := newModelWithLog()
	m.phase = phasePushing
	next, _, result := m.Update(runnerDoneMsg{phase: phasePushing, output: "already up to date"})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after successful push")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

func TestHandleRunnerDone_Pushing_WithTag_StartsTagPush(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := newModelWithLog()
	m.root = repo
	m.remote = "origin"
	m.tag = "v1.0.0"
	m.steps = []components.Step{{TitleBefore: "push", IsRunning: true}}
	next, cmd, _ := m.Update(runnerDoneMsg{phase: phasePushing, output: "pushed"})
	if next.phase != phaseTagPushing {
		t.Fatalf("expected phaseTagPushing, got %v", next.phase)
	}
	if cmd == nil {
		t.Fatal("expected non-nil tag push command")
	}
}

func TestHandleRunnerDone_Pushing_NonFastForward_ForceConfirm(t *testing.T) {
	m := newModelWithLog()
	m.phase = phasePushing
	m.steps = []components.Step{{TitleBefore: "push", IsRunning: true}}
	nffErr := &git.RunError{Stderr: "[rejected] non-fast-forward"}
	next, _, result := m.Update(runnerDoneMsg{phase: phasePushing, err: nffErr})
	if next.phase != phaseForceConfirm {
		t.Fatalf("expected phaseForceConfirm, got %v", next.phase)
	}
	if result.Done {
		t.Fatal("expected Done=false at force confirm")
	}
}

func TestHandleRunnerDone_TagPushing_WithPR_ShowsPRPrompt(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseTagPushing
	m.prURL = testPRURL
	m.steps = []components.Step{{TitleBefore: "push tag", IsRunning: true}}
	next, _, result := m.Update(runnerDoneMsg{phase: phaseTagPushing})
	if next.phase != phasePRPrompt {
		t.Fatalf("expected phasePRPrompt, got %v", next.phase)
	}
	if result.Done {
		t.Fatal("expected Done=false at PR prompt")
	}
}

func TestHandleRunnerDone_TagPushing_NoPR_Closes(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseTagPushing
	m.prURL = ""
	m.steps = []components.Step{{TitleBefore: "push tag", IsRunning: true}}
	next, _, result := m.Update(runnerDoneMsg{phase: phaseTagPushing})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after tag push with no PR")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

func TestHandleRunnerDone_ForcePushing_Closes(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseForcePushing
	m.steps = []components.Step{{TitleBefore: "force push", IsRunning: true}}
	next, _, result := m.Update(runnerDoneMsg{phase: phaseForcePushing})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after force push")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

func TestHandleKey_Diverged_AbortChoice(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseDiverged
	m.menu = components.MenuState{
		Items: []components.MenuItem{
			{Label: "Rebase", Value: "rebase"},
			{Label: "Push --force", Value: "force"},
			{Label: "Abort", Value: "abort"},
		},
		Cursor: 2,
	}
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after abort")
	}
	if !result.Done {
		t.Fatal("expected Done=true after abort")
	}
}

func TestHandleKey_ForceConfirm_Decline(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseForceConfirm
	m.forceYes = true
	next, _, result := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after declining force confirm")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

func TestView_FetchingPhase(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseFetching
	m.steps = []components.Step{{TitleBefore: "fetch", IsRunning: true}}
	if view := m.View(120); view == "" {
		t.Error("expected non-empty view in fetching phase")
	}
}

func TestView_DivergedPhase(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseDiverged
	m.divergence = &git.PushDivergence{Branch: "main", Remote: "origin/main"}
	m.menu = components.MenuState{
		Items: []components.MenuItem{{Label: "Rebase", Value: "rebase"}},
	}
	if view := m.View(120); view == "" {
		t.Error("expected non-empty view in diverged phase")
	}
}

func TestView_ForceConfirmPhase(t *testing.T) {
	m := newModelWithLog()
	m.phase = phaseForceConfirm
	if view := m.View(120); view == "" {
		t.Error("expected non-empty view in force confirm phase")
	}
}

func TestView_PRPromptPhase(t *testing.T) {
	m := newModelWithLog()
	m.phase = phasePRPrompt
	m.prURL = testPRURL
	if view := m.View(120); view == "" {
		t.Error("expected non-empty view in PR prompt phase")
	}
}
