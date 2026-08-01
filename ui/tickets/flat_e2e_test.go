package tickets_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets"
)

const (
	flatTermWidth  = 120
	flatTermHeight = 40
	flatWait       = 3 * time.Second
)

func writeFlatTicket(t *testing.T, root, epic, filename, content string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func startFlatTUI(t *testing.T, root, epicName string) *teatest.TestModel {
	t.Helper()
	m := tickets.NewFlatModel(root, epicName, ui.Settings{})
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
}

func waitForFlatText(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(flatWait))
}

func flatKeyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func flatKeySpecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestFlatTUI_LaunchRendersFlatTicketList(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nFirst body.\n")
	writeFlatTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nSecond body.\n")

	tm := startFlatTUI(t, root, "my-epic")
	defer tm.Quit()

	waitForFlatText(t, tm, "First ticket")
	waitForFlatText(t, tm, "Second ticket")

	frame := tm.CurrentFrame()
	if bytes.Contains(frame, []byte("Open epics")) || bytes.Contains(frame, []byte("Closed epics")) {
		t.Fatalf("expected a flat ticket list with no epic-of-epics grouping, got:\n%s", frame)
	}
}

func TestFlatTUI_NavigationAndPreviewRendering(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nFirstbodymarker\n")
	writeFlatTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nSecondbodymarker\n")

	tm := startFlatTUI(t, root, "my-epic")
	defer tm.Quit()

	waitForFlatText(t, tm, "Firstbodymarker")

	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Secondbodymarker")

	tm.Send(flatKeyRune('l'))
	tm.Send(flatKeyRune('h'))
	waitForFlatText(t, tm, "Secondbodymarker")
}

func TestFlatTUI_RefreshAndQuit(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	tm := startFlatTUI(t, root, "my-epic")

	waitForFlatText(t, tm, "First ticket")

	tm.Send(tea.KeyPressMsg{Code: 'R', Text: "R"})
	waitForFlatText(t, tm, "refreshed")

	tm.Send(flatKeyRune('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestFlatTUI_LiveEventsDriveRowState feeds synthetic ralphloop.LiveEvents
// through WithLiveEvents and asserts each of ticket 04a's row states renders
// distinctly: a running ticket's spinner+label, a paused ticket's badge+
// reason, a needs-attention ticket's own badge+reason, and a done ticket's
// unchanged (ticket 03) dimmed rendering.
func TestFlatTUI_LiveEventsDriveRowState(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirst body.\n")
	writeFlatTicket(t, root, "my-epic", "02-paused-ticket.md", "Status: open\n\nSecond body.\n")
	writeFlatTicket(t, root, "my-epic", "03-attention-ticket.md", "Status: open\n\nThird body.\n")
	writeFlatTicket(t, root, "my-epic", "04-done-ticket.md", "Status: done\n\nFourth body.\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Running ticket")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "02", Label: "iter-02"}
	waitForFlatText(t, tm, "iter-02")
	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-02",
		PauseKind: ralphloop.PauseRateLimit, Reason: "context budget exceeded",
	}
	waitForFlatText(t, tm, "context budget exceeded")

	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventTicketStillNeedsAttention, Identifier: "03",
	}
	waitForFlatText(t, tm, "no live iteration to reattach to")

	frame := tm.CurrentFrame()
	if !bytes.Contains(frame, []byte("iter-01")) {
		t.Fatalf("expected running ticket's spinner row to still show its label, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("context budget exceeded")) {
		t.Fatalf("expected paused ticket's reason to render, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("no live iteration to reattach to")) {
		t.Fatalf("expected needs-attention ticket's reason to render, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("Done ticket")) {
		t.Fatalf("expected the done ticket's title to still render, got:\n%s", frame)
	}
}

// TestFlatTUI_SupersededTicketIgnoresStaleLiveEntry covers ticket 03: a
// needs-attention ticket that becomes superseded on disk (a mid-flight
// split) leaves its LiveEventTicketStillNeedsAttention entry in m.live,
// since reconcile.go deliberately skips superseded tickets when clearing
// live state (reconcile.go:102-105). The row must still render via the
// disk-only (dimmed) path instead of the stale paused/needs-attention badge.
func TestFlatTUI_SupersededTicketIgnoresStaleLiveEntry(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-superseded-ticket.md", "Status: superseded\n\nFirst body.\n")
	writeFlatTicket(t, root, "my-epic", "02-running-ticket.md", "Status: open\n\nSecond body.\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Superseded ticket")

	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventTicketStillNeedsAttention, Identifier: "01",
	}
	// A second, unrelated event lands after the first in the single-threaded
	// update loop, so waiting for its confirmatory text also guarantees the
	// first event has already been folded into m.live.
	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "02", Label: "iter-02"}
	waitForFlatText(t, tm, "iter-02")

	frame := tm.CurrentFrame()
	var rowLine []byte
	for line := range bytes.SplitSeq(frame, []byte("\n")) {
		if bytes.Contains(line, []byte("Superseded ticket")) {
			rowLine = line
			break
		}
	}
	if rowLine == nil {
		t.Fatalf("expected to find the superseded ticket's row, got:\n%s", frame)
	}
	if bytes.Contains(rowLine, []byte("reattach")) {
		t.Fatalf("expected the superseded ticket's row to ignore its stale needs-attention entry, got row:\n%s", rowLine)
	}
}

// TestFlatTUI_LiveEventsDrivePhaseSuffix feeds CherryPickStarted/
// ConflictResolutionStarted LiveEvents through WithLiveEvents and asserts
// ticket 01's running row shows a phase-specific suffix instead of just the
// tab label.
func TestFlatTUI_LiveEventsDrivePhaseSuffix(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirst body.\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Running ticket")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "(implementing...)")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventCherryPickStarted, Identifier: "01"}
	waitForFlatText(t, tm, "(cherry-picking...)")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventConflictResolutionStarted, Identifier: "01"}
	waitForFlatText(t, tm, "(resolving conflicts...)")

	frame := tm.CurrentFrame()
	if !bytes.Contains(frame, []byte("iter-01")) {
		t.Fatalf("expected the row to still show its tab label alongside the phase suffix, got:\n%s", frame)
	}
}

