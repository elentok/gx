package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
)

func TestQueueModelRendersDependencyAwareEpicPlan(t *testing.T) {
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

	alpha := strings.Index(content, "alpha")
	parallel := strings.Index(content, "parallel")
	then := strings.Index(content, "then")
	dependent := strings.Index(content, "Dependent")
	beta := strings.Index(content, "beta")
	if alpha < 0 || parallel < alpha || then < parallel || dependent < then || beta < dependent {
		t.Fatalf("expected parallel wave, sequential dependency, and epic grouping in order:\n%s", content)
	}
	if strings.Count(content, "parallel") != 2 {
		t.Fatalf("expected available capacity to produce two parallel clusters, got:\n%s", content)
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

func TestQueueModelSpaceTogglesSharedCheckedSet(t *testing.T) {
	root := t.TempDir()
	name := "01-first.md"
	writeTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	path := ticketPath(root, "alpha", name)
	checked := map[string]bool{path: true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

	updated, _ := m.Update(spacePress())
	m = updated.(QueueModel)
	if checked[path] {
		t.Fatal("expected Queue uncheck to update the shared checked set")
	}

	updated, _ = m.Update(spacePress())
	if !checked[path] {
		t.Fatal("expected Queue recheck to update the shared checked set")
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
		if running, name := ralphLoopRegistry.snapshot(epicName); !running || name != epicName {
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
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(QueueModel)
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
