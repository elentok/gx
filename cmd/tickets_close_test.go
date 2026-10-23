package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
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
			if got.CancelledAt.IsZero() {
				t.Error("cancelled_at not stamped on the archived ticket")
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

func TestExecute_TicketsClose_StampsZeroFrictionOnANeverRunTicket(t *testing.T) {
	f := setupCloseProject(t)
	if _, err := runIn(t, f.repo, "tickets", "close", "follow-ups/01", "--reason", "fixed"); err != nil {
		t.Fatalf("close: %v", err)
	}
	got, err := schema.ParseTicket(filepath.Join(f.project, followUpsArchivedEpic, "issues", "01-one.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Friction == nil || got.Friction.TotalPausedSecs != 0 || len(got.Friction.PausesByReason) != 0 {
		t.Errorf("friction = %+v, want a zero summary", got.Friction)
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

func TestExecute_TicketsClose_RefusesQueuedAndLiveTickets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		queue []server.QueueItem
		live  []server.IterationInfo
	}{
		{"queued", []server.QueueItem{{Address: "mine:follow-ups/01", Agent: "claude"}}, nil},
		{"live", nil, []server.IterationInfo{{Address: "mine:follow-ups/01"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupCloseProject(t)
			cl, pings := fakeMoveServer(t, tc.queue, tc.live)
			before := moveTreeSnapshot(t, f.project)
			d := deps{
				stdout:       bytes.NewBuffer(nil),
				stderr:       bytes.NewBuffer(nil),
				getwd:        func() (string, error) { return f.repo, nil },
				serverClient: cl,
			}
			err := execute([]string{"tickets", "close", "follow-ups/01", "--reason", "fixed"}, d)
			requireMoveActive(t, err)
			if after := moveTreeSnapshot(t, f.project); after != before {
				t.Errorf("refusal wrote something:\nbefore %s\nafter  %s", before, after)
			}
			if _, err := os.Stat(filepath.Join(f.project, followUpsArchivedEpic)); !os.IsNotExist(err) {
				t.Errorf("archive epic created: %v", err)
			}
			if len(*pings) != 0 {
				t.Errorf("refusal pinged %v", *pings)
			}
		})
	}
}

func TestExecute_TicketsClose_SweepLeavesLogBesideArchivedTicket(t *testing.T) {
	f := setupCloseProject(t)
	fu := filepath.Join(f.project, followUpsEpic, "issues")
	testutil.WriteFile(t, fu, "05-kept.md", "---\nid: \"05\"\nstatus: cancelled\ntype: research\n---\n\n# 05 — Kept\n")
	const log = "{\"a\":1}\n{\"b\":2}\n"
	testutil.WriteFile(t, fu, "05-kept.jsonl", log)
	if _, err := runIn(t, f.repo, "tickets", "close", "--sweep"); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(f.project, followUpsArchivedEpic, "issues", "03-kept.jsonl"))
	if err != nil || string(got) != log {
		t.Errorf("archived log = %q, %v; want %q", got, err, log)
	}
	if _, err := os.Stat(filepath.Join(fu, "05-kept.jsonl")); !os.IsNotExist(err) {
		t.Errorf("old log still there: %v", err)
	}
}

func TestExecute_TicketsClose_SweepMovesDoneAndCancelled(t *testing.T) {
	for _, flag := range []string{"--sweep", "--done"} {
		t.Run(flag, func(t *testing.T) {
			f := setupCloseProject(t)
			fu := filepath.Join(f.project, followUpsEpic, "issues")
			testutil.WriteFile(t, fu, "05-kept.md", "---\nid: \"05\"\nstatus: cancelled\ntype: research\n---\n\n# 05 — Kept\n\n## Resolution\n\n- reason: fixed\n")
			out, err := runIn(t, f.repo, "tickets", "close", flag)
			if err != nil {
				t.Fatalf("sweep: %v", err)
			}
			if out != "mine:follow-ups-archived/01\nmine:follow-ups-archived/02\nmine:follow-ups-archived/03\n" {
				t.Errorf("stdout = %q", out)
			}
			archived := filepath.Join(f.project, followUpsArchivedEpic, "issues")
			for _, tc := range []struct {
				name       string
				status     schema.Status
				resolution string
			}{
				{"01-two.md", schema.StatusDone, ""},
				{"02-gone.md", schema.StatusCancelled, "- original: mine:follow-ups/04"},
				{"03-kept.md", schema.StatusCancelled, "- reason: fixed"},
			} {
				got, err := schema.ParseTicket(filepath.Join(archived, tc.name))
				if err != nil {
					t.Fatal(err)
				}
				if got.Status != tc.status {
					t.Errorf("%s status = %q, want %q kept", tc.name, got.Status, tc.status)
				}
				raw, err := os.ReadFile(filepath.Join(archived, tc.name))
				if err != nil {
					t.Fatal(err)
				}
				if res := strings.TrimSpace(schema.Section(schema.ParseBody(string(raw)), "Resolution")); res != tc.resolution {
					t.Errorf("%s resolution = %q, want %q", tc.name, res, tc.resolution)
				}
			}
			for _, name := range []string{"01-one.md", "03-run.md"} {
				if _, err := os.Stat(filepath.Join(fu, name)); err != nil {
					t.Errorf("%s should stay: %v", name, err)
				}
			}
		})
	}
}
