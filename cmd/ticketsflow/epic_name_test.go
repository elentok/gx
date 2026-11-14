package ticketsflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/testutil"
)

// mergeOldEpicTicket commits an earlier epic's ticket onto the repo's trunk,
// with the Ralph-Loop-Ticket trailer value a merging stamps.
func mergeOldEpicTicket(t *testing.T, repo, trailer string) {
	t.Helper()
	testutil.WriteFile(t, repo, "old-"+strings.NewReplacer(":", "-", "/", "-").Replace(trailer)+".txt", "old\n")
	testutil.CommitAll(t, repo, "old epic ticket")
	if err := history.AppendTrailer(repo, merge.TicketTrailerKey, trailer); err != nil {
		t.Fatalf("AppendTrailer: %v", err)
	}
}

func TestRunTicketsEpicAdd_RefusesNameUsedBefore(t *testing.T) {
	cwd, project := setupEpicAddProject(t)
	mergeOldEpicTicket(t, cwd, "mine:old/01")
	mergeOldEpicTicket(t, cwd, "legacy/01")
	testutil.MustGitExported(t, cwd, "branch", "branched")

	for _, tc := range []struct{ arg, wantErr string }{
		{"mine:old", `was used before`},
		{"mine:legacy", `was used before`},
		{"mine:branched", `branch "branched" exists`},
	} {
		err := runTicketsEpicAdd(config.DirsFrom(envFor(t)), cwd, tc.arg, "loop", "open", "", &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "-2") {
			t.Errorf("%s: err = %v, want containing %q and a -2 suggestion", tc.arg, err, tc.wantErr)
		}
		if _, statErr := os.Stat(filepath.Join(project, strings.TrimPrefix(tc.arg, "mine:"))); statErr == nil {
			t.Errorf("%s: refusal wrote an epic directory", tc.arg)
		}
	}
	if err := runTicketsEpicAdd(config.DirsFrom(envFor(t)), cwd, "mine:old-2", "loop", "open", "", &bytes.Buffer{}); err != nil {
		t.Errorf("old-2: %v, want created", err)
	}
}

func TestExecute_TicketsEpicNextName(t *testing.T) {
	cwd, _ := setupEpicAddProject(t)
	mergeOldEpicTicket(t, cwd, "mine:follow-ups-2026-11-12/01")
	if _, err := runIn(t, cwd, "tickets", "epic", "add", "mine:follow-ups-2026-11-12-2", "--kind", "loop"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ arg, want string }{
		{"mine:fresh", "mine:fresh\n"},
		{"mine:follow-ups-2026-11-12", "mine:follow-ups-2026-11-12-3\n"},
	} {
		out, err := runIn(t, cwd, "tickets", "epic", "next-name", tc.arg)
		if err != nil || out != tc.want {
			t.Errorf("next-name %s = (%q, %v), want %q", tc.arg, out, err, tc.want)
		}
	}
}
