package migrations

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

func ledgerRegistry() *Registry {
	r := &Registry{}
	r.Register(Step{Name: ledgerStepName, Append: true, Run: rebuildLedger})
	return r
}

func ledgerTurn(ts string, in, out int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":%d,"output_tokens":%d}}}`+"\n", ts, in, out)
}

func writeLedgerTranscript(t *testing.T, dirs config.Dirs, session string, turns ...string) {
	t.Helper()
	home, err := dirs.Home()
	if err != nil {
		t.Fatal(err)
	}
	path := transcript.PathIn(home, "/work/"+session, session)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(turns, "")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ledgerFixture is project "proj" with session q1 (a queue ticket's) and
// session "loose" (no project), each with turns on 2026-01-01 and 2026-01-02.
func ledgerFixture(t *testing.T) (Env, string) {
	t.Helper()
	dirs, root, _ := attrStore(t)
	writeLedgerTranscript(t, dirs, "q1", ledgerTurn("2026-01-01T12:00:00Z", 1000, 10), ledgerTurn("2026-01-02T12:00:00Z", 2000, 20))
	writeLedgerTranscript(t, dirs, "loose", ledgerTurn("2026-01-01T12:00:00Z", 500, 5))
	state := filepath.Join(t.TempDir(), "state")
	return Env{Dirs: dirs, Root: root, LookupRoot: root, StateDir: state}, state
}

func ledgerRow(t *testing.T, state, day, project string) (tokenledger.Row, bool) {
	t.Helper()
	l, err := tokenledger.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := l.Day(day)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Project == project {
			return r, true
		}
	}
	return tokenledger.Row{}, false
}

func ledgerBytes(t *testing.T, state string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(state, tokenledger.FileName))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return b
}

