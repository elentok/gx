package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
)

// hoverStub is a mouseRecorderStub that is a ui.HoverPage.
type hoverStub struct {
	mouseRecorderStub
	changes bool
	asked   []tea.MouseMotionMsg
}

func (s *hoverStub) HoverChanges(msg tea.MouseMotionMsg) bool {
	s.asked = append(s.asked, msg)
	return s.changes
}

func withHoverStub(m Model) (Model, *hoverStub) {
	stub := &hoverStub{}
	live := m.livePageByTab[nav.TabWorktrees]
	live.model = stub
	m.livePageByTab[nav.TabWorktrees] = live
	return m, stub
}

func TestViewUsesAllMotionOnTicketsAndQueueOnly(t *testing.T) {
	t.Parallel()
	for tab, want := range map[nav.TabID]tea.MouseMode{
		nav.TabTickets:   tea.MouseModeAllMotion,
		nav.TabQueue:     tea.MouseModeAllMotion,
		nav.TabWorktrees: tea.MouseModeCellMotion,
		nav.TabStatus:    tea.MouseModeCellMotion,
	} {
		t.Run(string(tab), func(t *testing.T) {
			t.Parallel()
			repoDir := testutil.TempRepo(t)
			repo, err := git.FindRepo(repoDir)
			if err != nil {
				t.Fatalf("FindRepo: %v", err)
			}
			m := New(*repo, Settings{
				InitialRoute:       nav.ViewState{Tab: tab, WorktreeRoot: repoDir},
				ActiveWorktreePath: repoDir,
			})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			if got := updated.(Model).View().MouseMode; got != want {
				t.Fatalf("mouse mode = %v, want %v", got, want)
			}
		})
	}
}

func TestFilterMotionDropsMotionThatKeepsTheHover(t *testing.T) {
	t.Parallel()
	m, stub := withHoverStub(newAppForMouseTest(t))
	motion := tea.MouseMotionMsg{X: 4, Y: 10}

	if got := FilterMotion(m, motion); got != nil {
		t.Fatalf("FilterMotion = %#v, want the event dropped", got)
	}
	stub.changes = true
	if got := FilterMotion(m, motion); got != motion {
		t.Fatalf("FilterMotion = %#v, want the event kept", got)
	}
	if want := (tea.MouseMotionMsg{X: 4, Y: 10 - ui.ChromeRows}); stub.asked[0] != want {
		t.Fatalf("page asked about %#v, want page coordinates %#v", stub.asked[0], want)
	}
}

func TestFilterMotionKeepsEverythingForOtherPages(t *testing.T) {
	t.Parallel()
	m, _ := withStubPage(newAppForMouseTest(t))

	for _, msg := range []tea.Msg{
		tea.MouseMotionMsg{X: 4, Y: 10},
		tea.MouseClickMsg{X: 4, Y: 10, Button: tea.MouseLeft},
	} {
		if got := FilterMotion(m, msg); got != msg {
			t.Fatalf("FilterMotion(%#v) = %#v, want it kept", msg, got)
		}
	}
}

func TestMotionOnChromeReachesHoverPage(t *testing.T) {
	t.Parallel()
	m, stub := withHoverStub(newAppForMouseTest(t))

	updated, _ := m.Update(tea.MouseMotionMsg{X: 3, Y: 0})
	m = updated.(Model)

	want := tea.MouseMotionMsg{X: 3, Y: -ui.ChromeRows}
	if len(stub.received) != 1 || stub.received[0] != want {
		t.Fatalf("page received %#v, want %#v", stub.received, want)
	}
}