// TestFlatTUI_SmartZoneRecoveryDrivesPhaseSuffixNotPausedBanner covers the
// fix for a smart-zone breach: it must render as a phase suffix on the still-
// running row ("(compacting...)" / "(telling the agent to finish up...)"),
// never as a paused badge or the "Ralph loop paused" banner, since the
// scheduler is never actually blocked for this recovery path — and once
// SmartZoneRecovered arrives, the suffix must revert to "(implementing...)"
// rather than sticking on the last phase it saw.
func TestFlatTUI_SmartZoneRecoveryDrivesPhaseSuffixNotPausedBanner(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirst body.\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Running ticket")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "(implementing...)")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventSmartZoneCompactStarted, Identifier: "01"}
	waitForFlatText(t, tm, "(compacting...)")
	if bytes.Contains(tm.CurrentFrame(), []byte("Ralph loop paused")) {
		t.Fatalf("expected no paused banner during smart-zone compaction, got:\n%s", tm.CurrentFrame())
	}

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventSmartZoneFinishingUp, Identifier: "01"}
	// The row column is narrower than the full suffix, so only its lead-in
	// renders — this still proves the phase changed, without depending on the
	// terminal's exact truncation point.
	waitForFlatText(t, tm, "(telling the agent to finish up")
	if bytes.Contains(tm.CurrentFrame(), []byte("Ralph loop paused")) {
		t.Fatalf("expected no paused banner while telling the agent to finish up, got:\n%s", tm.CurrentFrame())
	}

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventSmartZoneRecovered, Identifier: "01"}
	waitForFlatText(t, tm, "(implementing...)")
}

// TestFlatTUI_LivePreviewMetadataAndTranscript feeds synthetic transcript-line
// events (ticket 01's EventSink.TranscriptLine) through WithLiveEvents and
// asserts ticket 04b's preview-pane additions: a running ticket's preview
// gains a metadata line (herdr tab id) and a live-updating transcript tail,
// while a done ticket's preview stays ticket 03's unchanged shape.
func TestFlatTUI_LivePreviewMetadataAndTranscript(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirstbodymarker\n")
	writeFlatTicket(t, root, "my-epic", "02-done-ticket.md", "Status: done\n\nSecondbodymarker\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Firstbodymarker")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventTranscriptLine, Label: "iter-01", Line: "transcriptlinemarker"}
	waitForFlatText(t, tm, "transcriptlinemarker")

	frame := tm.CurrentFrame()
	if !bytes.Contains(frame, []byte("tab iter-01")) {
		t.Fatalf("expected preview metadata line with herdr tab id, got:\n%s", frame)
	}
	if !bytes.Contains(frame, []byte("transcriptlinemarker")) {
		t.Fatalf("expected preview transcript tail to show the live line, got:\n%s", frame)
	}

	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Secondbodymarker")

	frame = tm.CurrentFrame()
	if bytes.Contains(frame, []byte("tab iter-01")) {
		t.Fatalf("expected done ticket's preview to have no metadata line, got:\n%s", frame)
	}
	if bytes.Contains(frame, []byte("transcriptlinemarker")) {
		t.Fatalf("expected done ticket's preview to have no transcript tail, got:\n%s", frame)
	}
}

