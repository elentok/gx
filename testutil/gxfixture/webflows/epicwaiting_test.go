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
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
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
	for _, shape := range spec.EpicWaitingShapes {
		if !strings.Contains(string(out), shape) {
			t.Errorf("error does not list %q:\n%s", shape, out)
		}
	}
}

func TestEpicWaitingScenario_TwoBlockers(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "two-blockers")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[spec.EpicWaitingDeliver].Section == wire.SectionImplementing && rows[spec.EpicWaitingPush].Section == wire.SectionImplementing
	})
	for name, section := range map[string]wire.CardSection{
		spec.EpicWaitingQueued: wire.SectionImplementing,
		spec.EpicWaitingEpic:   wire.SectionReadyToQueue,
	} {
		row := rows[name]
		if !row.WaitingOnEpic || row.Section != section {
			t.Errorf("%s: waiting %v in %q, want waiting in %q", name, row.WaitingOnEpic, row.Section, section)
		}
		if got, want := blockerNames(row), []string{spec.EpicWaitingDeliver, spec.EpicWaitingPush}; !slices.Equal(got, want) {
			t.Errorf("%s: blockers %v, want %v", name, got, want)
		}
	}
	for _, row := range rows {
		if row.Section == wire.SectionNeedsYou {
			t.Errorf("%s is in Needs you", row.Name)
		}
	}
	for _, name := range []string{spec.EpicWaitingEpic, spec.EpicWaitingQueued} {
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
	for _, name := range []string{spec.EpicWaitingEpic, spec.EpicWaitingQueued} {
		if got := blockerNames(rows[name]); !slices.Equal(got, []string{spec.EpicWaitingDeliver}) {
			t.Fatalf("%s: blockers %v, want only %s", name, got, spec.EpicWaitingDeliver)
		}
	}
	return rows[spec.EpicWaitingEpic].Blockers[0]
}

func TestEpicWaitingScenario_BlockerNotQueued(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "blocker-not-queued")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[spec.EpicWaitingQueued].Section == wire.SectionImplementing && rows[spec.EpicWaitingDeliver].Name != ""
	})
	if b := oneBlocker(t, rows); b.State != wire.EpicListReady {
		t.Errorf("blocker state %q, want %q", b.State, wire.EpicListReady)
	}
	if got := rows[spec.EpicWaitingDeliver]; got.Section != wire.SectionReadyToQueue || got.Queued {
		t.Errorf("deliver: %q queued %v, want Ready to queue", got.Section, got.Queued)
	}
}

func TestEpicWaitingScenario_BlockerPaused(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "blocker-paused")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[spec.EpicWaitingDeliver].Section == wire.SectionNeedsYou
	})
	if b := oneBlocker(t, rows); b.State != wire.EpicListPaused {
		t.Errorf("blocker state %q, want %q", b.State, wire.EpicListPaused)
	}
	for _, name := range []string{spec.EpicWaitingEpic, spec.EpicWaitingQueued} {
		if rows[name].Section == wire.SectionNeedsYou {
			t.Errorf("%s is in Needs you", name)
		}
	}
}

func TestEpicWaitingScenario_PickerOpen(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "picker-open")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[spec.EpicWaitingDeliver].Section == wire.SectionImplementing
	})
	for _, name := range []string{spec.EpicWaitingPolish, spec.EpicWaitingUI, spec.EpicWaitingPush} {
		if got := rows[name]; got.Section != wire.SectionReadyToQueue || got.MergedAt.IsZero() == false {
			t.Errorf("%s: %q merged %v, want an open Ready to queue epic", name, got.Section, got.MergedAt)
		}
	}
	if got := rows[spec.EpicWaitingMerged]; got.MergedAt.IsZero() {
		t.Errorf("%s is not merged", spec.EpicWaitingMerged)
	}
	if _, ok := rows[spec.EpicWaitingArchived]; ok {
		t.Errorf("archived epic %s is listed", spec.EpicWaitingArchived)
	}
	var list wire.EpicList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, row := range list.Epics {
		count[row.Name]++
	}
	if count[spec.EpicWaitingEpic] != 1 {
		t.Errorf("%s listed %d times, want once", spec.EpicWaitingEpic, count[spec.EpicWaitingEpic])
	}
}

func TestEpicWaitingScenario_CycleRefusal(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-waiting", nil, "--shape", "cycle-refusal")
	rows := r.waitRows(t, func(rows map[string]wire.EpicListRow) bool {
		return rows[spec.EpicWaitingCycle].Name != "" && rows[spec.EpicWaitingDeliver].Section == wire.SectionImplementing
	})
	if got := blockerNames(rows[spec.EpicWaitingCycle]); !slices.Equal(got, []string{spec.EpicWaitingEpic}) {
		t.Fatalf("%s: blockers %v, want %s", spec.EpicWaitingCycle, got, spec.EpicWaitingEpic)
	}
	if rows[spec.EpicWaitingCycle].Queued {
		t.Errorf("%s is queued, want it unticked", spec.EpicWaitingCycle)
	}
	// Adding the cycle epic as a blocker of the alignment epic must be refused, with the file unchanged.
	file := filepath.Join(r.Root(), "data", "gx", "tickets", "demo", spec.EpicWaitingEpic, "ticket.md")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fixturetest.GxBin, "tickets", "epic", "set", "demo:"+spec.EpicWaitingEpic, "--blocked-by", spec.EpicWaitingDeliver+","+spec.EpicWaitingCycle)
	cmd.Env = r.Env
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(strings.ToLower(string(out)), "cycle") {
		t.Errorf("adding %s was not refused with the cycle text (err %v):\n%s", spec.EpicWaitingCycle, err, out)
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
		return rows[spec.EpicWaitingQueued].WaitingOnEpic
	})
	if rows[spec.EpicWaitingDeliver].Queued {
		t.Fatalf("%s is queued, want it complete and idle", spec.EpicWaitingDeliver)
	}
	r.Gx(t, "server", "epics", "mark-merged", "demo:"+spec.EpicWaitingDeliver)
	r.WaitStatus(t, "demo:"+spec.EpicWaitingQueued+"/03", "claimed")
	if rows := r.epicRows(t); rows[spec.EpicWaitingQueued].WaitingOnEpic {
		t.Errorf("%s still waits after the blocker merged", spec.EpicWaitingQueued)
	}
}
