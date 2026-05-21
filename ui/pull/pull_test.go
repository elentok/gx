package pull

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

func openedModel() Model {
	m := New()
	m.IsOpen = true
	m.phase = phaseChecking
	m.root = "/fake"
	m.log = ui.NewCommandOutputLog()
	return m
}

func modelAtStashConfirm() Model {
	m := openedModel()
	m.phase = phaseStashConfirm
	m.stashConfirmYes = true
	return m
}

// changesCheckMsg with no changes → goes straight to pulling (no stash confirm).
func TestChangesCheckNoChanges_StartsPulling(t *testing.T) {
	m := openedModel()
	m, cmd, _ := m.Update(changesCheckMsg{hasChanges: false})
	if m.phase != phasePulling {
		t.Fatalf("expected phasePulling, got %v", m.phase)
	}
	if cmd == nil {
		t.Fatal("expected a command to start pulling")
	}
	if m.stashed {
		t.Fatal("stashed should be false when there were no changes")
	}
}

// changesCheckMsg with changes → stops at phaseStashConfirm, no command issued.
func TestChangesCheckHasChanges_ShowsStashConfirm(t *testing.T) {
	m := openedModel()
	m, cmd, _ := m.Update(changesCheckMsg{hasChanges: true})
	if m.phase != phaseStashConfirm {
		t.Fatalf("expected phaseStashConfirm, got %v", m.phase)
	}
	if cmd != nil {
		t.Fatal("expected no command at stash confirm (no stash should start yet)")
	}
	if m.stashed {
		t.Fatal("stashed must not be set before user confirms")
	}
}

// changesCheckMsg with error → goes to phaseFailed.
func TestChangesCheckError_Fails(t *testing.T) {
	m := openedModel()
	m, cmd, _ := m.Update(changesCheckMsg{err: errFake})
	if m.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %v", m.phase)
	}
	if cmd != nil {
		t.Fatal("expected no command on error")
	}
}

// 'y' at phaseStashConfirm → starts stashing, stashed=true.
func TestStashConfirmYes_StartsStash(t *testing.T) {
	m := modelAtStashConfirm()
	m, cmd, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.phase != phaseStashing {
		t.Fatalf("expected phaseStashing, got %v", m.phase)
	}
	if !m.stashed {
		t.Fatal("stashed should be true after confirming stash")
	}
	if cmd == nil {
		t.Fatal("expected a stash command")
	}
}

// enter at phaseStashConfirm (default yes) → starts stashing.
func TestStashConfirmEnter_StartsStash(t *testing.T) {
	m := modelAtStashConfirm()
	m, cmd, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != phaseStashing {
		t.Fatalf("expected phaseStashing after enter, got %v", m.phase)
	}
	if cmd == nil {
		t.Fatal("expected a stash command")
	}
}

// 'n' at phaseStashConfirm → closes cleanly, no stash, Done=true, no error.
func TestStashConfirmNo_ClosesCleanly(t *testing.T) {
	m := modelAtStashConfirm()
	m, _, result := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.IsOpen {
		t.Fatal("expected IsOpen=false after declining stash confirm")
	}
	if !result.Done {
		t.Fatal("expected Result.Done=true")
	}
	if result.Err != nil {
		t.Fatalf("expected no error on decline, got %v", result.Err)
	}
	if m.stashed {
		t.Fatal("stashed must be false when user declined")
	}
}

// esc at phaseStashConfirm → same clean close.
func TestStashConfirmEsc_ClosesCleanly(t *testing.T) {
	m := modelAtStashConfirm()
	m, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.IsOpen {
		t.Fatal("expected IsOpen=false after esc")
	}
	if !result.Done {
		t.Fatal("expected Result.Done=true")
	}
	if result.Err != nil {
		t.Fatalf("expected no error on esc, got %v", result.Err)
	}
}

// stashDoneMsg success → advances to phasePulling.
func TestStashDoneSuccess_StartsPulling(t *testing.T) {
	m := openedModel()
	m.phase = phaseStashing
	m.stashed = true
	m, cmd, _ := m.Update(stashDoneMsg{output: "stash output"})
	if m.phase != phasePulling {
		t.Fatalf("expected phasePulling, got %v", m.phase)
	}
	if cmd == nil {
		t.Fatal("expected pull command")
	}
}

// stashDoneMsg error → phaseFailed.
func TestStashDoneError_Fails(t *testing.T) {
	m := openedModel()
	m.phase = phaseStashing
	next, _, _ := m.Update(stashDoneMsg{err: errFake})
	if next.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %v", next.phase)
	}
}

