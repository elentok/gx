package tickets

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

// TestQueueHeaderStateMatchesPrototype covers ticket 05's Option B redesign
// (.scratch/tickets-queue-batch3/issues/assets/08-header-prototype.md): the
// title always encodes run state, and the body carries at most one
// state-specific line — one assertion per state.
func TestQueueHeaderStateMatchesPrototype(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}
	base := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	t.Run("not started", func(t *testing.T) {
		m := base
		if got, want := m.queueHeaderTitle(), "Queue · today $0.00"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
		lines := m.queueHeaderBodyLines()
		if len(lines) != queueHeaderReservedLines {
			t.Fatalf("body lines = %v, want %d line(s)", lines, queueHeaderReservedLines)
		}
		got := ansi.Strip(lines[0])
		if !strings.Contains(got, "Idle") || strings.Contains(got, "enter") {
			t.Fatalf("body line = %q, want idle copy with no enter hint", got)
		}
		if strings.Contains(got, "·") {
			t.Fatalf("body line = %q, want no redundant \"· enter start\" hint suffix", got)
		}
		if want := epicStatusPausedAnswerStyle.Render(got); lines[0] != want {
			t.Fatalf("body line = %q, want orange-styled %q", lines[0], want)
		}
	})

	t.Run("running", func(t *testing.T) {
		m := base
		m.runningEpics = map[string]bool{"alpha": true}
		m.summaries = map[string]wire.EpicSummary{"alpha": {State: wire.EpicStateRunning, Total: 1}}
		got := m.queueHeaderTitle()
		if !strings.HasPrefix(got, "Queue · 0 of 1 done · ") || !strings.HasSuffix(got, " implementing... · today $0.00") {
			t.Fatalf("title = %q, want \"Queue · 0 of 1 done · <spinner> implementing... · today $0.00\"", got)
		}
		want := []string{""}
		if lines := m.queueHeaderBodyLines(); !slices.Equal(lines, want) {
			t.Fatalf("body lines = %v, want %v while running (reserved slot stays blank)", lines, want)
		}
	})

	t.Run("idle but globally paused", func(t *testing.T) {
		m := base
		m.paused = true
		if got, want := m.queueHeaderTitle(), "Queue · today $0.00"; got != want {
			t.Fatalf("title = %q, want %q", got, want)
		}
		lines := m.queueHeaderBodyLines()
		if len(lines) != queueHeaderReservedLines {
			t.Fatalf("body lines = %v, want %d line(s)", lines, queueHeaderReservedLines)
		}
		got := ansi.Strip(lines[0])
		if got != "Paused. No tickets running." {
			t.Fatalf("body line = %q, want the paused copy for a paused queue", got)
		}
	})

	t.Run("idle with nothing selected", func(t *testing.T) {
		emptyRoot := t.TempDir()
		m := loadQueueModel(t, withChecked(NewQueueModel(emptyRoot, ui.Settings{}, keys.Manager{}), map[string]bool{}))
		lines := m.queueHeaderBodyLines()
		if len(lines) != queueHeaderReservedLines {
			t.Fatalf("body lines = %v, want %d line(s)", lines, queueHeaderReservedLines)
		}
		want := "No selected tickets — go to the Tickets tab first"
		if got := ansi.Strip(lines[0]); got != want {
			t.Fatalf("body line = %q, want %q", got, want)
		}
		if wantStyled := epicStatusPausedAnswerStyle.Render(want); lines[0] != wantStyled {
			t.Fatalf("body line = %q, want orange-styled %q", lines[0], wantStyled)
		}

		emptyLine := m.queueRenderOpts(80).EmptyLine
		if emptyLine != "" {
			t.Fatalf("tree EmptyLine = %q, want empty (header banner already covers no-selection case)", emptyLine)
		}
	})
}

// Without a server there is no queue to clear: "c" and "C" do nothing.
func TestQueueModelClearKeysWithoutServerDoNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-done.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "01-done.md", "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n\nBody.\n")
	done := tickettest.TicketPath(root, "alpha", "01-done.md")
	checked := map[string]bool{done: true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	for _, key := range []rune{'c', 'C'} {
		updated, _ := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		m = updated.(QueueModel)
		if m.confirm.IsOpen {
			t.Fatalf("press %q: expected no confirmation without a server", key)
		}
		if !checked[done] {
			t.Fatalf("press %q: expected nothing cleared without a server", key)
		}
	}
}

