package epics_test

import (
	"context"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"os"
	"path/filepath"
	"testing"
)

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const (
	epicMergedText    = rigs.EpicMergedText
	epicNotMergedText = rigs.EpicNotMergedText
	answerAddr        = servertest.AnswerAddr
	optionsBlock      = servertest.OptionsBlock
	runEndText        = servertest.RunEndText
	repairToken       = servertest.RepairToken
)

type (
	schedFixture    = servertest.SchedFixture
	designsFixture  = servertest.DesignsFixture
	grillingFixture = servertest.GrillingFixture
)

var (
	newSchedFixture         = servertest.NewSchedFixture
	startDesigns            = servertest.StartDesigns
	startDesignsIn          = servertest.StartDesignsIn
	startAdvancing          = servertest.StartAdvancing
	endTurn                 = servertest.EndTurn
	startBlockedClaimed     = rigs.StartBlockedClaimed
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	pauseWithQuestion       = servertest.PauseWithQuestion
	storeWithMapStep        = servertest.StoreWithMapStep
	commitWork              = servertest.CommitWork
	commitOnBranch          = servertest.CommitOnBranch
	writeCompleteEpic       = servertest.WriteCompleteEpic
	atMergeCap              = servertest.AtMergeCap
	runGitIn                = servertest.RunGitIn
	chatRecorder            = rigs.ChatRecorder
	waitForChat             = rigs.WaitForChat
	waitFor                 = servertest.Await
	waitUntil               = servertest.WaitUntil
	waitForSession          = servertest.WaitForSession
	report                  = servertest.Report
	readMeta                = servertest.ReadMeta
	writeAssistantMessage   = servertest.WriteAssistantMessage
)

func TestSnapshot_TicketsCarryRenderedStatus(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic", "01", "parent", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", "epic", "02", "fork-child", servertest.TicketOpts{Parent: "01"})
	servertest.WriteTicketWith(t, store, "proj", "epic", "03", "waiting", servertest.TicketOpts{BlockedBy: []string{"04"}})
	servertest.WriteTicketWith(t, store, "proj", "epic", "04", "blocker", servertest.TicketOpts{})
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ti := range snap.Tickets {
		got[ti.Address] = ti.RenderedStatus
	}
	want := map[string]string{
		"proj:epic/01": "waiting-for-children",
		"proj:epic/02": "open",
		"proj:epic/03": "blocked",
		"proj:epic/04": "open",
	}
	for addr, w := range want {
		if got[addr] != w {
			t.Errorf("%s rendered status = %q, want %q", addr, got[addr], w)
		}
	}
}

// startWithPath starts a server whose PATH is only dir.
func startWithPath(t *testing.T, dir string) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.Env = append(c.Env, "PATH="+dir)
	})
}

func TestSnapshot_FlagsNodeMissingFromThePath(t *testing.T) {
	t.Parallel()
	h := startWithPath(t, t.TempDir())
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.NodeMissing {
		t.Error("node_missing = false with no node on PATH, want true")
	}
}

func TestSnapshot_NodeOnThePathIsNotFlagged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := startWithPath(t, dir)
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.NodeMissing {
		t.Error("node_missing = true with node on PATH, want false")
	}
}
