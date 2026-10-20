package app

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"
)

var update = flag.Bool("update", false, "rewrite golden files")

// titleText is the title row as drawn at the model's width, without styles.
func titleText(m Model) string {
	return ansi.Strip(m.renderTitleRow(m.width).line)
}

// titleModel is a shell on the worktrees view with no server deps, so the
// connection state comes only from the serverConnMsg the test sends.
func titleModel(t *testing.T, nerd bool, width int, conn ServerConn, actionable int) Model {
	t.Helper()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	s := Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	}
	s.UseNerdFontIcons = nerd
	m := New(*repo, s)
	m.setServerConn(ServerConn{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 10})
	if conn.State != ServerUnknown {
		next, _ = next.Update(serverConnMsg{conn: conn})
	}
	m = next.(Model)
	m.stream.vm.Sessions = nil
	for i := range actionable {
		m.stream.vm.Sessions = append(m.stream.vm.Sessions, server.SessionInfo{
			ID:         fmt.Sprintf("s%d", i),
			Actionable: server.SessionActionable{Reason: "blocked"},
		})
	}
	return m
}

var (
	connUp       = ServerConn{State: ServerUp, PID: 1}
	connDown     = ServerConn{State: ServerDown}
	connReadOnly = ServerConn{State: ServerReadOnly, PID: 1}
	connHerdr    = ServerConn{State: ServerHerdrUnavailable, PID: 1}
)

