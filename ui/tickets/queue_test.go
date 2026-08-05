package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"
)

func TestQueueModelRendersFlatDependencyOrderedEpicPlan(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-independent.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "04-independent.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-foundation.md"):  true,
		ticketPath(root, "alpha", "02-dependent.md"):   true,
		ticketPath(root, "alpha", "03-independent.md"): true,
		ticketPath(root, "alpha", "04-independent.md"): true,
		ticketPath(root, "beta", "01-other.md"):        true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	content := m.View().Content

	if strings.Contains(content, "parallel") || strings.Contains(content, "then") {
		t.Fatalf("expected a flat ticket-number-ordered list, not wave grouping:\n%s", content)
	}

	alpha := strings.Index(content, "alpha")
	foundation := strings.Index(content, "Foundation")
	dependent := strings.Index(content, "Dependent")
	independent3 := strings.Index(content, "03")
	independent4 := strings.Index(content, "04")
	beta := strings.Index(content, "beta")
	if alpha < 0 || foundation < alpha || dependent < foundation || independent3 < dependent || independent4 < independent3 || beta < independent4 {
		t.Fatalf("expected ticket-number order within alpha, then epic grouping into beta:\n%s", content)
	}
}

func TestQueueModelNeverShowsATicketRunnableWhenOutOfScopeBlockerIsUnmet(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	// Only the dependent ticket is checked — its blocker never got selected,
	// and it isn't done, so ralph-loop's own scheduler would never claim it
	// either (ralphloop.RunScope.Frontier resolves blockers epic-wide).
	checked := map[string]bool{
		ticketPath(root, "alpha", "02-dependent.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	content := m.View().Content

	if strings.Contains(content, "parallel") || strings.Contains(content, "then") {
		t.Fatalf("expected no runnable wave for a ticket whose out-of-scope blocker is unmet:\n%s", content)
	}
	if !strings.Contains(content, "Dependent") {
		t.Fatalf("expected the checked ticket to remain visible for toggling:\n%s", content)
	}
	if !strings.Contains(content, "no unblocked tickets") {
		t.Fatalf("expected an actionable plan error instead of a misleading wave:\n%s", content)
	}
}

func TestQueueModelSurfacesActionableErrorForDependencyCycle(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\nBlocked by: 02\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\nBlocked by: 01\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	content := m.View().Content

	if !strings.Contains(content, "no unblocked tickets") {
		t.Fatalf("expected an actionable cycle error:\n%s", content)
	}
	if !strings.Contains(content, "First") || !strings.Contains(content, "Second") {
		t.Fatalf("expected both cyclic tickets to remain visible for toggling:\n%s", content)
	}
}

func TestQueueModelBannerWhileRunningAggregatesCheckedEpics(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-done.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-running.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "beta", "01-running.md", "Status: claimed\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-done.md"):    true,
		ticketPath(root, "alpha", "02-running.md"): true,
		ticketPath(root, "beta", "01-running.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	m.executionStartedAt = now.Add(-time.Hour - 3*time.Minute)
	m.now = func() time.Time { return now }
	m.runningEpics = map[string]bool{"alpha": true, "beta": true}
	m.executionTickets = map[string]bool{"alpha/01": true, "alpha/02": true, "beta/01": true}
	m.liveContextTokens = map[string]int{"alpha/02": 7000, "beta/01": 5000}

	content := m.View().Content
	want := "status: implementing (1 of 3 done), elapsed: 1h03m, context windows: 12.0k tok"
	if !strings.Contains(content, want) {
		t.Fatalf("running banner missing %q:\n%s", want, content)
	}
}

func TestQueueModelBannerWhenCompletedAggregatesLandedTicketMetrics(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "beta", "01-third.md", "Status: claimed\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
		ticketPath(root, "beta", "01-third.md"):   true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	m.executionStartedAt = now.Add(-time.Hour - 3*time.Minute)
	m.now = func() time.Time { return now }
	m.runningEpics = map[string]bool{"beta": true}
	m.executionTickets = map[string]bool{"alpha/01": true, "alpha/02": true, "beta/01": true}

	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: task\nactual_context_window: 12000\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-second.md", "---\nid: \"02\"\nstatus: done\ntype: task\nactual_context_window: 7000\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "beta", "01-third.md", "---\nid: \"01\"\nstatus: done\ntype: task\nactual_context_window: 5000\n---\n\nBody.\n")

	updated, cmd := m.Update(implementPollMsg{epicName: "beta"})
	m = deliverQueueCommands(t, updated.(QueueModel), cmd)

	content := m.View().Content
	want := "status: done, took 1h03m, context windows: total 24.0k tok, avg 8.0k tok, max 12.0k tok"
	if !strings.Contains(content, want) {
		done, total := m.completedExecutionProgress()
		t.Fatalf("completion banner missing %q (completed=%v, progress=%d/%d):\n%s", want, m.executionCompletedAt, done, total, content)
	}
}

// TestQueueModelRowsRenderWithNoCheckbox covers ticket 08: the Queue tab is
// read-only for selection, so its rows must not render a checkbox glyph
// (checking/selecting only happens in the Tickets tab).
func TestQueueModelRowsRenderWithNoCheckbox(t *testing.T) {
	root := t.TempDir()
	name := "01-first.md"
	writeTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", name): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	content := m.View().Content
	icons := ui.Icons(false)
	if strings.Contains(content, icons.CheckboxChecked) || strings.Contains(content, icons.CheckboxUnchecked) {
		t.Fatalf("expected no checkbox glyph in Queue tab rows:\n%s", content)
	}
}

// TestQueueModelClearAllRequiresConfirmation covers ticket 08's "C" keymap:
// pressing it opens a confirmation, and only accepting it clears every
// queued ticket.
func TestQueueModelClearAllRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	alpha := ticketPath(root, "alpha", "01-first.md")
	beta := ticketPath(root, "beta", "01-first.md")
	checked := map[string]bool{alpha: true, beta: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	m = updated.(QueueModel)
	if !m.confirm.IsOpen {
		t.Fatal("expected \"C\" to open a confirmation before clearing")
	}
	if !checked[alpha] || !checked[beta] {
		t.Fatal("expected nothing cleared before the confirmation is accepted")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	if checked[alpha] || checked[beta] {
		t.Fatal("expected accepting the confirmation to clear every queued ticket")
	}
}

// TestQueueModelClearCompleteRequiresConfirmation covers ticket 08's "c"
// keymap: only done tickets (and epics left with nothing visible) are
// cleared, after confirmation.
func TestQueueModelClearCompleteRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-open.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-done.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-done.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "02-done.md", "---\nid: \"02\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	writeRawQueueTicket(t, root, "beta", "01-done.md", "---\nid: \"01\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	open := ticketPath(root, "alpha", "01-open.md")
	alphaDone := ticketPath(root, "alpha", "02-done.md")
	betaDone := ticketPath(root, "beta", "01-done.md")
	checked := map[string]bool{open: true, alphaDone: true, betaDone: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)
	if !m.confirm.IsOpen {
		t.Fatal("expected \"c\" to open a confirmation before clearing")
	}
	if !checked[alphaDone] || !checked[betaDone] {
		t.Fatal("expected nothing cleared before the confirmation is accepted")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	if checked[alphaDone] || checked[betaDone] {
		t.Fatal("expected accepting the confirmation to clear done tickets")
	}
	if !checked[open] {
		t.Fatal("expected non-done tickets to remain checked")
	}

	content := m.View().Content
	if strings.Contains(content, "beta") {
		t.Fatalf("expected beta epic to disappear once its only ticket cleared:\n%s", content)
	}
}

// TestQueueModelHideCompleteToggleHidesDoneTicketsButKeepsPlanValidation
// covers ticket 09: "tc" hides StatusDone rows from rows()/View() without
// affecting epicWaves' plan validation (which must keep treating the hidden
// ticket as queued), and toggling "tc" again restores it.
func TestQueueModelHideCompleteToggleHidesDoneTicketsButKeepsPlanValidation(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "01-first.md", "---\nid: \"01\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	first := ticketPath(root, "alpha", "01-first.md")
	second := ticketPath(root, "alpha", "02-second.md")
	checked := map[string]bool{first: true, second: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	if content := m.View().Content; !strings.Contains(content, "First") {
		t.Fatalf("expected done ticket visible by default:\n%s", content)
	}
	if len(m.rows()) != 2 {
		t.Fatalf("expected both tickets in rows() by default, got %d", len(m.rows()))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)

	rows := m.rows()
	if len(rows) != 1 || rows[0].ticket.Path != second {
		t.Fatalf("expected only the non-done ticket in rows() after tc, got %+v", rows)
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
	if len(m.rows()) != 2 {
		t.Fatalf("expected tc toggled again to restore both tickets, got %d", len(m.rows()))
	}
}

// TestQueueModelTChordDoesNotCollideWithClearKeymaps covers ticket 09: the
// "t"-prefix chord swallows its second key without triggering "c"'s clear
// behavior, and plain "c"/"C" (with no preceding "t") still open their clear
// confirmations unaffected.
func TestQueueModelTChordDoesNotCollideWithClearKeymaps(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-done.md", "Status: open\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "01-done.md", "---\nid: \"01\"\nstatus: done\ntype: task\n---\n\nBody.\n")
	done := ticketPath(root, "alpha", "01-done.md")
	checked := map[string]bool{done: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(QueueModel)
	if m.hideComplete {
		t.Fatal("expected \"t\" followed by an unrelated key not to toggle hideComplete")
	}
	if m.confirm.IsOpen {
		t.Fatal("expected \"t\",\"q\" not to open any confirmation")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(QueueModel)
	if !m.confirm.IsOpen {
		t.Fatal("expected plain \"c\" (no preceding \"t\") to still open the clear-complete confirmation")
	}
	if m.hideComplete {
		t.Fatal("expected plain \"c\" not to toggle hideComplete")
	}
}

func TestQueueModelIncludesSelectionsAddedAfterLoad(t *testing.T) {
	root := t.TempDir()
	name := "01-later.md"
	writeTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", name)
	checked := map[string]bool{}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	checked[path] = true
	if content := m.View().Content; !strings.Contains(content, "Later") {
		t.Fatalf("expected cached Queue model to include a later shared selection:\n%s", content)
	}
}

func TestQueueModelEnterChoosesAgentAndStartsOneEpicSubset(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-unchecked.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
		ticketPath(root, "beta", "01-other.md"):   true,
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	runOptions := make(chan ralphloop.RunOptions, 1)
	releaseRun := make(chan struct{})
	runReturned := make(chan struct{})
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		runOptions <- opts
		<-releaseRun
		close(runReturned)
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		close(releaseRun)
		select {
		case <-runReturned:
		case <-time.After(time.Second):
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	if content := m.View().Content; !strings.Contains(content, "Choose the agent") {
		t.Fatalf("expected Enter to open the agent picker:\n%s", content)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	if cmd == nil {
		t.Fatal("expected choosing Codex to start execution")
	}
	updated, _ = m.Update(cmd())
	m = updated.(QueueModel)
	if !m.runningEpics["alpha"] {
		t.Fatalf("expected alpha running after kickoff, got %v", m.runningEpics)
	}

	select {
	case opts := <-runOptions:
		if opts.EpicName != "alpha" || opts.Agent != ralphloop.AgentCodex {
			t.Fatalf("unexpected run target: epic=%q agent=%q", opts.EpicName, opts.Agent)
		}
		if strings.Join(opts.TicketIDs, ",") != "01,02" {
			t.Fatalf("expected checked alpha subset [01 02], got %v", opts.TicketIDs)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ralph-loop kickoff")
	}
}

func TestQueueModelSchedulesCheckedEpicsInCheckOrderAndBackfillsAtCap(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "gamma", "01-first.md", "Status: open\n\nBody.\n")
	alpha := ticketPath(root, "alpha", "01-first.md")
	beta := ticketPath(root, "beta", "01-first.md")
	gamma := ticketPath(root, "gamma", "01-first.md")
	checked := map[string]bool{alpha: true, beta: true, gamma: true}
	checkOrder := map[string]uint64{gamma: 1, alpha: 2, beta: 3}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	starts := make(chan string, 3)
	releases := map[string]chan struct{}{
		"alpha": make(chan struct{}),
		"beta":  make(chan struct{}),
		"gamma": make(chan struct{}),
	}
	var mu sync.Mutex
	active, maxActive := 0, 0
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		mu.Lock()
		active++
		maxActive = max(maxActive, active)
		mu.Unlock()
		starts <- opts.EpicName
		<-releases[opts.EpicName]
		mu.Lock()
		active--
		mu.Unlock()
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(2)
	t.Cleanup(func() {
		for _, release := range releases {
			select {
			case <-release:
			default:
				close(release)
			}
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, checkOrder))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if first, second := <-starts, <-starts; first != "gamma" || second != "alpha" {
		t.Fatalf("start order = [%s %s], want [gamma alpha]", first, second)
	}
	select {
	case name := <-starts:
		t.Fatalf("epic %q started before a slot was free", name)
	default:
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(QueueModel)
	close(releases["gamma"])
	waitForEpicToFinish(t, "gamma")
	updated, cmd = m.Update(implementPollMsg{epicName: "gamma"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	select {
	case name := <-starts:
		t.Fatalf("epic %q backfilled while the queue was paused", name)
	default:
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)
	select {
	case name := <-starts:
		if name != "beta" {
			t.Fatalf("backfill epic = %q, want beta", name)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for beta to backfill gamma's slot")
	}
	mu.Lock()
	gotMaxActive := maxActive
	mu.Unlock()
	if gotMaxActive != 2 {
		t.Fatalf("maximum concurrent epics = %d, want 2", gotMaxActive)
	}
}

func TestQueueModelReactivationRecoversTwoConcurrentEpicsFromRegistry(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: claimed\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"): true,
		ticketPath(root, "beta", "01-first.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	r := newLoopRegistry(2)
	r.tryStart("alpha", 0, 1)
	r.tryStart("beta", 0, 1)
	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01", Identifier: "01"})
	r.reduceLiveEvent("beta", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01", Identifier: "01"})
	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 4000})
	r.reduceLiveEvent("beta", ralphloop.LiveEvent{Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 6000})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		r.finish("beta", nil)
		ralphLoopRegistry = previous
	})

	// This model never received implementStartedMsg for either epic — as if
	// both runs were launched while the Queue tab was backgrounded elsewhere.
	// OnPageActivated must recover both from the registry's snapshots alone.
	updated, _ := m.Update(m.OnPageActivated()())
	m = updated.(QueueModel)

	if !m.runningEpics["alpha"] || !m.runningEpics["beta"] {
		t.Fatalf("expected both epics reflected as running from registry snapshots, got %v", m.runningEpics)
	}
	content := m.View().Content
	if !strings.Contains(content, "implementing (0 of 2 done)") {
		t.Fatalf("expected aggregated progress across both concurrent epics:\n%s", content)
	}
	if !strings.Contains(content, "10.0k tok") {
		t.Fatalf("expected context tokens aggregated across both concurrent epics:\n%s", content)
	}
}

func TestQueueModelReactivationBackfillsPendingEpicAfterMissedCompletion(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	alpha := ticketPath(root, "alpha", "01-first.md")
	beta := ticketPath(root, "beta", "01-first.md")
	checked := map[string]bool{alpha: true, beta: true}
	checkOrder := map[string]uint64{alpha: 1, beta: 2}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	starts := make(chan string, 2)
	releases := map[string]chan struct{}{
		"alpha": make(chan struct{}),
		"beta":  make(chan struct{}),
	}
	runRalphLoop = func(opts ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		starts <- opts.EpicName
		<-releases[opts.EpicName]
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1) // one slot: beta must wait for alpha
	t.Cleanup(func() {
		for _, release := range releases {
			select {
			case <-release:
			default:
				close(release)
			}
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, checkOrder))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if got := <-starts; got != "alpha" {
		t.Fatalf("expected alpha to start first, got %q", got)
	}

	// Simulate the tab being backgrounded from here on: alpha finishes but no
	// implementPollMsg is ever delivered to this model instance (the app
	// shell would drop it since another tab was active).
	close(releases["alpha"])
	waitForEpicToFinish(t, "alpha")

	select {
	case name := <-starts:
		t.Fatalf("epic %q backfilled before reactivation", name)
	default:
	}

	updated, syncCmd := m.Update(m.OnPageActivated()())
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, syncCmd)

	select {
	case name := <-starts:
		if name != "beta" {
			t.Fatalf("backfill epic = %q, want beta", name)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for beta to backfill alpha's slot after reactivation")
	}
	close(releases["beta"])
}

func deliverQueueCommands(t *testing.T, m QueueModel, cmd tea.Cmd) QueueModel {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			m = deliverQueueCommands(t, m, nested)
		}
		return m
	}
	updated, _ := m.Update(msg)
	return updated.(QueueModel)
}

func waitForEpicToFinish(t *testing.T, epicName string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !ralphLoopRegistry.isRunningEpic(epicName) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for epic %q to finish", epicName)
}

func loadQueueModel(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	msg := m.Init()()
	updated, _ := m.Update(msg)
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	return updated.(QueueModel)
}

func TestQueueModelPersistsRunningThenDoneStatusThroughStore(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(path); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	started := make(chan ralphloop.EventSink, 1)
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, sink ralphloop.EventSink) error {
		started <- sink
		<-release
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	sink := <-started
	if got := store.Snapshot().Status[path]; got != queueStatusRunning {
		t.Fatalf("status after start = %v, want running", got)
	}

	ticket := m.epics[0].Tickets[0]
	sink.IterationStarted(ticket.Identifier, "iter-01", "", "")
	sink.IterationFinished(ticket, "alpha", ralphloop.IterationStats{})
	close(release)
	waitForEpicToFinish(t, "alpha")

	updated, cmd = m.Update(implementPollMsg{epicName: "alpha"})
	m = deliverQueueCommands(t, updated.(QueueModel), cmd)

	if got := store.Snapshot().Status[path]; got != queueStatusDone {
		t.Fatalf("status after completion = %v, want done", got)
	}
}

func TestQueueModelPersistsErroredStatusOnFailure(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(path); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		<-release
		return errors.New("boom")
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if got := store.Snapshot().Status[path]; got != queueStatusRunning {
		t.Fatalf("status after start = %v, want running", got)
	}

	close(release)
	waitForEpicToFinish(t, "alpha")

	updated, cmd = m.Update(implementPollMsg{epicName: "alpha"})
	m = deliverQueueCommands(t, updated.(QueueModel), cmd)

	if got := store.Snapshot().Status[path]; got != queueStatusErrored {
		t.Fatalf("status after failure = %v, want errored", got)
	}
}

func TestQueueModelPauseDoesNotRewriteRunningStatus(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(path); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		<-release
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(QueueModel)
	if !strings.Contains(m.View().Content, "Queue paused") {
		t.Fatalf("expected paused banner:\n%s", m.View().Content)
	}
	if got := store.Snapshot().Status[path]; got != queueStatusRunning {
		t.Fatalf("status while paused = %v, want running (unchanged)", got)
	}

	close(release)
}

func TestQueueModelMidRunSelectionChangeDoesNotRewriteProgressTotals(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-later.md", "Status: open\n\nBody.\n")
	alpha1 := ticketPath(root, "alpha", "01-first.md")
	alpha2 := ticketPath(root, "alpha", "02-second.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.SetChecked([]string{alpha1, alpha2}, true); err != nil {
		t.Fatal(err)
	}

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, _ ralphloop.EventSink) error {
		<-release
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		deadline := time.Now().Add(time.Second)
		for ralphLoopRegistry.isRunning() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, store))
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if done, total := m.checkedProgress(); total != 2 {
		t.Fatalf("progress before selection change = %d/%d, want total 2", done, total)
	}

	beta1 := ticketPath(root, "beta", "01-later.md")
	if err := store.Check(beta1); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(QueueModel)

	if done, total := m.checkedProgress(); total != 2 {
		t.Fatalf("progress after adding a future selection = %d/%d, want total still 2", done, total)
	}

	close(release)
}

// TestQueueModelRestoresAllStatusesAsInitialTab simulates a restart landing
// directly on Queue (ticket 21): a QueueStore pre-populated with every status
// (as a prior process session would have left it) must be fully reflected in
// a freshly constructed QueueModel with no prior Tickets-tab visit.
func TestQueueModelRestoresAllStatusesAsInitialTab(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-pending.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-running.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-done.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "04-errored.md", "Status: open\n\nBody.\n")
	pending := ticketPath(root, "alpha", "01-pending.md")
	running := ticketPath(root, "alpha", "02-running.md")
	done := ticketPath(root, "alpha", "03-done.md")
	errored := ticketPath(root, "alpha", "04-errored.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	for path, status := range map[string]queueItemStatus{
		pending: queueStatusPending,
		running: queueStatusRunning,
		done:    queueStatusDone,
		errored: queueStatusErrored,
	} {
		if err := store.Check(path); err != nil {
			t.Fatal(err)
		}
		if err := store.SetStatus(path, status); err != nil {
			t.Fatal(err)
		}
	}

	m := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, store))

	for path, want := range map[string]queueItemStatus{
		pending: queueStatusPending,
		running: queueStatusRunning,
		done:    queueStatusDone,
		errored: queueStatusErrored,
	} {
		if !m.checked[path] {
			t.Fatalf("expected %s checked after restart", path)
		}
		if got := m.queueStatus[path]; got != want {
			t.Fatalf("status for %s = %v, want %v", path, got, want)
		}
	}
}

// TestTicketsAndQueueMatchAfterRestartRegardlessOfNavigationOrder covers the
// "identical state in either navigation order" acceptance criterion: both
// tabs read the same QueueStore, so a Tickets-first-then-Queue construction
// and a Queue-first-then-Tickets construction must agree.
func TestTicketsAndQueueMatchAfterRestartRegardlessOfNavigationOrder(t *testing.T) {
	root := testutil.TempRepo(t)
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", "01-first.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(path); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(path, queueStatusDone); err != nil {
		t.Fatal(err)
	}

	queueFirst := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, store))
	ticketsModel := NewModelWithScopeAndStore(root, ui.Settings{}, keys.New(nil), false, store)
	ticketsModel = deliverLoad(t, ticketsModel)

	if queueFirst.checked[path] != ticketsModel.isChecked(path) {
		t.Fatalf("checked mismatch: queue=%v tickets=%v", queueFirst.checked[path], ticketsModel.isChecked(path))
	}
	if queueFirst.queueStatus[path] != ticketsModel.queueStatus[path] {
		t.Fatalf("status mismatch: queue=%v tickets=%v", queueFirst.queueStatus[path], ticketsModel.queueStatus[path])
	}
}

// TestQueueModelShowsSameTwoLineStatusAsTicketsTab covers ticket 25's first
// request: the Queue tab must show each ticket's status icon, blocked-by
// suffix, and (for a landed ticket) the same elapsed/tokens metrics line the
// Tickets tab renders (view.go's renderTicketRow), not just a bare checkbox
// and title.
func TestQueueModelShowsSameTwoLineStatusAsTicketsTab(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	writeRawQueueTicket(t, root, "alpha", "03-done.md", "---\nid: \"03\"\nstatus: done\ntype: task\nactual_context_window: 12000\nelapsed_time: 754\n---\n\nDone.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-foundation.md"): true,
		ticketPath(root, "alpha", "02-dependent.md"):  true,
		ticketPath(root, "alpha", "03-done.md"):       true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	content := m.View().Content

	if !strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected blocked-by suffix for the dependent ticket:\n%s", content)
	}
	if !strings.Contains(content, "12.0k tok") || !strings.Contains(content, "12m34s") {
		t.Fatalf("expected the done ticket's elapsed/tokens metrics line:\n%s", content)
	}
}

func TestRenderQueueTicketRow_CommitlessSuffix(t *testing.T) {
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Open ticket", Status: "open", Commitless: true},
	}}

	lines := m.renderQueueTicketRow(epic, epic.Tickets[0], 0)
	if !strings.Contains(lines[0], "Open ticket (commitless)") {
		t.Fatalf("title line = %q, want title followed by \" (commitless)\"", lines[0])
	}
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " "))]
}

// TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow covers ticket 10:
// renderLiveTicketRow already prefixes its output with 2 spaces, so the
// caller must not add its own on top or a running row ends up indented 4
// spaces instead of 2.
func TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow(t *testing.T) {
	var m QueueModel
	m.runningEpics = map[string]bool{"epic": true}
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Normal ticket", Status: "open"},
		{Identifier: "02", Title: "Running ticket", Status: "claimed"},
	}}
	m.live = map[string]map[string]liveTicketState{
		"epic": {"02": {running: true, label: "iter-01"}},
	}

	normalLine := m.renderQueueTicketRow(epic, epic.Tickets[0], 0)[0]
	liveLine := m.renderQueueTicketRow(epic, epic.Tickets[1], 1)[0]

	normalIndent := leadingWhitespace(ansi.Strip(normalLine))
	liveIndent := leadingWhitespace(ansi.Strip(liveLine))
	if liveIndent != normalIndent {
		t.Fatalf("live row indent = %q, want %q (matching normal row): live=%q normal=%q", liveIndent, normalIndent, liveLine, normalLine)
	}
}

func TestRenderQueueTicketRow_DoneMetricsLineMatchesTitleColor(t *testing.T) {
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
	}}

	lines := m.renderQueueTicketRow(epic, epic.Tickets[0], 0)
	wantMetrics := renderRowMetricsLine(formatMetricsLine(5, 100), statusDoneStyle)
	if lines[1] != wantMetrics {
		t.Fatalf("metrics line = %q, want %q", lines[1], wantMetrics)
	}
}