// TestFlatTUI_EnterJumpsToLiveTabOrNoOps covers ticket 05's `enter` binding:
// it calls herdr.TabFocus with the live ticket's tab label and never
// touches focus/preview, and it's a silent no-op for a ticket with no live
// tab (here, a done ticket that never got a LiveEvent).
func TestFlatTUI_EnterJumpsToLiveTabOrNoOps(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirstbodymarker\n")
	writeFlatTicket(t, root, "my-epic", "02-done-ticket.md", "Status: done\n\nSecondbodymarker\n")

	var mu sync.Mutex
	var focusedTabIDs []string
	fakeTabFocus := func(tabID string) (herdr.Tab, error) {
		mu.Lock()
		defer mu.Unlock()
		focusedTabIDs = append(focusedTabIDs, tabID)
		return herdr.Tab{TabID: tabID, Focused: true}, nil
	}
	getFocusedTabIDs := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), focusedTabIDs...)
	}

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).
		WithLiveEvents(events).
		WithTabFocus(fakeTabFocus)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Running ticket")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")

	// Select the done ticket (no live tab): enter must no-op, never calling
	// TabFocus and never switching focus to the preview pane.
	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Secondbodymarker")
	tm.Send(flatKeySpecial(tea.KeyEnter))

	// If enter had switched focus to the preview pane, "k" would scroll the
	// viewport instead of moving the list selection, and the preview would
	// still show the done ticket's body. It shows the running ticket's body
	// instead, proving list nav (and thus list focus) survived enter.
	tm.Send(flatKeyRune('k'))
	waitForFlatText(t, tm, "Firstbodymarker")

	if got := getFocusedTabIDs(); len(got) != 0 {
		t.Fatalf("focusedTabIDs = %v, want none after no-op enter", got)
	}

	// Now on the running ticket: enter jumps to its live tab.
	tm.Send(flatKeySpecial(tea.KeyEnter))

	teatest.WaitFor(t, tm.Output(), func([]byte) bool {
		return len(getFocusedTabIDs()) > 0
	}, teatest.WithDuration(flatWait))

	if got := getFocusedTabIDs(); len(got) != 1 || got[0] != "iter-01" {
		t.Fatalf("focusedTabIDs = %v, want [iter-01]", got)
	}
}

// TestFlatTUI_EnterSurfacesTabFocusError covers the tab_not_found case: a
// live ticket's tab closed between the row rendering and enter being
// pressed, and the error is toasted rather than crashing the TUI.
func TestFlatTUI_EnterSurfacesTabFocusError(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-running-ticket.md", "Status: open\n\nFirst body.\n")

	fakeTabFocus := func(tabID string) (herdr.Tab, error) {
		return herdr.Tab{}, errors.New("tab_not_found")
	}

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).
		WithLiveEvents(events).
		WithTabFocus(fakeTabFocus)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Running ticket")
	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")

	tm.Send(flatKeySpecial(tea.KeyEnter))
	waitForFlatText(t, tm, "tab_not_found")
}

// TestFlatTUI_PausedAndAttentionBanner_AppearsAndDisappears covers ticket
// 06a's banner: it appears above the plain "? help" footer with
// state-specific copy for a smart-zone/rate-limit pause and for a
// needs-attention pause, and disappears once its ticket's IterationResumed
// event arrives (the same event waitForAttentionRecovery's automatic pane
// recovery emits, with no operator action).
func TestFlatTUI_PausedAndAttentionBanner_AppearsAndDisappears(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nFirst body.\n")
	writeFlatTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nSecond body.\n")

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).WithLiveEvents(events)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "? help")
	if bytes.Contains(tm.CurrentFrame(), []byte("Ralph loop")) {
		t.Fatalf("expected no banner before any pause, got:\n%s", tm.CurrentFrame())
	}

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")
	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseRateLimit, Reason: "context budget exceeded",
	}
	waitForFlatText(t, tm, "Ralph loop paused — context budget exceeded.")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationResumed, Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")
	if bytes.Contains(tm.CurrentFrame(), []byte("Ralph loop paused")) {
		t.Fatalf("expected the paused banner to disappear once iter-01 resumed, got:\n%s", tm.CurrentFrame())
	}

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "02", Label: "iter-02"}
	waitForFlatText(t, tm, "iter-02")
	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-02",
		PauseKind: ralphloop.PauseNeedsAttention, Reason: "Codex is waiting for operator intervention",
	}
	waitForFlatText(t, tm, "Ralph loop needs attention — Codex is waiting for operator intervention.")

	// The pane recovering on its own (no operator action, no modal) resumes
	// the iteration and clears the banner — mirroring waitForAttentionRecovery
	// noticing the pane left "blocked" and emitting IterationResumed itself.
	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationResumed, Label: "iter-02"}
	waitForFlatText(t, tm, "iter-02")
	if bytes.Contains(tm.CurrentFrame(), []byte("Ralph loop")) {
		t.Fatalf("expected the needs-attention banner to disappear once iter-02's pane recovered, got:\n%s", tm.CurrentFrame())
	}
}

