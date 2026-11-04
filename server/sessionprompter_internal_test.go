package server

import (
	"slices"
	"testing"
	"time"
)

// fakePrompter records the actions the Resumer sends through the Prompter hook.
type fakePrompter struct {
	sent chan []string
}

func (f fakePrompter) newFirstPrompt(string, string, int) FirstPrompt { return nil }
func (f fakePrompter) markTurn(string, string) (turnMark, bool)       { return turnMark{}, false }
func (f fakePrompter) settleTurn(string, string, turnMark, designRef, <-chan struct{}) {
}
func (f fakePrompter) detectDesignActionable(string, string, designRef) {}
func (f fakePrompter) cancelAutoAdvance(string, string) bool            { return false }
func (f fakePrompter) sendAction(_, _ string, action []string)          { f.sent <- action }

var _ FirstPrompt = (*firstPrompt)(nil)

func TestStartContinue_SendsTheActionThroughThePrompter(t *testing.T) {
	t.Parallel()
	s := &Server{}
	s.sessions = newManager(s)
	fake := fakePrompter{sent: make(chan []string, 1)}
	s.sessions.prompter = fake
	t.Cleanup(s.sessions.watches.close)

	action := []string{"/continue"}
	(&Resumer{srv: s}).startContinue(t.TempDir(), sessionMeta{ID: "abcdef"}, "Continue", action)

	select {
	case got := <-fake.sent:
		if !slices.Equal(got, action) {
			t.Errorf("sent action = %v, want %v", got, action)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the Prompter was never asked to send the action")
	}
}
