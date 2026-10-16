package worktrees

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func headerTestModel(width int) Model {
	m := New(git.Repo{}, "")
	m.ready = true
	m.worktrees = []git.Worktree{{Name: "feature-a", Branch: "feature-a"}}
	m.width = width
	m.height = 30
	return m.resized()
}

// worktreesHeader is the Worktrees panel's title row, which is the first row
// in both the stacked and the side-by-side layout.
func worktreesHeader(m Model) string {
	header := strings.Split(ansi.Strip(m.normalView()), "\n")[0]
	tableW, _ := m.splitWidth()
	return ansi.Cut(header, 0, tableW)
}

func withPaste(m Model) Model {
	m.mode = modePaste
	m.clipboard = &clipboardState{srcName: "feat-x", files: []string{"a", "b", "c"}}
	return m
}

func TestWorktreesHeaderShowsSpinnerAndLabelWhileBusy(t *testing.T) {
	t.Parallel()
	m := headerTestModel(120)
	m.spinnerActive = true
	m.spinnerLabel = "Fetching remotes…"

	header := worktreesHeader(m)
	for _, want := range []string{"Worktrees", strings.TrimSpace(ansi.Strip(m.spinner.View())), "Fetching remotes…"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header %q missing %q", header, want)
		}
	}
}

func TestWorktreesHeaderShowsPasteChipAndSource(t *testing.T) {
	t.Parallel()
	m := withPaste(headerTestModel(120))

	header := worktreesHeader(m)
	for _, want := range []string{"Worktrees", "PASTE", "3 files from feat-x"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header %q missing %q", header, want)
		}
	}
}

func TestWorktreesHeaderPasteModeHidesRunningJob(t *testing.T) {
	t.Parallel()
	m := withPaste(headerTestModel(120))
	m.spinnerActive = true
	m.spinnerLabel = "Pushing feature-a…"

	header := worktreesHeader(m)
	if strings.Contains(header, "Pushing") {
		t.Fatalf("paste mode should hide the job label, got %q", header)
	}
	if !strings.Contains(header, "PASTE") {
		t.Fatalf("header %q missing PASTE", header)
	}
}

func TestWorktreesHeaderIdleHasNoRightSide(t *testing.T) {
	t.Parallel()
	header := strings.TrimSpace(worktreesHeader(headerTestModel(120)))
	if header != "Worktrees" {
		t.Fatalf("idle header = %q, want just the title", header)
	}
}

func TestWorktreesHeaderNarrowTruncatesTextBeforeTitle(t *testing.T) {
	t.Parallel()
	m := headerTestModel(30)
	m.spinnerActive = true
	m.spinnerLabel = "Rebasing a-very-long-worktree-name..."

	header := worktreesHeader(m)
	spin := strings.TrimSpace(ansi.Strip(m.spinner.View()))
	if !strings.Contains(header, "Worktrees") || !strings.Contains(header, spin) || !strings.Contains(header, "…") {
		t.Fatalf("want full title, spinner and truncated label, got %q", header)
	}
}

func TestWorktreesHeaderTooNarrowKeepsSpinnerAndChip(t *testing.T) {
	t.Parallel()
	busy := headerTestModel(14)
	busy.spinnerActive = true
	busy.spinnerLabel = "Fetching remotes…"
	if header := worktreesHeader(busy); !strings.Contains(header, strings.TrimSpace(ansi.Strip(busy.spinner.View()))) {
		t.Fatalf("spinner dropped at narrow width: %q", header)
	}

	header := worktreesHeader(withPaste(headerTestModel(14)))
	if !strings.Contains(header, "PASTE") {
		t.Fatalf("chip dropped at narrow width: %q", header)
	}
	if strings.Contains(header, "Worktrees") || strings.Contains(header, "files") {
		t.Fatalf("want text dropped and title truncated, got %q", header)
	}
}

func TestWorktreesViewHasNoFooterRow(t *testing.T) {
	t.Parallel()
	m := headerTestModel(120)
	view := ansi.Strip(m.normalView())
	if got := len(strings.Split(view, "\n")); got != m.height {
		t.Fatalf("view rows = %d, want the full height %d", got, m.height)
	}
	if strings.Contains(view, "? help") {
		t.Fatalf("view still has the footer: %q", view)
	}
}

func TestEnteringPasteModeNotifiesTheKeys(t *testing.T) {
	t.Parallel()
	m := headerTestModel(120)
	m.mode = modeYank
	m.yankSource = git.Worktree{Name: "feat-x", Path: "/tmp/feat-x"}
	m.yankChecklist = components.Checklist{Items: []components.Item{{Label: "a", Value: "a", Checked: true}}}

	updated, cmd := m.handleYankKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := updated.(Model).mode; got != modePaste {
		t.Fatalf("mode = %v, want modePaste", got)
	}
	if cmd == nil {
		t.Fatal("expected a notification command")
	}
	want := notify.NotifyMsg{Kind: notify.KindInfo, Message: "p paste · esc cancel"}
	if got := cmd(); got != want {
		t.Fatalf("notification = %#v, want %#v", got, want)
	}
}

func TestHelpHasPasteModeSection(t *testing.T) {
	t.Parallel()
	m := New(git.Repo{}, "")
	for _, s := range m.helpModel.KeySections {
		if s.Title != "Paste mode" {
			continue
		}
		var got []string
		for _, b := range s.Bindings {
			got = append(got, b.Keys()+" "+b.Title)
		}
		if strings.Join(got, ", ") != "p paste, esc cancel" {
			t.Fatalf("paste section = %v", got)
		}
		return
	}
	t.Fatal("help has no Paste mode section")
}
