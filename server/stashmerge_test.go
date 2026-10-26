package server_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// stashMergeFixture pauses the epic on a dirty trunk: the branch adds same.txt,
// and main's checkout holds a different untracked same.txt. late.txt is not
// dirty yet, so the pause does not name it.
func stashMergeFixture(t *testing.T) *schedFixture {
	t.Helper()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		// The trunk-conflict ticket a stash-merge opens may be picked up too; it has nothing to commit.
		if id != "01" {
			return
		}
		testutil.WriteFile(t, p.Cwd, "same.txt", "branch")
		testutil.WriteFile(t, p.Cwd, "late.txt", "branch")
		testutil.CommitAll(t, p.Cwd, "work")
		testutil.WriteFile(t, f.repo, "same.txt", "local")
	}, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	servertest.WriteTicketWith(t, f.store, "proj", "waiter", "01", "t", servertest.TicketOpts{})
	waiterMD := "---\nstatus: open\nblocked_by: [" + schedEpic + "]\n---\n"
	if err := os.WriteFile(filepath.Join(f.store, "proj", "waiter", "ticket.md"), []byte(waiterMD), 0o644); err != nil {
		t.Fatal(err)
	}
	f.pauseRoot(t)
	return f
}

func (f *schedFixture) webStashMerge(t *testing.T, state string) (int, string) {
	t.Helper()
	hs, err := f.h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	cookie := login(t, f.h.StateDir, hs.WebURL)
	body := `{"address":"proj:` + schedEpic + `","state":"` + state + `"}`
	return webPostJSON(t, hs.WebURL+"/v1/epics/stash-merge", body, cookie, "", jsonPost(hs.WebURL))
}

func TestStashMerge_WebRefusesAStaleTokenAndTouchesNothing(t *testing.T) {
	f := stashMergeFixture(t)
	main := f.git(t, "rev-parse", "main")

	code, body := f.webStashMerge(t, string(server.EpicStateRunning))

	if code != http.StatusOK || reasonOf(t, body) != server.ReasonStale {
		t.Errorf("stale state = %d %s, want the stale refusal", code, body)
	}
	if got := f.git(t, "rev-parse", "main"); got != main {
		t.Errorf("main moved to %s", got)
	}
	if got := f.read(t, "same.txt"); got != "local" {
		t.Errorf("same.txt = %q, want the local edit", got)
	}
	if refs := f.backupRefs(t); len(refs) != 0 {
		t.Errorf("backup refs = %v, want none", refs)
	}
}

// The Epics card sends the list state, which names a merge-blocked epic "merge-blocked"
// where the Epic page's EpicState says paused.
func TestStashMerge_AcceptsTheCardsMergeBlockedState(t *testing.T) {
	f := stashMergeFixture(t)

	code, body := f.webStashMerge(t, string(server.EpicListMergeBlocked))

	if code != http.StatusOK || reasonOf(t, body) != "" {
		t.Errorf("stash-merge from a card = %d %s, want it accepted", code, body)
	}
}

