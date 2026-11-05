package tickets

import (
	"errors"
	"github.com/elentok/gx/ui/tickets/sidebar"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

func menuHasStartSession(m Model) bool {
	return m.menus.Menu.IsOpen && strings.Contains(m.menus.Menu.View(), rowactions.StartSessionLabel)
}

func selectTicketByAddress(t *testing.T, m *Model, address string) {
	t.Helper()
	for i, e := range m.sidebar.Tree.Entries() {
		r := e.Value
		if r.Kind == sidebar.NodeTicket && !r.Archived && m.sidebar.Epics[r.EpicIdx].Tickets[r.TicketIdx].Path == address {
			m.sidebar.Tree.SetSelectedIndex(i)
			return
		}
	}
	t.Fatalf("no ticket row %q", address)
}

func startSessionModel(t *testing.T) Model {
	t.Helper()
	m := withState(newServerModel(t), wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:map/01", Title: "Open", Status: "open", EpicIsMap: true},
		{Address: "gx:map/02", Title: "Done", Status: "done", EpicIsMap: true},
		{Address: "gx:loop/01", Title: "Loop", Status: "open"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	setCollapsedSection(&m, sidebar.SectionOpenMaps, false)
	setCollapsedSection(&m, sidebar.SectionOpen, false)
	return m
}

func TestStartSession_OfferedOnMapEpicAndItsOpenTicketsOnly(t *testing.T) {
	m := startSessionModel(t)
	for _, tc := range []struct {
		address string
		want    bool
	}{{"gx:map/01", true}, {"gx:map/02", false}, {"gx:loop/01", false}} {
		selectTicketByAddress(t, &m, tc.address)
		_, ready := requestTicketMenu(t, m)
		got := slices.ContainsFunc(ready.Items, func(it contextmenu.Item[rowactions.TicketTarget]) bool {
			return it.Label == rowactions.StartSessionLabel
		})
		if got != tc.want {
			t.Errorf("%s: start session offered = %v, want %v", tc.address, got, tc.want)
		}
	}

	if menuHasStartSession(openEpicMenuOn(t, m, "gx:loop")) {
		t.Error("start session offered on a loop epic")
	}
	if !menuHasStartSession(openEpicMenuOn(t, m, "gx:map")) {
		t.Error("start session not offered on a map epic")
	}
}

func TestStartSession_CompletedMapEpicSuggestsStartAndMerge(t *testing.T) {
	m := withState(newServerModel(t), wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:done-map/01", Title: "Done", Status: "done", EpicIsMap: true},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	setCollapsedSection(&m, sidebar.SectionCompleteMaps, false)
	selectEpicRow(t, &m, "gx:done-map")
	var suggested []string
	for _, it := range openEpicMenuOn(t, m, "gx:done-map").menus.Menu.Items() {
		if it.Group == contextmenu.Suggested {
			suggested = append(suggested, it.Label)
		}
	}
	if want := []string{rowactions.StartSessionLabel, "Merge"}; !slices.Equal(suggested, want) {
		t.Errorf("suggested = %v, want %v", suggested, want)
	}
}

func TestStartSession_RunsWithoutAServer(t *testing.T) {
	m := startSessionModel(t)
	m.server.API = nil
	m = openEpicMenuOn(t, m, "gx:map")
	if _, cmd := chooseTicketMenuItem(t, m, "Start session"); cmd == nil {
		t.Error("start session issued nothing on a map epic with no server")
	}
}

func TestStartSession_ChoosingItRequestsThePrefilledForm(t *testing.T) {
	for _, tc := range []struct {
		address string
		epicRow bool
		want    tabapi.StartSessionMsg
	}{
		{"gx:map", true, tabapi.StartSessionMsg{Project: "gx", Name: "map", Prompt: "/gx-design gx:map", Ticket: "gx:map"}},
		{"gx:map/01", false, tabapi.StartSessionMsg{Project: "gx", Name: "map/01", Prompt: "/gx-design gx:map/01", Ticket: "gx:map/01"}},
	} {
		var calls []string
		api := tickettest.FakeServerAPI{Calls: &calls}
		m := startSessionModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api}).WithSessions(api)
		var cmd tea.Cmd
		if tc.epicRow {
			_, cmd = chooseTicketMenuItem(t, openEpicMenuOn(t, m, tc.address), "Start session")
		} else {
			selectTicketByAddress(t, &m, tc.address)
			_, cmd = chooseTicketMenuItem(t, pressMenuKey(t, m), "Start session")
		}
		if cmd == nil {
			t.Fatalf("%s: choosing start session issued no cmd", tc.address)
		}
		if got := cmd(); got != tc.want {
			t.Errorf("%s: msg = %+v, want %+v", tc.address, got, tc.want)
		}
		if want := []string{"design-worktree gx:map"}; !slices.Equal(calls, want) {
			t.Errorf("%s: server calls = %v, want %v (no ticket status written)", tc.address, calls, want)
		}
	}
}

func TestStartSession_FillsDirWithTheDesignWorktree(t *testing.T) {
	api := tickettest.FakeServerAPI{DesignWorktreeRes: wire.DesignWorktreeResult{Path: "/wt/design-map"}}
	m := startSessionModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api}).WithSessions(api)
	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:map"), "Start session")
	msg, ok := cmd().(tabapi.StartSessionMsg)
	if !ok {
		t.Fatalf("not a tabapi.StartSessionMsg: %T", cmd())
	}
	if msg.Dir != "/wt/design-map" {
		t.Errorf("Dir = %q, want the design worktree", msg.Dir)
	}
}