// TestQueueModelScrollsWithKeysAndMouse covers ticket 25's third request: the
// Queue tab must scroll (keyboard, including ctrl+d/ctrl+u, and mouse wheel)
// once its content overflows the visible viewport — previously it had no
// scroll offset at all, so content past the panel's height was simply
// unreachable.
func TestQueueModelScrollsWithKeysAndMouse(t *testing.T) {
	root := t.TempDir()
	for i := 1; i <= 40; i++ {
		writeTicket(t, root, "alpha", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}
	checked := map[string]bool{}
	for i := 1; i <= 40; i++ {
		checked[ticketPath(root, "alpha", fmt.Sprintf("%02d-ticket.md", i))] = true
	}

	m := NewQueueModel(root, ui.Settings{}, checked)
	msg := m.Init()()
	updated, _ := m.Update(msg)
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m = updated.(QueueModel)

	if m.scrollOffset != 0 {
		t.Fatalf("expected no initial scroll, got offset %d", m.scrollOffset)
	}

	// ctrl+d pages the selection (and viewport) down, twice so there's slack
	// for ctrl+u to give back — a single page-down can land the selection
	// exactly at the viewport's top line, which a lone page-up wouldn't need
	// to scroll further for (it'd already be visible).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(QueueModel)
	if m.scrollOffset == 0 {
		t.Fatalf("expected ctrl+d to scroll the viewport down, got offset %d", m.scrollOffset)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(QueueModel)
	afterPageDown := m.scrollOffset

	// ctrl+u pages back up — repeated past the top so the selection (and so
	// the viewport, via ensureQueueVisible) actually has to move rather than
	// staying put because the target row was already in view.
	for range 3 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		m = updated.(QueueModel)
	}
	if m.selected != 0 {
		t.Fatalf("expected ctrl+u to page the selection back to the top, got %d", m.selected)
	}
	if m.scrollOffset >= afterPageDown {
		t.Fatalf("expected ctrl+u to scroll the viewport back up from %d, got %d", afterPageDown, m.scrollOffset)
	}

	// Mouse wheel scrolls the viewport without needing a key press.
	before := m.scrollOffset
	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(QueueModel)
	if m.scrollOffset <= before {
		t.Fatalf("expected mouse wheel down to increase scroll offset from %d, got %d", before, m.scrollOffset)
	}
}

// TestQueueModelShowsPreviewPaneForSelectedTicket covers ticket 15's core
// acceptance criterion: the Queue tab, previously a single full-width list
// with no preview at all, now shows the same shared preview pane
// (renderTicketPreview) as the Tickets tab for whichever row is selected.
func TestQueueModelShowsPreviewPaneForSelectedTicket(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\nType: task\n\nDistinctive queue-preview body.\n")

	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "Preview") {
		t.Fatalf("expected a Preview panel title in the Queue tab, got:\n%s", content)
	}
	if !strings.Contains(content, "Distinctive queue-preview body.") {
		t.Fatalf("expected the selected ticket's body rendered in the preview pane, got:\n%s", content)
	}
	if !strings.Contains(content, "Status: open") {
		t.Fatalf("expected prettified frontmatter in the Queue tab's preview pane, got:\n%s", content)
	}
}