func TestLedgerStepRebuildsThenChangesNothing(t *testing.T) {
	env, state := ledgerFixture(t)
	changes, _, err := ledgerRegistry().Run(env)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := ledgerRow(t, state, "2026-01-01", "proj")
	if !ok || !r.Rebuilt || r.Tokens != 1010 || r.Group != tokenledger.GroupQueue || r.Harness != "claude" {
		t.Errorf("Jan 1 proj row = %+v, ok=%v", r, ok)
	}
	if len(changes) != 1 || len(changes[0].Notes) != 1 || !strings.Contains(changes[0].Notes[0], "2 days written") {
		t.Errorf("changes = %+v", changes)
	}
	before := ledgerBytes(t, state)

	changes, _, err = ledgerRegistry().Run(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 || !bytes.Equal(before, ledgerBytes(t, state)) {
		t.Errorf("second run changed something: %+v", changes)
	}
}

func TestLedgerStepSkipsLiveDay(t *testing.T) {
	env, state := ledgerFixture(t)
	testutil.WriteFile(t, state, tokenledger.FileName, `{"rows":[{"day":"2026-01-02","project":"proj","group":"Queue","harness":"claude","tokens":7,"dollars":0.5}],"seen":{},"firstLiveDay":"2026-01-02"}`)

	changes, _, err := ledgerRegistry().Run(env)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := ledgerRow(t, state, "2026-01-02", "proj"); r.Tokens != 7 || r.Rebuilt {
		t.Errorf("live row rewritten: %+v", r)
	}
	if r, ok := ledgerRow(t, state, "2026-01-01", "proj"); !ok || !r.Rebuilt {
		t.Errorf("Jan 1 not rebuilt: %+v", r)
	}
	if len(changes) != 1 || !strings.Contains(changes[0].Notes[0], "1 days skipped") {
		t.Errorf("changes = %+v", changes)
	}
}

func TestLedgerStepSkipsTodayWithoutLiveRow(t *testing.T) {
	env, state := ledgerFixture(t)
	today := time.Now().Format(tokenledger.DayLayout)
	writeLedgerTranscript(t, env.Dirs, "q1",
		ledgerTurn("2026-01-01T12:00:00Z", 1000, 10),
		ledgerTurn(time.Now().UTC().Format(time.RFC3339), 100, 1))

	if _, _, err := ledgerRegistry().Run(env); err != nil {
		t.Fatal(err)
	}
	if _, ok := ledgerRow(t, state, today, "proj"); ok {
		t.Error("today was rebuilt")
	}
	if _, ok := ledgerRow(t, state, "2026-01-01", "proj"); !ok {
		t.Error("Jan 1 not rebuilt")
	}
}

func TestLedgerStepDryRunWritesNothing(t *testing.T) {
	env, state := ledgerFixture(t)
	env.DryRun = true
	changes, _, err := ledgerRegistry().Run(env)
	if err != nil {
		t.Fatal(err)
	}
	if b := ledgerBytes(t, state); b != nil {
		t.Errorf("dry run wrote the ledger: %s", b)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("dry run created the state dir: %v", err)
	}
	if len(changes) != 1 || !strings.Contains(changes[0].Notes[0], "2 days would be written") ||
		!strings.Contains(changes[0].Notes[0], "tokens") {
		t.Errorf("changes = %+v", changes)
	}
}

func TestLedgerStepNoProjectOncePerPass(t *testing.T) {
	env, stateA := ledgerFixture(t)
	stateB := filepath.Join(t.TempDir(), "state")
	pass := &Pass{}

	env.Pass = pass
	if _, _, err := ledgerRegistry().Run(env); err != nil {
		t.Fatal(err)
	}
	env.StateDir = stateB
	if _, _, err := ledgerRegistry().Run(env); err != nil {
		t.Fatal(err)
	}

	if r, ok := ledgerRow(t, stateA, "2026-01-01", ""); !ok || r.Tokens != 505 || r.Group != tokenledger.GroupOther {
		t.Errorf("first run no-project row = %+v, ok=%v", r, ok)
	}
	if _, ok := ledgerRow(t, stateB, "2026-01-01", ""); ok {
		t.Error("second run of the pass rebuilt the no-project transcripts")
	}
	if _, ok := ledgerRow(t, stateB, "2026-01-01", "proj"); !ok {
		t.Error("second run skipped its own project")
	}
}

func TestLedgerStepRecordedOnlyAfterSuccess(t *testing.T) {
	env, _ := ledgerFixture(t)
	env.AppliedPath = filepath.Join(t.TempDir(), "applied.json")

	// A state dir below a regular file cannot be written.
	blocker := filepath.Join(t.TempDir(), "file")
	testutil.WriteFile(t, filepath.Dir(blocker), "file", "x")
	good := env.StateDir
	env.StateDir = filepath.Join(blocker, "state")
	if _, _, err := ledgerRegistry().Run(env); err == nil {
		t.Fatal("want an error from the unwritable ledger")
	}
	if got, _ := readApplied(env.AppliedPath); slices.Contains(got, ledgerStepName) {
		t.Errorf("failed step recorded: %v", got)
	}

	env.StateDir = good
	if _, _, err := ledgerRegistry().Run(env); err != nil {
		t.Fatal(err)
	}
	if got, _ := readApplied(env.AppliedPath); !slices.Contains(got, ledgerStepName) {
		t.Errorf("step not recorded: %v", got)
	}
}

func TestLedgerStepNotAStoreProjectOrNoTranscripts(t *testing.T) {
	env, state := ledgerFixture(t)
	notStore := t.TempDir()
	changes, _, err := ledgerRegistry().Run(Env{Dirs: env.Dirs, Root: notStore, LookupRoot: notStore, StateDir: state})
	if err != nil || len(changes) != 0 {
		t.Errorf("not a store project: %+v, %v", changes, err)
	}

	dirs, root, _ := attrStore(t)
	var none []tickets.MigrationChange
	none, _, err = ledgerRegistry().Run(Env{Dirs: dirs, Root: root, LookupRoot: root, StateDir: state})
	if err != nil || len(none) != 0 || ledgerBytes(t, state) != nil {
		t.Errorf("no transcripts: %+v, %v", none, err)
	}
}
