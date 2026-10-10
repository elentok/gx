package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// setupCloseProject is a project with a follow-ups epic holding open 01, done
// 02, open-with-run-history 03 and cancelled 04, plus an unrelated epic.
func setupCloseProject(t *testing.T) moveFixture {
	t.Helper()
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repo)
	for _, e := range []string{followUpsEpic, "work"} {
		testutil.EnsureEpicTicketMD(t, filepath.Join(project, e))
		if err := os.MkdirAll(filepath.Join(project, e, "issues"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	fu := filepath.Join(project, followUpsEpic, "issues")
	testutil.WriteFile(t, fu, "01-one.md", "---\nid: \"01\"\nstatus: open\ntype: research\n---\n\n# 01 — One\n\nbody\n")
	testutil.WriteFile(t, fu, "02-two.md", "---\nid: \"02\"\nstatus: done\ntype: research\n---\n\n# 02 — Two\n")
	testutil.WriteFile(t, fu, "03-run.md", "---\nid: \"03\"\nstatus: open\ntype: research\nsession_ids:\n  - abc\n---\n\n# 03 — Run\n")
	testutil.WriteFile(t, fu, "04-gone.md", "---\nid: \"04\"\nstatus: cancelled\ntype: research\n---\n\n# 04 — Gone\n")
	testutil.WriteFile(t, filepath.Join(project, "work", "issues"), "01-w.md", "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01\n")
	return moveFixture{repo: repo, project: project}
}

func TestExecute_TicketsClose_EachReasonArchivesWithResolution(t *testing.T) {
	for _, tc := range []struct{ reason, note string }{
		{"fixed", ""},
		{"obsolete", "no longer relevant"},
		{"not-worth-it", ""},
		{"duplicate", "mine:follow-ups/02"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			f := setupCloseProject(t)
			args := []string{"tickets", "close", "follow-ups/01", "--reason", tc.reason}
			if tc.note != "" {
				args = append(args, "--note", tc.note)
			}
			out, err := runIn(t, f.repo, args...)
			if err != nil {
				t.Fatalf("close: %v", err)
			}
			if out != "mine:follow-ups-archived/01\n" {
				t.Errorf("stdout = %q, want the archive address", out)
			}
			if _, err := os.Stat(filepath.Join(f.project, followUpsEpic, "issues", "01-one.md")); !os.IsNotExist(err) {
				t.Errorf("old file still there: %v", err)
			}
			path := filepath.Join(f.project, followUpsArchivedEpic, "issues", "01-one.md")
			got, err := schema.ParseTicket(path)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != schema.StatusCancelled {
				t.Errorf("status = %q, want cancelled", got.Status)
			}
			raw, _ := os.ReadFile(path)
			res := schema.Section(schema.ParseBody(string(raw)), "Resolution")
			for _, want := range []string{"reason: " + tc.reason, "original: mine:follow-ups/01", tc.note} {
				if !strings.Contains(res, want) {
					t.Errorf("## Resolution missing %q:\n%s", want, res)
				}
			}
		})
	}
}

func TestExecute_TicketsClose_CreatesArchiveDraftOnce(t *testing.T) {
	f := setupCloseProject(t)
	if _, err := runIn(t, f.repo, "tickets", "close", "follow-ups/01", "--reason", "fixed"); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Join(f.project, followUpsArchivedEpic, "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "status: draft") {
		t.Errorf("archive not draft:\n%s", md)
	}
	// A second close reuses it and renumbers.
	out, err := runIn(t, f.repo, "tickets", "close", "follow-ups/03", "--reason", "obsolete")
	if err != nil {
		t.Fatalf("close with run history: %v", err)
	}
	if out != "mine:follow-ups-archived/02\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestExecute_TicketsClose_RefusalsWriteNothing(t *testing.T) {
	f := setupCloseProject(t)
	for _, tc := range []struct {
		name string
		args []string
		code string
	}{
		{"outside follow-ups", []string{"work/01", "--reason", "fixed"}, tickets.CodeNotFollowUpsEpic},
		{"done status", []string{"follow-ups/02", "--reason", "fixed"}, tickets.CodeCloseStatus},
		{"cancelled status", []string{"follow-ups/04", "--reason", "fixed"}, tickets.CodeCloseStatus},
		{"duplicate without note", []string{"follow-ups/01", "--reason", "duplicate"}, tickets.CodeCloseNoteNeeded},
		{"bad reason", []string{"follow-ups/01", "--reason", "meh"}, tickets.CodeCloseReason},
		{"unknown ticket", []string{"follow-ups/99", "--reason", "fixed"}, tickets.CodeUnknownTicket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := moveTreeSnapshot(t, f.project)
			_, err := runIn(t, f.repo, append([]string{"tickets", "close"}, tc.args...)...)
			var ae *tickets.AddressError
			if !errors.As(err, &ae) || ae.Code != tc.code {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
			if after := moveTreeSnapshot(t, f.project); after != before {
				t.Errorf("refusal wrote something:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

func TestExecute_TicketsClose_DoneSweepMovesOnlyDone(t *testing.T) {
	f := setupCloseProject(t)
	out, err := runIn(t, f.repo, "tickets", "close", "--done")
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if out != "mine:follow-ups-archived/01\n" {
		t.Errorf("stdout = %q", out)
	}
	got, err := schema.ParseTicket(filepath.Join(f.project, followUpsArchivedEpic, "issues", "01-two.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != schema.StatusDone {
		t.Errorf("status = %q, want done kept", got.Status)
	}
	for _, name := range []string{"01-one.md", "03-run.md", "04-gone.md"} {
		if _, err := os.Stat(filepath.Join(f.project, followUpsEpic, "issues", name)); err != nil {
			t.Errorf("%s should stay: %v", name, err)
		}
	}
}