// TestQueueModelMouseClickSelectsRowOnly covers ticket 05c: clicking a row
// in the Queue tab must select it (like arrowing there) and do nothing else
// — no confirm modal, no other side effect.
func TestQueueModelMouseClickSelectsRowOnly(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "03-third.md", "Status: open\n\nBody.\n")

	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
		ticketPath(root, "alpha", "03-third.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	if m.selected != 0 {
		t.Fatalf("expected initial selection at row 0, got %d", m.selected)
	}

	_, offsets, _ := m.buildQueueLines()
	if len(offsets) != 3 {
		t.Fatalf("expected 3 row offsets, got %d: %v", len(offsets), offsets)
	}

	// Click on the third row (bodyLine = mouse.Y-1, no scroll offset).
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: offsets[2] + 1})
	m = updated.(QueueModel)
	if m.selected != 2 {
		t.Fatalf("expected click to select row 2, got %d", m.selected)
	}
	if m.confirm.IsOpen {
		t.Fatalf("expected click-to-select to not open the confirm modal")
	}

	// A non-left click must not move the selection.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseRight, Y: offsets[0] + 1})
	m = updated.(QueueModel)
	if m.selected != 2 {
		t.Fatalf("expected non-left click to leave selection at row 2, got %d", m.selected)
	}

	// A click above the body (row -1) must not crash or change selection.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, Y: 0})
	m = updated.(QueueModel)
	if m.selected != 2 {
		t.Fatalf("expected out-of-bounds click to leave selection at row 2, got %d", m.selected)
	}
}

