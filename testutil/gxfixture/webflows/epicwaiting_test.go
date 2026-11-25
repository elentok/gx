package webflows

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/epicwaiting"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
)

// epicRows is `gx server epics --json` by epic name.
func (r *running) epicRows(t *testing.T) map[string]wire.EpicListRow {
	t.Helper()
	var list wire.EpicList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	rows := map[string]wire.EpicListRow{}
	for _, row := range list.Epics {
		rows[row.Name] = row
	}
	return rows
}

// waitRows polls the epic list until ok holds, then returns it.
func (r *running) waitRows(t *testing.T, ok func(map[string]wire.EpicListRow) bool) map[string]wire.EpicListRow {
	t.Helper()
	var rows map[string]wire.EpicListRow
	if !testutil.Poll(60*time.Second, 200*time.Millisecond, func() bool {
		rows = r.epicRows(t)
		return ok(rows)
	}) {
		t.Fatalf("epic list never reached the wanted state: %+v", rows)
	}
	return rows
}

func blockerNames(row wire.EpicListRow) []string {
	var names []string
	for _, b := range row.Blockers {
		names = append(names, b.Name)
	}
	slices.Sort(names)
	return names
}

func TestEpicWaitingScenario_UnknownShapeListsTheKnownOnes(t *testing.T) {
	t.Parallel()
	out, err := exec.Command(fixturetest.FixtureBin, "--gx", fixturetest.GxBin, "--fakeclaude", fixturetest.FakeBin, "--shape", "nope", "epic-waiting").CombinedOutput()
	if err == nil {
		t.Fatalf("--shape nope started:\n%s", out)
	}
	for _, shape := range epicwaiting.Shapes {
		if !strings.Contains(string(out), shape) {
			t.Errorf("error does not list %q:\n%s", shape, out)
		}
	}
}

func TestEpicWaitingScenario_TwoBlockers(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "two-blockers")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[epicwaiting.Deliver].Section == wire.SectionImplementing && rows[epicwaiting.Push].Section == wire.SectionImplementing
	})
	for name, section := range map[string]wire.CardSection{
		epicwaiting.Queued: wire.SectionImplementing,
		epicwaiting.Epic:   wire.SectionReadyToQueue,
	} {
		row := rows[name]
		if !row.WaitingOnEpic || row.Section != section {
			t.Errorf("%s: waiting %v in %q, want waiting in %q", name, row.WaitingOnEpic, row.Section, section)
		}
		if got, want := blockerNames(row), []string{epicwaiting.Deliver, epicwaiting.Push}; !slices.Equal(got, want) {
			t.Errorf("%s: blockers %v, want %v", name, got, want)
		}
	}
	for _, row := range rows {
		if row.Section == wire.SectionNeedsYou {
			t.Errorf("%s is in Needs you", row.Name)
		}
	}
	for _, name := range []string{epicwaiting.Epic, epicwaiting.Queued} {
		if r.runLogMentions(t, name, "deadlocked") {
			t.Errorf("run log of %s has a deadlocked event", name)
		}
	}
}

// runLogMentions says whether any file of the epic's folder in the ticket store holds word.
func (r *running) runLogMentions(t *testing.T, epic, word string) bool {
	t.Helper()
	var found bool
	dir := filepath.Join(r.Root(), "data", "gx", "tickets", "demo", epic)
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(path) != "ticket.md" && !strings.HasSuffix(path, ".md") {
			if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), word) {
				found = true
			}
		}
		return nil
	})
	return found
}

// oneBlocker checks the waiting epics hold only deliver-primitive-impl and returns its entry.
func oneBlocker(t *testing.T, rows map[string]wire.EpicListRow) wire.EpicBlocker {
	t.Helper()
	for _, name := range []string{epicwaiting.Epic, epicwaiting.Queued} {
		if got := blockerNames(rows[name]); !slices.Equal(got, []string{epicwaiting.Deliver}) {
			t.Fatalf("%s: blockers %v, want only %s", name, got, epicwaiting.Deliver)
		}
	}
	return rows[epicwaiting.Epic].Blockers[0]
}

