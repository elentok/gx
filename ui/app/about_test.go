package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/version"
)

func TestHelpAbout_ShowsCurrentServerAndSplitApp(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	s := Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	}
	s.Terminal = ui.TerminalTmux
	var next tea.Model = New(*repo, s)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Toggles help open, reads it and closes it again, since page models share
	// state across copies.
	helpAbout := func(conn ServerConn) string {
		next, _ = next.Update(serverConnMsg{conn: conn})
		next, _ = next.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
		view := ansi.Strip(next.(Model).View().Content)
		next, _ = next.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
		return view
	}

	view := helpAbout(ServerConn{State: ServerUp, PID: 4242})
	for _, want := range []string{"About", "version  " + version.Get(), "server   running", "pid      4242", "split    tmux"} {
		if !strings.Contains(view, want) {
			t.Errorf("help view missing %q:\n%s", want, view)
		}
	}

	view = helpAbout(ServerConn{State: ServerDown})
	if !strings.Contains(view, "server   down") {
		t.Errorf("help view missing down state:\n%s", view)
	}
	if strings.Contains(view, "pid ") {
		t.Errorf("help view shows a pid while the server is down:\n%s", view)
	}
}
