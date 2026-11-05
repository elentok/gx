package gitops

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/stash"
)

func key(code rune, text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code, Text: text} }

func TestNew_NothingOpen(t *testing.T) {
	m := New()
	if m.IsOpen() || m.ActionOpen() || m.OutputOpen() || m.InputFocused() || m.HasOutput() {
		t.Fatal("expected a fresh model to have everything closed")
	}
	if _, ok := m.ViewAction(80); ok {
		t.Fatal("expected no action view when closed")
	}
	_, cmd, res := m.Update(key('x', "x"))
	if cmd != nil || res != (Result{}) {
		t.Fatalf("expected closed Update to be a no-op, got cmd=%v res=%+v", cmd, res)
	}
}

func TestOutput_SetOpenAndDismiss(t *testing.T) {
	m := New()
	m.SetOutput("title", "   ")
	if m.HasOutput() {
		t.Fatal("blank output must be ignored")
	}
	m.SetOutput("title", "hello")
	if !m.HasOutput() || m.OutputOpen() {
		t.Fatal("SetOutput should record without opening")
	}
	m.OpenOutput(80, 24)
	if !m.OutputOpen() || !m.IsOpen() || m.ActionOpen() {
		t.Fatal("output modal should be open but not count as an action")
	}
	if m.ViewOutput() == "" {
		t.Fatal("expected output view while open")
	}
	next, _, res := m.Update(key(tea.KeyEscape, ""))
	if next.OutputOpen() || res.Kind != KindNone {
		t.Fatalf("esc should close output with no result, got open=%v res=%+v", next.OutputOpen(), res)
	}
}

func TestPush_PRPromptEnterIsDoneAndMutated(t *testing.T) {
	m := New()
	m.OpenPRPrompt("https://github.com/owner/repo/pull/new/feature")
	if !m.PushOpen() || !m.ActionOpen() || !m.IsOpen() {
		t.Fatal("expected push modal open")
	}
	if v, ok := m.ViewAction(80); !ok || v == "" {
		t.Fatal("expected push view")
	}

	next, cmd, res := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res.Kind != KindPush || !res.Done || res.Err != nil || res.Aborted {
		t.Fatalf("unexpected result %+v", res)
	}
	if !res.Mutated() {
		t.Fatal("finished push must report Mutated")
	}
	if next.PushOpen() {
		t.Fatal("push should close when done")
	}
	if cmd == nil {
		t.Fatal("expected URL-open and notify cmds to be kept")
	}
}

func TestStash_CancelAndOpen(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := New()
	if cmd := m.OpenStash(repo, true); cmd == nil {
		t.Fatal("expected a focus cmd from OpenStash")
	}
	if !m.StashOpen() || !m.StashStagedOnly() || !m.InputFocused() {
		t.Fatal("expected stash open, staged-only, with its input focused")
	}
	if v, ok := m.ViewAction(80); !ok || v == "" {
		t.Fatal("expected stash view")
	}

	next, _, res := m.Update(key(tea.KeyEscape, ""))
	if res.Kind != KindStash || !res.Done || res.Outcome != stash.OutcomeCancelled {
		t.Fatalf("unexpected result %+v", res)
	}
	if res.Mutated() || next.StashOpen() {
		t.Fatal("cancelled stash must close and not report Mutated")
	}
}

func TestOpenPull_And_Bump_OpenAndCancel(t *testing.T) {
	repo := testutil.TempRepo(t)

	m := New()
	_ = m.OpenPull(repo)
	if !m.ActionOpen() {
		t.Fatal("expected pull modal to open")
	}

	m = New()
	if err := m.OpenBump(repo); err != nil {
		t.Fatalf("OpenBump: %v", err)
	}
	if !m.ActionOpen() || !m.InputFocused() {
		t.Fatal("expected bump open with its picker focused")
	}
	next, _, res := m.Update(key(tea.KeyEscape, ""))
	if next.ActionOpen() || res.Kind != KindBump || res.Mutated() {
		t.Fatalf("esc should close bump without mutation, got res=%+v", res)
	}
}

func TestResultMutated(t *testing.T) {
	tests := []struct {
		name string
		res  Result
		want bool
	}{
		{"not done", Result{Kind: KindPush}, false},
		{"push done", Result{Kind: KindPush, Done: true}, true},
		{"push failed", Result{Kind: KindPush, Done: true, Err: errors.New("x")}, true},
		{"pull aborted", Result{Kind: KindPull, Done: true, Aborted: true}, true},
		{"stash stashed", Result{Kind: KindStash, Done: true, Outcome: stash.OutcomeStashed}, true},
		{"stash cancelled", Result{Kind: KindStash, Done: true, Outcome: stash.OutcomeCancelled}, false},
		{"stash error", Result{Kind: KindStash, Done: true, Err: errors.New("x")}, false},
		{"bump done", Result{Kind: KindBump, Done: true, NewTag: "v1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.res.Mutated(); got != tt.want {
				t.Fatalf("Mutated()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestStash_ErrorRecordsOutput(t *testing.T) {
	m2, cmd, res := New().mapStash(nil, stash.Result{Done: true, Err: errors.New("boom")})
	if res.Err == nil || cmd == nil || !m2.HasOutput() {
		t.Fatalf("expected error result, notify cmd and recorded output; got res=%+v cmd=%v has=%v", res, cmd, m2.HasOutput())
	}
}