func TestTitleRow_ServerStates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		conn ServerConn
		want string
	}{
		{"unknown", ServerConn{}, "gx › worktrees ▾ ? help"},
		{"up", connUp, "gx › worktrees ▾ ● ? help"},
		{"down", connDown, "gx › worktrees ▾ ○ server down ? help"},
		{"read-only", connReadOnly, "gx › worktrees ▾ ◐ read-only — restart ? help"},
		{"herdr", connHerdr, "gx › worktrees ▾ ⚠ herdr unavailable ? help"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(strings.Fields(titleText(titleModel(t, false, 120, tc.conn, 0))), " ")
			if got != tc.want {
				t.Errorf("title = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTitleRow_ShortServerWords(t *testing.T) {
	t.Parallel()
	for conn, want := range map[ServerConn]string{
		connDown:     "○ down ?",
		connReadOnly: "◐ read-only ?",
		connHerdr:    "⚠ no herdr ?",
	} {
		// Too narrow for the full words, wide enough for the short ones.
		m := titleModel(t, false, 32, conn, 0)
		if got := titleText(m); !strings.HasSuffix(got, " "+want+" ") {
			t.Errorf("%v: title = %q, want it to end with %q", conn.State, got, want)
		}
	}
}

// TestTitleRow_DropOrder walks the steps from the widest row down; each width
// is the narrowest at which its step still fits, and one column less takes the
// next step.
func TestTitleRow_DropOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		width int
		want  string
		help  string
	}{
		{48, "gx › worktrees ▾ agents 3 ○ server down ? help", "? help"},
		{47, "gx › worktrees ▾ agents 3 ○ server down ?", "?"},
		{43, "gx › worktrees ▾ agents 3 ○ server down ?", "?"},
		{42, "gx › worktrees ▾ agents 3 ○ down ?", "?"},
		{36, "gx › worktrees ▾ agents 3 ○ down ?", "?"},
		{35, "gx › worktrees ▾ agents 3 ○ ?", "?"},
		{31, "gx › worktrees ▾ agents 3 ○ ?", "?"},
		{30, "worktrees ▾ agents 3 ○ ?", "?"},
		{26, "worktrees ▾ agents 3 ○ ?", "?"},
		{25, "worktrees agents 3 ○ ?", "?"},
		{24, "worktrees agents 3 ○ ?", "?"},
		{20, "work… agents 3 ○ ?", "?"},
		{14, "… agents 3 ○", ""},
	}
	for _, tc := range cases {
		m := titleModel(t, false, tc.width, connDown, 3)
		row := m.renderTitleRow(m.width)
		plain := ansi.Strip(row.line)
		if got := strings.Join(strings.Fields(plain), " "); got != tc.want {
			t.Errorf("width %d: title = %q, want %q", tc.width, got, tc.want)
		}
		if w := ansi.StringWidth(plain); w != tc.width {
			t.Errorf("width %d: row is %d columns", tc.width, w)
		}
		if got := ansi.Cut(plain, row.help.start, row.help.end); got != tc.help {
			t.Errorf("width %d: help span = %q, want %q", tc.width, got, tc.help)
		}
	}
}

func TestTitleRow_HealthyFitsAt40(t *testing.T) {
	t.Parallel()
	for _, nerd := range []bool{false, true} {
		got := titleText(titleModel(t, nerd, 40, connUp, 0))
		if !strings.HasPrefix(got, " gx › worktrees ▾ ") || !strings.HasSuffix(got, " ? help ") {
			t.Errorf("nerd=%v: healthy row at 40 = %q, want it whole", nerd, got)
		}
	}
}

func TestTitleRow_AgentsCountAppearsAndDisappears(t *testing.T) {
	t.Parallel()
	m := titleModel(t, false, 120, connUp, 2)
	if got := titleText(m); !strings.Contains(got, "agents 2 ● ? help") {
		t.Fatalf("title = %q, want the agents count", got)
	}
	m.stream.vm.Sessions = nil
	if got := titleText(m); strings.Contains(got, "agents") {
		t.Errorf("title = %q, want no agents count at 0", got)
	}
}

func TestTitleRow_Golden(t *testing.T) {
	t.Parallel()
	states := []struct {
		name string
		conn ServerConn
	}{{"healthy", connUp}, {"down", connDown}, {"read-only", connReadOnly}}
	for _, nerd := range []bool{false, true} {
		var b strings.Builder
		for _, width := range []int{120, 60, 40, 30} {
			for _, st := range states {
				for _, actionable := range []int{0, 3} {
					m := titleModel(t, nerd, width, st.conn, actionable)
					fmt.Fprintf(&b, "%d %s agents=%d\n%s\n", width, st.name, actionable, m.renderTitleRow(width).line)
				}
			}
		}
		name := "title_row_plain"
		if nerd {
			name = "title_row_nerd"
		}
		assertGolden(t, name, b.String())
	}
}

func assertGolden(t *testing.T, name, text string) {
	t.Helper()
	golden := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if text != string(want) {
		t.Errorf("render mismatch (run with -update to accept)\ngot:\n%s\nwant:\n%s", ansi.Strip(text), ansi.Strip(string(want)))
	}
}

// withQueue gives m an epic of seven tickets, three done, the fourth claimed;
// running decides whether the server holds a run for the claimed one.
func withQueue(m Model, mode server.QueueMode, running bool) Model {
	statuses := []string{"done", "done", "done", "claimed", "open", "open", "open"}
	m.stream.vm.Tickets = nil
	for i, st := range statuses {
		info := server.TicketInfo{Address: fmt.Sprintf("gx:alpha/%02d", i+1), Title: "t", Status: st}
		if st == "claimed" && running {
			info.ClaimedAt = time.Now()
		}
		m.stream.vm.Tickets = append(m.stream.vm.Tickets, info)
	}
	m.stream.vm.Queue = []string{"gx:alpha/04", "gx:alpha/05"}
	m.stream.vm.Mode = mode
	m.stream.setVM(m.stream.vm)
	return m
}

func TestTitleRow_QueueStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mode    server.QueueMode
		running bool
		want    string
	}{
		{"running", server.QueueModeRunning, true, "▶ ▰▰▰▱▱▱▱ 3/7 agents 2 ● ? help"},
		{"paused", server.QueueModePaused, false, "‖ ▰▰▰▱▱▱▱ 3/7 agents 2 ● ? help"},
		{"draining", server.QueueModeDraining, true, "↓ ▰▰▰▱▱▱▱ 3/7 agents 2 ● ? help"},
		{"queued but idle", server.QueueModeRunning, false, "gx › worktrees ▾ agents 2 ● ? help"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := withQueue(titleModel(t, false, 120, connUp, 2), tc.mode, tc.running)
			got := strings.Join(strings.Fields(titleText(m)), " ")
			if !strings.HasSuffix(got, tc.want) {
				t.Errorf("title = %q, want it to end with %q", got, tc.want)
			}
		})
	}
}

func TestTitleRow_QueueBarDropsFirst(t *testing.T) {
	t.Parallel()
	m := withQueue(titleModel(t, false, 44, connUp, 2), server.QueueModeRunning, true)
	got := strings.Join(strings.Fields(titleText(m)), " ")
	if want := "gx › worktrees ▾ ▶ 3/7 agents 2 ● ?"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}