// pullDoneMsg success, not stashed → closes and Done.
func TestPullDoneSuccess_NoStash_Closes(t *testing.T) {
	m := openedModel()
	m.phase = phasePulling
	next, _, result := m.Update(pullDoneMsg{output: "pulled"})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false after successful pull")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

// pullDoneMsg success, stashed → starts stash pop.
func TestPullDoneSuccess_WithStash_StartsStashPop(t *testing.T) {
	m := openedModel()
	m.phase = phasePulling
	m.stashed = true
	next, cmd, _ := m.Update(pullDoneMsg{output: "pulled"})
	if next.phase != phaseStashPopping {
		t.Fatalf("expected phaseStashPopping, got %v", next.phase)
	}
	if cmd == nil {
		t.Fatal("expected stash pop command")
	}
}

// pullDoneMsg error, not stashed → phaseFailed.
func TestPullDoneError_NoStash_Fails(t *testing.T) {
	m := openedModel()
	m.phase = phasePulling
	next, _, _ := m.Update(pullDoneMsg{err: errFake})
	if next.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %v", next.phase)
	}
}

// pullDoneMsg error, stashed → phasePopStashConfirm so user can recover stash.
func TestPullDoneError_WithStash_ShowsPopConfirm(t *testing.T) {
	m := openedModel()
	m.phase = phasePulling
	m.stashed = true
	next, _, _ := m.Update(pullDoneMsg{err: errFake})
	if next.phase != phasePopStashConfirm {
		t.Fatalf("expected phasePopStashConfirm, got %v", next.phase)
	}
}

// stashPopDoneMsg success → closes and Done.
func TestStashPopDoneSuccess_Closes(t *testing.T) {
	m := openedModel()
	m.phase = phaseStashPopping
	next, _, result := m.Update(stashPopDoneMsg{output: "popped"})
	if next.IsOpen {
		t.Fatal("expected IsOpen=false")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

// stashPopDoneMsg error → phaseFailed.
func TestStashPopDoneError_Fails(t *testing.T) {
	m := openedModel()
	m.phase = phaseStashPopping
	next, _, _ := m.Update(stashPopDoneMsg{err: errFake})
	if next.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %v", next.phase)
	}
}

// esc at phaseFailed → closes with error.
func TestFailedEsc_Closes(t *testing.T) {
	m := openedModel()
	m.phase = phaseFailed
	m.failErr = errFake
	m, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.IsOpen {
		t.Fatal("expected IsOpen=false")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
	if result.Err == nil {
		t.Fatal("expected error in result")
	}
}

// phasePopStashConfirm accept → starts stash pop.
func TestPopStashConfirmYes_StartsStashPop(t *testing.T) {
	m := openedModel()
	m.phase = phasePopStashConfirm
	m.stashYes = true
	m, cmd, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != phaseStashPopping {
		t.Fatalf("expected phaseStashPopping, got %v", m.phase)
	}
	if cmd == nil {
		t.Fatal("expected stash pop command")
	}
}

// phasePopStashConfirm decline → closes with Done.
func TestPopStashConfirmNo_Closes(t *testing.T) {
	m := openedModel()
	m.phase = phasePopStashConfirm
	m.stashYes = true
	m, _, result := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.IsOpen {
		t.Fatal("expected IsOpen=false")
	}
	if !result.Done {
		t.Fatal("expected Done=true")
	}
}

// errFake is a sentinel error for testing.
var errFake = fakeErr("test error")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestModalWidth(t *testing.T) {
	if got := modalWidth(0); got != 56 {
		t.Errorf("modalWidth(0) = %d, want 56", got)
	}
	if got := modalWidth(300); got != 100 {
		t.Errorf("modalWidth(300) = %d, want 100", got)
	}
	if got := modalWidth(120); got != 60 {
		t.Errorf("modalWidth(120) = %d, want 60", got)
	}
}

func TestHandlePoll_NilRunner(t *testing.T) {
	m := openedModel()
	m.activeRunner = nil
	next, cmd, result := m.handlePoll()
	if cmd != nil || result.Done {
		t.Error("handlePoll with nil runner should return empty result")
	}
	_ = next
}

func TestView_CheckingPhase(t *testing.T) {
	m := openedModel()
	m.phase = phaseChecking
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in checking phase")
	}
}

func TestView_StashConfirmPhase(t *testing.T) {
	m := openedModel()
	m.phase = phaseStashConfirm
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in stash confirm phase")
	}
}

func TestView_FailedPhase(t *testing.T) {
	m := openedModel()
	m.phase = phaseFailed
	m.failErr = fakeErr("something failed")
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in failed phase")
	}
}

func TestView_PullingPhase(t *testing.T) {
	m := openedModel()
	m.phase = phasePulling
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in pulling phase")
	}
}

func TestOpen_InitializesModel(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := New()
	cmd := m.Open(repo)
	if !m.IsOpen {
		t.Error("expected IsOpen=true after Open")
	}
	if m.phase != phaseChecking {
		t.Errorf("expected phaseChecking, got %v", m.phase)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (cmdCheckChanges)")
	}
	if m.log == nil {
		t.Error("expected log to be initialized")
	}
}