func ticketPath(root, epic, name string) string {
	return filepath.Join(root, ".scratch", epic, "issues", name)
}

func writeRawQueueTicket(t *testing.T, root, epic, name, content string) {
	t.Helper()
	if err := os.WriteFile(ticketPath(root, epic, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestQueueSearch_SlashEntersInputMode(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)

	if m.search.Mode() != search.SearchModeInput {
		t.Fatalf("expected search input mode after '/', got mode=%v", m.search.Mode())
	}
}

func TestQueueSearch_TypedCharactersFilterAndHighlight(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	for _, r := range "/first" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(QueueModel)
	}

	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected exactly one match for %q, got %d", "first", m.search.MatchesCount())
	}

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]
	rows := m.rows()
	matchedLine := m.renderQueueTicketRow(rows[0].epic, rows[0].ticket, 0)[0]
	nonMatchedLine := m.renderQueueTicketRow(rows[1].epic, rows[1].ticket, 1)[0]
	if strings.Contains(matchedLine, dimPrefix) {
		t.Fatalf("expected matching row undimmed, got: %q", matchedLine)
	}
	if !strings.Contains(nonMatchedLine, dimPrefix) {
		t.Fatalf("expected non-matching row dimmed while searching, got: %q", nonMatchedLine)
	}
}

func TestQueueSearch_EscExitsSearchMode(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(QueueModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)

	if m.search.Mode() != search.SearchModeNone {
		t.Fatalf("expected esc to fully clear search mode, got %v", m.search.Mode())
	}
	if m.search.HasQuery() {
		t.Fatalf("expected esc to clear the query")
	}
}

func TestQueueSearch_EnterExitsInputButKeepsResults(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	for _, r := range "/first" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(QueueModel)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	if m.search.Mode() == search.SearchModeInput {
		t.Fatalf("expected enter to leave input mode")
	}
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected matches to persist after enter")
	}
}

// TestQueueSearch_DigitsTypeIntoQueryNotBoundKeys covers this Queue tab's
// digit-routing counterpart to the Tickets tab fix (ticket 14): while search
// input is active, digit keys must type into the query rather than falling
// through to any of handleQueueKey's own bindings.
func TestQueueSearch_DigitsTypeIntoQueryNotBoundKeys(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = updated.(QueueModel)

	if m.search.Query() != "1" {
		t.Fatalf("expected digit to type into the search query, got query=%q", m.search.Query())
	}
	if !m.InputFocused() {
		t.Fatalf("expected InputFocused()=true while search input is active")
	}
}
