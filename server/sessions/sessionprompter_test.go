package sessions_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

func TestSessionTargetsWrite_InternalErrorOnLaterTargetKeepsEarlierResults(t *testing.T) {
	do := func(id string, _ bool) (wire.SessionResult, error) {
		if id == "b" {
			return wire.SessionResult{}, errors.New("disk full")
		}
		return wire.SessionResult{}, nil
	}
	body := `{"targets":[{"id":"a"},{"id":"b"},{"id":"c"}]}`
	w := httptest.NewRecorder()
	new(sessions.Manager).SessionTargetsWrite(do, false)(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with per-target results", w.Code)
	}
	var out wire.TargetsResult
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 3 {
		t.Fatalf("results = %+v, want one per target", out.Results)
	}
	if out.Results[0].Status != wire.TargetDone || out.Results[2].Status != wire.TargetDone {
		t.Errorf("results = %+v, want a and c done", out.Results)
	}
	if got := out.Results[1]; got.Target != "b" || got.Status != wire.TargetFailed || !strings.Contains(got.Message, "disk full") {
		t.Errorf("b = %+v, want failed with the error", got)
	}
}

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
func (f fakePrompter) CancelAutoQueue(string, string) bool                   { return false }
func (f fakePrompter) AutoAdvanceDeadline(string) time.Time                  { return time.Time{} }
func (f fakePrompter) AutoQueueDeadline(string) time.Time                    { return time.Time{} }
func (f fakePrompter) SendAction(_, _ string, action []string)               { f.sent <- action }

func TestStartContinue_SendsTheActionThroughThePrompter(t *testing.T) {
	t.Parallel()
	fake := fakePrompter{sent: make(chan []string, 1)}
	mg := sessions.NewManager(sessions.ManagerDeps{Prompter: fake})
	t.Cleanup(mg.Watches.Close)

	action := []string{"/continue"}
	sessions.NewResumer(mg).StartContinue(t.TempDir(), sessions.Meta{ID: "abcdef"}, "Continue", action)

	select {
	case got := <-fake.sent:
		if !slices.Equal(got, action) {
			t.Errorf("sent action = %v, want %v", got, action)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the Prompter was never asked to send the action")
	}
}