func TestCompleteCurrentStep_WithSteps(t *testing.T) {
	m := openedModel()
	m.steps = []components.Step{{TitleBefore: "pull", IsRunning: true}}
	m.completeCurrentStep()
	if !m.steps[0].IsDone {
		t.Error("expected IsDone=true after completeCurrentStep")
	}
	if m.steps[0].IsRunning {
		t.Error("expected IsRunning=false after completeCurrentStep")
	}
}

func TestFailCurrentStep_WithSteps(t *testing.T) {
	m := openedModel()
	m.steps = []components.Step{{TitleBefore: "pull", IsRunning: true}}
	m.failCurrentStep()
	if !m.steps[0].HasFailed {
		t.Error("expected HasFailed=true after failCurrentStep")
	}
	if m.steps[0].IsRunning {
		t.Error("expected IsRunning=false after failCurrentStep")
	}
}

func TestView_PopStashConfirmPhase(t *testing.T) {
	m := openedModel()
	m.phase = phasePopStashConfirm
	m.stashYes = true
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in pop stash confirm phase")
	}
}

func TestHandleStashDone_WithSteps_Success(t *testing.T) {
	m := openedModel()
	m.root = testutil.TempRepo(t)
	m.phase = phaseStashing
	m.stashed = true
	m.steps = []components.Step{{TitleBefore: "stash", IsRunning: true}}
	next, cmd, _ := m.Update(stashDoneMsg{output: "stash output"})
	if !m.steps[0].IsRunning && next.steps[0].IsDone {
		// step completed
	}
	if next.phase != phasePulling {
		t.Fatalf("expected phasePulling, got %v", next.phase)
	}
	if cmd == nil {
		t.Fatal("expected pull command")
	}
}

func TestHandleStashDone_WithSteps_Error(t *testing.T) {
	m := openedModel()
	m.phase = phaseStashing
	m.steps = []components.Step{{TitleBefore: "stash", IsRunning: true}}
	next, _, _ := m.Update(stashDoneMsg{err: errFake})
	if next.phase != phaseFailed {
		t.Fatalf("expected phaseFailed, got %v", next.phase)
	}
	if !next.steps[0].HasFailed {
		t.Error("expected step HasFailed=true")
	}
}

func TestUpdate_SpinnerTick(t *testing.T) {
	m := openedModel()
	m.phase = phasePulling
	_, cmd, result := m.Update(spinner.TickMsg{})
	if result.Done {
		t.Fatal("spinner tick should not emit done")
	}
	_ = cmd
}

func TestUpdate_PollMsg_NilRunner(t *testing.T) {
	m := openedModel()
	m.activeRunner = nil
	_, cmd, result := m.Update(pollMsg{})
	if cmd != nil || result.Done {
		t.Fatal("pollMsg with nil runner should return empty result")
	}
}

func TestView_WithSteps(t *testing.T) {
	m := openedModel()
	m.phase = phasePulling
	m.steps = []components.Step{{TitleBefore: "pull", IsRunning: true}}
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view with steps")
	}
}

func TestHandleKey_StashConfirm_UnhandledKey(t *testing.T) {
	m := modelAtStashConfirm()
	_, _, result := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if result.Done {
		t.Fatal("unhandled key should not emit done")
	}
}

func TestHandleKey_PopStashConfirm_UnhandledKey(t *testing.T) {
	m := openedModel()
	m.phase = phasePopStashConfirm
	m.stashYes = true
	_, _, result := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if result.Done {
		t.Fatal("unhandled key should not emit done")
	}
}

func TestView_FailedPhase_WithSteps(t *testing.T) {
	m := openedModel()
	m.phase = phaseFailed
	m.failErr = fakeErr("pull failed")
	m.steps = []components.Step{{TitleBefore: "pull", HasFailed: true}}
	view := m.View(120)
	if view == "" {
		t.Error("expected non-empty view in failed phase with steps")
	}
}

func TestHandleKey_Failed_UnhandledKey(t *testing.T) {
	m := openedModel()
	m.phase = phaseFailed
	m.failErr = errFake
	next, _, result := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !next.IsOpen {
		t.Fatal("expected IsOpen to remain true for unhandled key at phaseFailed")
	}
	if result.Done {
		t.Fatal("unhandled key should not emit done")
	}
}

func TestCmdCheckChanges_ExecutesOnRepo(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	cmd := cmdCheckChanges(repo)
	msg := cmd()
	result, ok := msg.(changesCheckMsg)
	if !ok {
		t.Fatalf("expected changesCheckMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("cmdCheckChanges failed: %v", result.err)
	}
	if result.hasChanges {
		t.Error("expected no changes in clean repo")
	}
}
