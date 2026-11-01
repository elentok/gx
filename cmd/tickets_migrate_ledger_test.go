package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

// ledgerCmdFixture is a store project with one old session on 2026-01-01 and
// 2026-01-02, under this test's own HOME and state dir.
func ledgerCmdFixture(t *testing.T) (dirs config.Dirs, root, state string) {
	t.Helper()
	dirs = config.DirsFrom(envFor(t))
	root = filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	home, err := dirs.Home()
	if err != nil {
		t.Fatal(err)
	}
	path := transcript.PathIn(home, "/work/loose", "loose")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	turn := func(ts string, in int) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":%d,"output_tokens":0}}}`+"\n", ts, in)
	}
	data := turn("2026-01-01T12:00:00Z", 500) + turn("2026-01-02T12:00:00Z", 700)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if state, err = dirs.StateDir(); err != nil {
		t.Fatal(err)
	}
	return dirs, root, state
}

func ledgerCmdDayTokens(t *testing.T, state, day string) int64 {
	t.Helper()
	l, err := tokenledger.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := l.Day(day)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, r := range rows {
		total += r.Tokens
	}
	return total
}

func TestMigrateLedger_RebuildsThenRerunChangesNothing(t *testing.T) {
	dirs, root, state := ledgerCmdFixture(t)
	var out bytes.Buffer
	if err := runTicketsMigrate(dirs, root, false, &out); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if got := ledgerCmdDayTokens(t, state, "2026-01-01"); got != 500 {
		t.Errorf("Jan 1 tokens = %d, want 500", got)
	}
	before, err := os.ReadFile(filepath.Join(state, tokenledger.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := runTicketsMigrate(dirs, root, false, &out); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(state, tokenledger.FileName))
	if !bytes.Equal(before, after) {
		t.Errorf("rerun changed the ledger")
	}
}

func TestMigrateLedger_SkipsDaysTheLivePollWrote(t *testing.T) {
	dirs, root, state := ledgerCmdFixture(t)
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	live := `{"rows":[{"day":"2026-01-02","project":"proj","group":"Queue","harness":"claude","tokens":7,"dollars":0.5}],"seen":{},"firstLiveDay":"2026-01-02"}`
	if err := os.WriteFile(filepath.Join(state, tokenledger.FileName), []byte(live), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runTicketsMigrate(dirs, root, false, &out); err != nil {
		t.Fatal(err)
	}
	if got := ledgerCmdDayTokens(t, state, "2026-01-02"); got != 7 {
		t.Errorf("live day tokens = %d, want 7 (untouched)", got)
	}
	if got := ledgerCmdDayTokens(t, state, "2026-01-01"); got != 500 {
		t.Errorf("Jan 1 tokens = %d, want 500", got)
	}
}

func TestMigrateLedger_DryRunWritesNothingAndPrintsSummary(t *testing.T) {
	dirs, root, state := ledgerCmdFixture(t)
	var out bytes.Buffer
	if err := runTicketsMigrate(dirs, root, true, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(state, tokenledger.FileName)); !os.IsNotExist(err) {
		t.Errorf("dry run wrote the ledger: %v", err)
	}
	if !strings.Contains(out.String(), "days would be written") {
		t.Errorf("output lacks the summary:\n%s", out.String())
	}
}

func TestMigrateLedger_FailureReturnsError(t *testing.T) {
	dirs, root, _ := ledgerCmdFixture(t)
	home, _ := dirs.Home()
	// A file where the Codex home should be makes the transcript scan fail.
	if err := os.WriteFile(filepath.Join(home, ".codex"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runTicketsMigrate(dirs, root, false, &out); err == nil {
		t.Fatalf("want an error, got output:\n%s", out.String())
	}
}