func TestStashMerge_LeavesMarkersUncommittedAndBacksUpAFileThatClashesAtClickTime(t *testing.T) {
	f := stashMergeFixture(t)
	// Dirty only now: the pause did not list it.
	testutil.WriteFile(t, f.repo, "late.txt", "local late")

	code, body := f.webStashMerge(t, string(server.EpicStatePaused))

	if code != http.StatusOK || reasonOf(t, body) != "" {
		t.Fatalf("stash-merge = %d %s", code, body)
	}
	if got, want := f.git(t, "rev-parse", "main"), f.git(t, "rev-parse", schedEpic); got != want {
		t.Errorf("main at %s, want the feature branch tip %s", got, want)
	}
	for _, name := range []string{"same.txt", "late.txt"} {
		if got := f.read(t, name); !strings.Contains(got, "<<<<<<<") {
			t.Errorf("%s = %q, want conflict markers", name, got)
		}
	}
	if got := f.git(t, "log", "--format=%s", "-1", "main"); strings.Contains(got, "gx backup") {
		t.Errorf("main's tip is the backup commit: %q", got)
	}
	refs := f.backupRefs(t)
	if len(refs) != 1 {
		t.Fatalf("backup refs = %v, want one", refs)
	}
	if got := f.git(t, "show", refs[0]+":late.txt"); got != "local late" {
		t.Errorf("backup late.txt = %q, want the local content", got)
	}
	if got := f.git(t, "show", refs[0]+":same.txt"); got != "local" {
		t.Errorf("backup same.txt = %q, want the local content", got)
	}

	// A top-level trunk-conflict ticket in the root epic names the ref and files.
	ticket := f.ticketFile(t, "02")
	md := readFile(t, ticket)
	// Not "status: open": the queue may have claimed the ticket by the time it is read.
	for _, want := range []string{"type: trunk-conflict", refs[0], "same.txt", "late.txt"} {
		if !strings.Contains(md, want) {
			t.Errorf("trunk-conflict ticket lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "parent:") {
		t.Errorf("trunk-conflict ticket has a parent:\n%s", md)
	}
	if !strings.Contains(body, `"trunk_conflict":"proj:`+schedEpic+`/02"`) {
		t.Errorf("result %s does not name the ticket", body)
	}
}

func TestStashMerge_RecordsTheBackupOnTheRootAndTheRunLogAndKeepsTheRootUnmerged(t *testing.T) {
	f := stashMergeFixture(t)

	res, err := f.h.Client.EpicStashMerge(context.Background(), "proj:"+schedEpic)
	if err != nil || res.Refused {
		t.Fatalf("stash-merge = %+v, %v", res, err)
	}

	refs := f.backupRefs(t)
	if len(refs) != 1 {
		t.Fatalf("backup refs = %v, want one", refs)
	}
	epic := tickets.LoadEpicTicketMD(filepath.Join(f.store, "proj", schedEpic))
	if epic.TrunkConflictBackup != refs[0] || !slices.Equal(epic.TrunkConflictFiles, []string{"same.txt"}) || epic.TrunkFastForwardedAt.IsZero() {
		t.Errorf("epic fields = %q %v %v", epic.TrunkConflictBackup, epic.TrunkConflictFiles, epic.TrunkFastForwardedAt)
	}
	if !epic.MergedAt.IsZero() || !epic.CompletedAt.IsZero() {
		t.Errorf("merged_at %v, completed_at %v, want both unset", epic.MergedAt, epic.CompletedAt)
	}
	if md := f.epicMD(t); strings.Contains(md, "land_block") {
		t.Errorf("the dirty-trunk pause is still stored:\n%s", md)
	}
	f.assertRootQueued(t)

	log, _, err := ralphloop.ReadEvents(filepath.Join(f.store, "proj"), schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(log, func(ev ralphloop.Event) bool { return events.Type(ev.Type) == events.TrunkConflictOpened })
	if i < 0 || log[i].BackupRef != refs[0] || log[i].Reason != "same.txt" {
		t.Errorf("run log = %+v, want a trunk-conflict-opened event naming the ref and files", log)
	}

	// The dependent epic still waits for the merge.
	list, err := f.h.Client.Epics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range list.Epics {
		if row.Name == "waiter" && !slices.Equal(row.BlockedByEpics, []string{"proj:" + schedEpic}) {
			t.Errorf("waiter blocked_by_epics = %v, want [proj:%s]", row.BlockedByEpics, schedEpic)
		}
	}
}

func TestStashMerge_RefusesAnEpicThatIsNotPausedOnADirtyTrunk(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, _ string) {
		commitFiles(t, p, "a.txt")
	})
	f.runEpic(t, server.EventRootCompleted)

	res, err := f.h.Client.EpicStashMerge(context.Background(), "proj:"+schedEpic)

	if err != nil || res.Reason != server.ReasonNotDirtyTrunk {
		t.Errorf("stash-merge = %+v, %v, want the not-dirty-trunk refusal", res, err)
	}
}