// TestQueueModelHideCompleteToggleHidesDoneTicketsButKeepsPlanValidation
// covers ticket 09: "tc" hides StatusDone rows from rows()/View() without
// affecting epicWaves' plan validation (which must keep treating the hidden
// ticket as queued), and toggling "tc" again restores it.
func TestQueueModelHideCompleteToggleHidesDoneTicketsButKeepsPlanValidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n\nBody.\n")
	first := tickettest.TicketPath(root, "alpha", "01-first.md")
	second := tickettest.TicketPath(root, "alpha", "02-second.md")
	checked := map[string]bool{first: true, second: true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	if content := m.View().Content; !strings.Contains(content, "First") {
		t.Fatalf("expected done ticket visible by default:\n%s", content)
	}
	if len(queueTicketEntries(m)) != 2 {
		t.Fatalf("expected both tickets in Entries() by default, got %d", len(queueTicketEntries(m)))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)

	rows := queueTicketEntries(m)
	if len(rows) != 1 || rows[0].Value.ticket.ticket.Path != second {
		t.Fatalf("expected only the non-done ticket in Entries() after tc, got %+v", rows)
	}
	content := m.View().Content
	if strings.Contains(content, "First") {
		t.Fatalf("expected done ticket hidden after tc:\n%s", content)
	}
	if !strings.Contains(content, "Second") {
		t.Fatalf("expected non-done ticket still visible after tc:\n%s", content)
	}
	if strings.Contains(content, "no unblocked tickets") {
		t.Fatalf("expected plan validation to still see the hidden-but-queued blocker:\n%s", content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)
	if len(queueTicketEntries(m)) != 2 {
		t.Fatalf("expected tc toggled again to restore both tickets, got %d", len(queueTicketEntries(m)))
	}
}

// TestQueueModelTChordDoesNotCollideWithClearKeymaps covers ticket 09: the
// "t"-prefix chord swallows its second key, and plain "c" (with no preceding
// "t") doesn't toggle hideComplete.
func TestQueueModelTChordDoesNotCollideWithClearKeymaps(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-done.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "01-done.md", "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-open.md", "Status: open\n\nBody.\n")
	done := tickettest.TicketPath(root, "alpha", "01-done.md")
	open := tickettest.TicketPath(root, "alpha", "02-open.md")
	checked := map[string]bool{done: true, open: true}
	m := selectFirstQueueTicketRow(t, loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked)))

	start := m.queueTree.SelectedIndex()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(QueueModel)
	if want := start + 1; m.queueTree.SelectedIndex() != want {
		t.Fatalf("expected \"t\",\"j\" to still move the selection down, got %d, want %d", m.queueTree.SelectedIndex(), want)
	}
	if m.hideComplete {
		t.Fatal("expected \"t\" followed by an unrelated key not to toggle hideComplete")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(QueueModel)
	if m.hideComplete {
		t.Fatal("expected \"t\" followed by an unrelated key not to toggle hideComplete")
	}
	if m.confirm.IsOpen {
		t.Fatal("expected \"t\",\"q\" not to open any confirmation")
	}
	if cmd == nil || !nav.IsBack(cmd()) {
		t.Fatal("expected \"t\",\"q\" to still trigger \"q\"'s own nav.Back() action")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)
	if m.hideComplete {
		t.Fatal("expected plain \"c\" not to toggle hideComplete")
	}
}

func TestQueueEditChordLaunchesEditorOnSelectedTicket(t *testing.T) {
	// not parallel-safe: t.Setenv (EDITOR) is process-wide
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	chords := []struct {
		name   string
		second string
	}{
		{"ee (in place)", "e"},
		{"es (split)", "s"},
		{"ev (vsplit)", "v"},
		{"et (tab)", "t"},
	}

	for _, tt := range chords {
		t.Run(tt.name, func(t *testing.T) {
			m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

			updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
			m = updated.(QueueModel)
			updated, cmd := m.Update(tea.KeyPressMsg{Text: tt.second})
			m = updated.(QueueModel)
			if cmd == nil {
				t.Fatalf("expected e%s to launch an editor command", tt.second)
			}

			// handleEditFileFinished completes the round trip the same way
			// regardless of how the launch itself resolved (in-place exec,
			// split, or a "not supported" warning for this test's plain
			// terminal setting) — the acceptance criterion is that the
			// finished-edit message routes back into QueueModel without a
			// type-assertion panic.
			updated, _ = m.Update(editFileFinishedMsg{})
			m = updated.(QueueModel)
		})
	}
}

func TestQueueEditChordCancelsOnEsc(t *testing.T) {
	// not parallel-safe: t.Setenv (EDITOR) is process-wide
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)
	if cmd != nil {
		t.Fatalf("expected e-esc to cancel the chord without launching a command")
	}

	// A follow-up "esc" (outside a chord) still navigates back, confirming the
	// cancel didn't leave the chord's prefix stuck mid-sequence.
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)
	if cmd == nil {
		t.Fatalf("expected plain esc after cancel to navigate back")
	}
}
