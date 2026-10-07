package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// fakeServerClient answers the handshake from fields; the tab calls the
// embedded (nil) API are not reached by these tests' messages.
type fakeServerClient struct {
	ticketsui.ServerAPI
	down     bool
	readOnly bool
}

func (f *fakeServerClient) Snapshot(context.Context) (server.Snapshot, error) {
	return server.Snapshot{}, errors.New("not needed")
}

func (f *fakeServerClient) Negotiate(context.Context, string) (apiclient.Negotiation, error) {
	if f.down {
		return apiclient.Negotiation{}, errors.New("connection refused")
	}
	return apiclient.Negotiation{Handshake: server.Handshake{Pid: 4242}, ReadOnly: f.readOnly}, nil
}

func newServerShell(t *testing.T, client ServerClient) Model {
	t.Helper()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabQueue, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
		Server:             &ServerDeps{Client: client, Build: "test"},
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// Deliver the Queue tab's initial load so it leaves "loading…".
	next, _ = next.Update(next.(Model).activePage().model.Init()())
	return next.(Model)
}

func probe(t *testing.T, m Model) Model {
	t.Helper()
	msg := m.cmdServerProbe()()
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestServerMode_ConnectionDropAndReconnect(t *testing.T) {
	client := &fakeServerClient{}
	m := newServerShell(t, client)

	m = probe(t, m)
	if got := ansi.Strip(m.tabsView()); !strings.Contains(got, "server ● pid 4242") {
		t.Fatalf("up: tabs = %q", got)
	}

	client.down = true
	m = probe(t, m)
	if got := ansi.Strip(m.tabsView()); !strings.Contains(got, "server ○ down") {
		t.Fatalf("down: tabs = %q", got)
	}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "server down — s to start") {
		t.Fatalf("down: Queue tab has no banner:\n%s", got)
	}

	client.down = false
	m = probe(t, m)
	if got := ansi.Strip(m.View().Content); strings.Contains(got, "server down") {
		t.Fatalf("up again: banner still shown:\n%s", got)
	}
}

func TestServerMode_VersionMismatchIsReadOnly(t *testing.T) {
	m := probe(t, newServerShell(t, &fakeServerClient{readOnly: true}))
	if got := ansi.Strip(m.tabsView()); !strings.Contains(got, "read-only") {
		t.Fatalf("tabs = %q", got)
	}
}

func TestInProcessMode_NoProbe(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(*repo, Settings{ActiveWorktreePath: repoDir})
	if m.serverMode() {
		t.Fatal("in-process shell reports server mode")
	}
	if got := ansi.Strip(m.tabsView()); strings.Contains(got, "server") {
		t.Fatalf("tabs = %q", got)
	}
}