// TestFlatTUI_ResumeConfirm_ConfirmAndCancel covers ticket 06b's `r` binding:
// on a paused row it opens a resume confirm whose `y`/enter calls the wired
// resume-control function with that ticket's live label; on a needs-attention
// row it opens a recheck confirm the same way. `n`/esc/q on either just
// closes the modal without calling anything.
func TestFlatTUI_ResumeConfirm_ConfirmAndCancel(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-paused-ticket.md", "Status: open\n\nFirst body.\n")
	writeFlatTicket(t, root, "my-epic", "02-attention-ticket.md", "Status: open\n\nSecond body.\n")

	var mu sync.Mutex
	var calledLabels []string
	fakeResumeControl := func(label string) bool {
		mu.Lock()
		defer mu.Unlock()
		calledLabels = append(calledLabels, label)
		return true
	}
	getCalledLabels := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calledLabels...)
	}

	events := make(chan ralphloop.LiveEvent, 16)
	m := tickets.NewFlatModel(root, "my-epic", ui.Settings{}).
		WithLiveEvents(events).
		WithResumeControl(fakeResumeControl)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(flatTermWidth, flatTermHeight))
	defer tm.Quit()

	waitForFlatText(t, tm, "Paused ticket")

	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"}
	waitForFlatText(t, tm, "iter-01")
	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseRateLimit, Reason: "context budget exceeded",
	}
	waitForFlatText(t, tm, "context budget exceeded")

	// `r` on the paused row (still selected) opens the resume confirm; `n`
	// cancels it without calling resumeControl.
	tm.Send(flatKeyRune('r'))
	waitForFlatText(t, tm, "Resume this ticket?")
	tm.Send(flatKeyRune('n'))
	waitForFlatText(t, tm, "context budget exceeded")
	if got := getCalledLabels(); len(got) != 0 {
		t.Fatalf("resumeControl calls after cancel = %v, want none", got)
	}

	// Confirming with `y` calls resumeControl with the paused ticket's label.
	tm.Send(flatKeyRune('r'))
	waitForFlatText(t, tm, "Resume this ticket?")
	tm.Send(flatKeyRune('y'))

	waitForFlatText(t, tm, "resumed")
	if got := getCalledLabels(); len(got) != 1 || got[0] != "iter-01" {
		t.Fatalf("resumeControl calls = %v, want [iter-01]", got)
	}

	// Select the needs-attention ticket: `r` opens a recheck confirm instead.
	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Attention ticket")
	events <- ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "02", Label: "iter-02"}
	waitForFlatText(t, tm, "iter-02")
	events <- ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-02",
		PauseKind: ralphloop.PauseNeedsAttention, Reason: "Codex is waiting for operator intervention",
	}
	waitForFlatText(t, tm, "Codex is waiting for operator intervention")

	tm.Send(flatKeyRune('r'))
	waitForFlatText(t, tm, "Re-check this ticket?")
	tm.Send(flatKeySpecial(tea.KeyEsc))
	waitForFlatText(t, tm, "Codex is waiting for operator intervention")
	if got := getCalledLabels(); len(got) != 1 {
		t.Fatalf("resumeControl calls after esc on recheck = %v, want still just [iter-01]", got)
	}

	tm.Send(flatKeyRune('r'))
	waitForFlatText(t, tm, "Re-check this ticket?")
	tm.Send(flatKeySpecial(tea.KeyEnter))

	waitForFlatText(t, tm, "rechecked")
	if got := getCalledLabels(); len(got) != 2 || got[1] != "iter-02" {
		t.Fatalf("resumeControl calls = %v, want [iter-01 iter-02]", got)
	}
}

func TestFlatTUI_EditChordCancels(t *testing.T) {
	root := t.TempDir()
	writeFlatTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeFlatTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	tm := startFlatTUI(t, root, "my-epic")
	defer tm.Quit()

	waitForFlatText(t, tm, "First ticket")

	tm.Send(flatKeyRune('e'))
	tm.Send(flatKeySpecial(tea.KeyEsc)) // cancels the "e"-prefix chord

	// The list must still respond to plain navigation after the cancel.
	tm.Send(flatKeyRune('j'))
	waitForFlatText(t, tm, "Second ticket")
}