func TestEpicWaitingScenario_BlockerNotQueued(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "blocker-not-queued")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[epicwaiting.Queued].Section == wire.SectionImplementing && rows[epicwaiting.Deliver].Name != ""
	})
	if b := oneBlocker(t, rows); b.State != wire.EpicListReady {
		t.Errorf("blocker state %q, want %q", b.State, wire.EpicListReady)
	}
	if got := rows[epicwaiting.Deliver]; got.Section != wire.SectionReadyToQueue || got.Queued {
		t.Errorf("deliver: %q queued %v, want Ready to queue", got.Section, got.Queued)
	}
}

func TestEpicWaitingScenario_BlockerPaused(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "blocker-paused")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[epicwaiting.Deliver].Section == wire.SectionNeedsYou
	})
	if b := oneBlocker(t, rows); b.State != wire.EpicListPaused {
		t.Errorf("blocker state %q, want %q", b.State, wire.EpicListPaused)
	}
	for _, name := range []string{epicwaiting.Epic, epicwaiting.Queued} {
		if rows[name].Section == wire.SectionNeedsYou {
			t.Errorf("%s is in Needs you", name)
		}
	}
}

func TestEpicWaitingScenario_PickerOpen(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "picker-open")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[epicwaiting.Deliver].Section == wire.SectionImplementing
	})
	for _, name := range []string{epicwaiting.Polish, epicwaiting.UI, epicwaiting.Push} {
		if got := rows[name]; got.Section != wire.SectionReadyToQueue || got.MergedAt.IsZero() == false {
			t.Errorf("%s: %q merged %v, want an open Ready to queue epic", name, got.Section, got.MergedAt)
		}
	}
	if got := rows[epicwaiting.Merged]; got.MergedAt.IsZero() {
		t.Errorf("%s is not merged", epicwaiting.Merged)
	}
	if _, ok := rows[epicwaiting.Archived]; ok {
		t.Errorf("archived epic %s is listed", epicwaiting.Archived)
	}
	var list wire.EpicList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, row := range list.Epics {
		count[row.Name]++
	}
	if count[epicwaiting.Epic] != 1 {
		t.Errorf("%s listed %d times, want once", epicwaiting.Epic, count[epicwaiting.Epic])
	}
}

func TestEpicWaitingScenario_CycleRefusal(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "cycle-refusal")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[epicwaiting.Cycle].Name != "" && rows[epicwaiting.Deliver].Section == wire.SectionImplementing
	})
	if got := blockerNames(rows[epicwaiting.Cycle]); !slices.Equal(got, []string{epicwaiting.Epic}) {
		t.Fatalf("%s: blockers %v, want %s", epicwaiting.Cycle, got, epicwaiting.Epic)
	}
	if rows[epicwaiting.Cycle].Queued {
		t.Errorf("%s is queued, want it unticked", epicwaiting.Cycle)
	}
	// Adding the cycle epic as a blocker of the alignment epic must be refused, with the file unchanged.
	file := filepath.Join(r.Root(), "data", "gx", "tickets", "demo", epicwaiting.Epic, "ticket.md")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fixturetest.GxBin, "tickets", "epic", "set", "demo:"+epicwaiting.Epic, "--blocked-by", epicwaiting.Deliver+","+epicwaiting.Cycle)
	cmd.Env = r.Env
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(strings.ToLower(string(out)), "cycle") {
		t.Errorf("adding %s was not refused with the cycle text (err %v):\n%s", epicwaiting.Cycle, err, out)
	}
	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("refused add changed the file:\n%s\n--- was ---\n%s", after, before)
	}
}

func TestEpicWaitingScenario_Release(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "release")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[epicwaiting.Queued].WaitingOnEpic
	})
	if rows[epicwaiting.Deliver].Queued {
		t.Fatalf("%s is queued, want it complete and idle", epicwaiting.Deliver)
	}
	r.Gx(t, "server", "epics", "mark-merged", "demo:"+epicwaiting.Deliver)
	r.WaitStatus(t, "demo:"+epicwaiting.Queued+"/03", "claimed")
	if rows := r.epicRows(t); rows[epicwaiting.Queued].WaitingOnEpic {
		t.Errorf("%s still waits after the blocker merged", epicwaiting.Queued)
	}
}
