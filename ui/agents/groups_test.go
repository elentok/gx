package agents

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	ticketsui "github.com/elentok/gx/ui/tickets"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/viewmodel"
)

func groupSessions() []server.SessionInfo {
	return []server.SessionInfo{
		{ID: "r1", Name: "regular", Project: "gx"},
		{ID: "q1", Name: "queue-one", Project: "gx", Queue: true},
		{ID: "q2", Name: "queue-two", Project: "gx", Queue: true},
		{ID: "d1", Name: "design-one", Project: "gx", Design: true},
	}
}

func groupsModel(t *testing.T, dirs config.Dirs) Model {
	t.Helper()
	m := NewModel(&fakeSessionClient{}, ui.Settings{Dirs: dirs})
	m.opened = true
	m = withState(t, m, &viewmodel.State{Sessions: groupSessions()}, ticketsui.ServerLinkUp)
	return m
}

func sidebarText(m Model) string {
	var out []string
	for _, r := range m.sidebarRows(sidebarWidth - 2) {
		out = append(out, r.text)
	}
	return strings.Join(out, "\n")
}

func TestGroups_StartHiddenWithCounts(t *testing.T) {
	m := groupsModel(t, config.DirsFrom([]string{"HOME=" + t.TempDir()}))
	text := sidebarText(m)
	for _, want := range []string{"regular", "2 Queue agents hidden", "1 Design agent hidden"} {
		if !strings.Contains(text, want) {
			t.Fatalf("sidebar lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "queue-one") || strings.Contains(text, "design-one") {
		t.Fatalf("hidden agents are listed:\n%s", text)
	}
}

func TestGroups_TogglesShowAndHide(t *testing.T) {
	m := groupsModel(t, config.DirsFrom([]string{"HOME=" + t.TempDir()}))
	m = press(t, press(t, m, "t"), "q")
	text := sidebarText(m)
	if !strings.Contains(text, "queue-one") || !strings.Contains(text, "Queue agents") || strings.Contains(text, "Queue agents hidden") {
		t.Fatalf("t q did not show Queue agents:\n%s", text)
	}
	if !strings.Contains(text, "1 Design agent hidden") {
		t.Fatalf("t q touched Design agents:\n%s", text)
	}
	m = press(t, press(t, m, "t"), "d")
	if !strings.Contains(sidebarText(m), "design-one") {
		t.Fatalf("t d did not show Design agents:\n%s", sidebarText(m))
	}
	m = press(t, press(t, m, "t"), "q")
	if strings.Contains(sidebarText(m), "queue-one") {
		t.Fatalf("t q again did not hide Queue agents:\n%s", sidebarText(m))
	}
}

func TestGroups_StateSurvivesANewModel(t *testing.T) {
	dirs := config.DirsFrom([]string{"HOME=" + t.TempDir()})
	m := press(t, press(t, groupsModel(t, dirs), "t"), "d")
	if !m.showDesign {
		t.Fatal("t d did not show Design agents")
	}
	again := groupsModel(t, dirs)
	if again.showQueue || !again.showDesign {
		t.Fatalf("new model: showQueue=%v showDesign=%v, want false true", again.showQueue, again.showDesign)
	}
}

func TestGroups_SelectedAgentShowsWhileItsGroupIsHidden(t *testing.T) {
	m := groupsModel(t, config.DirsFrom([]string{"HOME=" + t.TempDir()}))
	m.cursor = 2
	text := sidebarText(m)
	if !strings.Contains(text, "queue-two") || !strings.Contains(text, "1 Queue agent hidden") {
		t.Fatalf("selected hidden-group agent not shown alone:\n%s", text)
	}
}

func TestGroups_PendingSelectShowsTheAgent(t *testing.T) {
	m := NewModel(&fakeSessionClient{}, ui.Settings{Dirs: config.DirsFrom([]string{"HOME=" + t.TempDir()})})
	m.opened = true
	m.pendingSelect = "d1"
	m = withState(t, m, &viewmodel.State{Sessions: groupSessions()}, ticketsui.ServerLinkUp)
	if got := m.selectedID(); got != "d1" || !strings.Contains(sidebarText(m), "design-one") {
		t.Fatalf("selected %q; design-one listed: %v", got, strings.Contains(sidebarText(m), "design-one"))
	}
}

func TestGroups_JumpListsHiddenAgentsAndOpeningShowsOne(t *testing.T) {
	m := press(t, groupsModel(t, config.DirsFrom([]string{"HOME=" + t.TempDir()})), "/")
	if got := len(m.jumpMatches()); got != 4 {
		t.Fatalf("jump lists %d agents, want 4", got)
	}
	for _, r := range "queue-two" {
		m = press(t, m, string(r))
	}
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.selectedID(); got != "q2" || !strings.Contains(sidebarText(m), "queue-two") {
		t.Fatalf("selected %q after jump", got)
	}
}

func TestGroups_JKSkipHiddenAgents(t *testing.T) {
	m := groupsModel(t, config.DirsFrom([]string{"HOME=" + t.TempDir()}))
	m = press(t, m, "j")
	if got := m.selectedID(); got != "r1" {
		t.Fatalf("j moved onto hidden agent %q", got)
	}
}

func TestGroups_HelpListsTheChords(t *testing.T) {
	var got []string
	for _, b := range sidebarBindings {
		got = append(got, b.Keys())
	}
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "t q") || !strings.Contains(joined, "t d") {
		t.Fatalf("sidebar help keys = %s", joined)
	}
}