func TestStartSession_CarriesTheRebaseNoteAndTrustFlag(t *testing.T) {
	api := tickettest.FakeServerAPI{DesignWorktreeRes: wire.DesignWorktreeResult{Path: "/wt/design-map", Note: "not rebased: dirty"}}
	m := startSessionModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api}).WithSessions(api)
	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:map"), "Start session")
	msg, ok := cmd().(tabapi.StartSessionMsg)
	if !ok {
		t.Fatalf("not a tabapi.StartSessionMsg: %T", cmd())
	}
	if msg.RebaseNote != "not rebased: dirty" || !msg.MadeWorktree {
		t.Errorf("RebaseNote, MadeWorktree = %q, %v; want the note, true", msg.RebaseNote, msg.MadeWorktree)
	}
}

func TestStartSession_ServerErrorIsShownAndOpensNoForm(t *testing.T) {
	api := tickettest.FakeServerAPI{DesignWorktreeErr: errors.New("500 boom")}
	m := startSessionModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api}).WithSessions(api)
	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:map"), "Start session")
	got := cmd()
	if _, form := got.(tabapi.StartSessionMsg); form {
		t.Fatal("a server error opened the create form")
	}
	if _, ok := got.(notify.NotifyMsg); !ok {
		t.Errorf("msg = %T, want a notification", got)
	}
}

func TestStartSession_NotRunningFallsBackToTrunk(t *testing.T) {
	api := tickettest.FakeServerAPI{DesignWorktreeErr: syscall.ECONNREFUSED}
	m := startSessionModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api}).WithSessions(api)
	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:map"), "Start session")
	msg, ok := cmd().(tabapi.StartSessionMsg)
	if !ok {
		t.Fatalf("not a tabapi.StartSessionMsg: %T", cmd())
	}
	if msg.MadeWorktree || msg.RebaseNote != "" {
		t.Errorf("trunk fallback = %+v, want no note and no trust flag", msg)
	}
}

func TestStartSession_RefusalShowsMessageAndOpensNoForm(t *testing.T) {
	api := tickettest.FakeServerAPI{DesignWorktreeRes: wire.DesignWorktreeResult{Refused: true, Message: "another session is live"}}
	m := startSessionModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api}).WithSessions(api)
	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:map"), "Start session")
	got := cmd()
	if _, form := got.(tabapi.StartSessionMsg); form {
		t.Fatal("a refusal opened the create form")
	}
	n, ok := got.(notify.NotifyMsg)
	if !ok || !strings.Contains(n.Message, "another session is live") {
		t.Errorf("msg = %+v, want a notification carrying the refusal", got)
	}
}

func TestStartSession_CarriesTheTrunkCheckout(t *testing.T) {
	repo := testutil.TempRepo(t)
	bare := filepath.Join(t.TempDir(), "r.git")
	testutil.MustGitExported(t, repo, "clone", "--bare", repo, bare)
	trunkWt := filepath.Join(t.TempDir(), "main-wt")
	testutil.MustGitExported(t, bare, "worktree", "add", trunkWt, "main")

	for _, tc := range []struct {
		name, repo, want string
	}{
		{"normal repo", repo, repo},
		{"bare repo with a trunk worktree", bare, trunkWt},
		{"bare repo without one", bare, ""},
	} {
		store := t.TempDir()
		testutil.Mkdir(t, filepath.Join(store, "gx"))
		testutil.WriteFile(t, filepath.Join(store, "gx"), "project.json", `{"name":"gx","repo":"`+tc.repo+`","trunk":"main"}`)
		if tc.name == "bare repo without one" {
			testutil.MustGitExported(t, bare, "worktree", "remove", "--force", trunkWt)
		}
		msg, ok := rowactions.StartSessionCmd(nil, store, "gx:map")().(tabapi.StartSessionMsg)
		if !ok {
			t.Fatalf("%s: not a tabapi.StartSessionMsg", tc.name)
		}
		want := tc.want
		if resolved, err := filepath.EvalSymlinks(want); err == nil && want != "" {
			want = resolved
		}
		if msg.Dir != want {
			t.Errorf("%s: Dir = %q, want %q", tc.name, msg.Dir, want)
		}
	}
}
