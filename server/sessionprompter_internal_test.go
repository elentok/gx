package server

import (
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"slices"
	"testing"
	"time"
)

// fakePrompter records the actions the Resumer sends through the Prompter hook.
type fakePrompter struct {
	sent chan []string
}

func (f fakePrompter) NewFirstPrompt(string, string, int) sessions.FirstPrompt { return nil }
func (f fakePrompter) MarkTurn(string, string) (chat.TurnMark, bool)           { return chat.TurnMark{}, false }
func (f fakePrompter) SettleTurn(string, string, chat.TurnMark, chat.DesignRef, <-chan struct{}) {
}
func (f fakePrompter) DetectDesignActionable(string, string, chat.DesignRef) {}
func (f fakePrompter) CancelAutoAdvance(string, string) bool                 { return false }
func (f fakePrompter) SendAction(_, _ string, action []string)               { f.sent <- action }

func TestStartContinue_SendsTheActionThroughThePrompter(t *testing.T) {
	t.Parallel()
	s := &Server{}
	fake := fakePrompter{sent: make(chan []string, 1)}
	s.sessions = sessions.NewManager(sessions.ManagerDeps{Prompter: fake})
	t.Cleanup(s.sessions.Watches.Close)

	action := []string{"/continue"}
	newResumer(s).StartContinue(t.TempDir(), sessions.Meta{ID: "abcdef"}, "Continue", action)

	select {
	case got := <-fake.sent:
		if !slices.Equal(got, action) {
			t.Errorf("sent action = %v, want %v", got, action)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the Prompter was never asked to send the action")
	}
}
