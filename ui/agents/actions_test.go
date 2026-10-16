package agents

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/notify"
)

func actionSessions() []server.SessionInfo {
	return []server.SessionInfo{
		{ID: "live", Project: "gx", Status: idleStatus},
		{ID: "done", Project: "gx", Status: server.SessionStatus{State: server.SessionExited}},
	}
}

// actionModel has the sidebar focused on session id.
func actionModel(t *testing.T, f *fakeSessionClient, id string) Model {
	t.Helper()
	m := openedModel(t, f, "", actionSessions()...)
	m.terminalFocused = false
	m.cursor = slices.IndexFunc(m.sessions, func(s server.SessionInfo) bool { return s.ID == id })
	return m
}

// act sends k and runs what it starts, returning the footer message shown.
func act(t *testing.T, m Model, k tea.KeyPressMsg) (Model, string) {
	t.Helper()
	next, cmd := m.Update(k)
	m = next.(Model)
	if cmd == nil {
		return m, ""
	}
	msg := cmd()
	if n, ok := msg.(notify.NotifyMsg); ok {
		return m, n.Message
	}
	next, cmd = m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		if n, ok := cmd().(notify.NotifyMsg); ok {
			return m, n.Message
		}
	}
	return m, ""
}

func calls(f *fakeSessionClient) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func TestActions_StopAsksAndSendsOnlyOnYes(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   []string
	}{{"n", nil}, {"y", []string{"stop live"}}} {
		t.Run(tc.answer, func(t *testing.T) {
			f := &fakeSessionClient{}
			m, _ := act(t, actionModel(t, f, "live"), letter("x"))
			if !m.confirm.IsOpen || !m.InputFocused() || !strings.Contains(plainView(m), "Stop session live?") {
				t.Fatalf("x did not ask:\n%s", plainView(m))
			}
			if got := calls(f); len(got) != 0 {
				t.Fatalf("calls before the answer = %v", got)
			}
			m, _ = act(t, m, letter(tc.answer))
			if m.confirm.IsOpen {
				t.Fatal("the confirm stayed open")
			}
			if got := calls(f); !slices.Equal(got, tc.want) {
				t.Fatalf("calls = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestActions_LeaderXStops(t *testing.T) {
	f := &fakeSessionClient{}
	m, cmd := leaderThen(t, actionModel(t, f, "live"), letter("x"))
	if cmd != nil || !m.confirm.IsOpen {
		t.Fatal("leader x did not ask")
	}
	act(t, m, letter("y"))
	if got := calls(f); !slices.Equal(got, []string{"stop live"}) {
		t.Fatalf("calls = %v", got)
	}
}

func TestActions_RemoveExitedAsksAndSends(t *testing.T) {
	f := &fakeSessionClient{}
	m, _ := act(t, actionModel(t, f, "done"), letter("X"))
	if !m.confirm.IsOpen || !strings.Contains(plainView(m), "Remove session done?") {
		t.Fatalf("X did not ask:\n%s", plainView(m))
	}
	act(t, m, letter("y"))
	if got := calls(f); !slices.Equal(got, []string{"remove done"}) {
		t.Fatalf("calls = %v", got)
	}
}

func TestActions_RemoveLiveShowsTheRefusal(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Refused: true, Message: "session live is live"}}
	m, note := act(t, actionModel(t, f, "live"), letter("X"))
	if m.confirm.IsOpen {
		t.Fatal("X on a live session asked")
	}
	if note != "session live is live" {
		t.Fatalf("footer = %q, want the refusal", note)
	}
}

func TestActions_ResumeReattachesTheNewHolder(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Session: &server.SessionInfo{ID: "done", Project: "gx", Status: idleStatus}}}
	m := actionModel(t, f, "done")
	for name, run := range map[string]func(Model) Model{
		"sidebar": func(m Model) Model { m, _ = act(t, m, letter("r")); return m },
		"leader": func(m Model) Model {
			m, cmd := leaderThen(t, m, letter("r"))
			return update1(t, m, cmd())
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := run(m)
			if got := calls(f); got[len(got)-1] != "resume done" {
				t.Fatalf("calls = %v", got)
			}
			if m.attach == nil || m.attach.id != "done" {
				t.Fatalf("attach = %+v, want one to done", m.attach)
			}
			assertFocus(t, m, true, "done")
		})
	}
}

func TestActions_ResumeRefusalShowsItsMessage(t *testing.T) {
	f := &fakeSessionClient{result: server.SessionResult{Refused: true, Reason: server.ReasonSessionDirGone, Message: "dir is gone"}}
	m, note := act(t, actionModel(t, f, "done"), letter("r"))
	if note != "dir is gone" {
		t.Fatalf("footer = %q, want the refusal", note)
	}
	if m.attach != nil && m.attach.id == "done" {
		t.Fatal("a refused resume attached")
	}
}

func TestActions_WhichKeyListsThem(t *testing.T) {
	m := actionModel(t, &fakeSessionClient{}, "live")
	m = update1(t, m, ctrlA)
	view := plainView(m)
	for _, want := range []string{"stop session", "remove session", "resume session"} {
		if !strings.Contains(view, want) {
			t.Fatalf("which-key lacks %q:\n%s", want, view)
		}
	}
}
