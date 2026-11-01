package server_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

const ledgerStep = "token-ledger-backfill"

func transcriptOf(stateDir string) string {
	// The harness's last HOME entry (claudeenv.In) is claude-home beside the state dir.
	return transcript.PathIn(filepath.Join(filepath.Dir(stateDir), "claude-home"), "/work/loose", "loose")
}

// withLedgerStep runs the real registry, whose last step is the ledger backfill.
func withLedgerStep(c *server.Config) { c.Migrations = migrations.Default }

// seedLedgerTranscript writes one old Claude session under the harness home.
func seedLedgerTranscript(t *testing.T, stateDir string) {
	t.Helper()
	path := transcriptOf(stateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	turn := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":%d,"output_tokens":%d}}}`+"\n", "2026-01-01T12:00:00Z", 500, 5)
	if err := os.WriteFile(path, []byte(turn), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ledgerDayTokens(t *testing.T, stateDir, day string) int64 {
	t.Helper()
	l, err := tokenledger.Open(stateDir)
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

// Seam: a store whose applied list lacks the ledger step runs it at start,
// before the token poll, and records it; a second start runs nothing.
func TestMigrateLedger_StartBackfillsAndRecords(t *testing.T) {
	t.Parallel()
	store := migrateStore(t, "proj")
	h := servertest.StartWithStore(t, store, withLedgerStep, func(c *server.Config) { seedLedgerTranscript(t, c.StateDir) })
	waitMigrations(t, h)

	if got := ledgerDayTokens(t, h.StateDir, "2026-01-01"); got != 505 {
		t.Errorf("rebuilt day tokens = %d, want 505", got)
	}
	l, err := tokenledger.Open(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if day, err := l.FirstLiveDay(); err != nil || day != "" {
		t.Errorf("first live day = %q, %v; the poll must not run before the step ends", day, err)
	}
	if got := appliedOf(t, store, "proj"); !strings.Contains(got, ledgerStep+"\n") {
		t.Errorf("applied = %q", got)
	}

	h2 := servertest.StartWithStore(t, store, withLedgerStep, func(c *server.Config) { seedLedgerTranscript(t, c.StateDir) })
	waitMigrations(t, h2)
	if got := ledgerDayTokens(t, h2.StateDir, "2026-01-01"); got != 0 {
		t.Errorf("second start rebuilt %d tokens, want 0", got)
	}
}

// Seam: a failing step gives the project a Migration block and start-up still ends.
func TestMigrateLedger_FailureRecordsAndStartContinues(t *testing.T) {
	t.Parallel()
	store := migrateStore(t, "proj")
	h := servertest.StartWithStore(t, store, withLedgerStep, func(c *server.Config) {
		seedLedgerTranscript(t, c.StateDir)
		// A file where the Codex home should be makes the transcript scan fail.
		codex := filepath.Join(filepath.Dir(c.StateDir), "claude-home", ".codex")
		if err := os.WriteFile(codex, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	waitMigrations(t, h)

	fails, err := migratefail.Read(h.StateDir, []string{"proj"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fails) != 1 || fails[0].Project != "proj" {
		t.Errorf("failures = %v", fails)
	}
	if got := appliedOf(t, store, "proj"); strings.Contains(got, ledgerStep) {
		t.Errorf("failed step recorded: %q", got)
	}
}
