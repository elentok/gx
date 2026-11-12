package ticketsflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

// triageProject is a project with two open follow-ups.
func triageProject(t *testing.T) (repo string) {
	t.Helper()
	repo, _ = triageProjectDir(t)
	return repo
}

func triageProjectDir(t *testing.T) (repo, project string) {
	t.Helper()
	repo, project = setupShowProject(t)
	for _, id := range []string{"01", "02"} {
		writeEpicTicket(t, project, tickets.FollowUpsEpic, id+"-f.md",
			"---\nid: \""+id+"\"\nstatus: open\ntype: research\ncategory: bug\n---\n\n# Follow-up "+id+"\n")
	}
	return repo, project
}

type triageShown struct {
	ID       string `json:"id"`
	Step     string `json:"step"`
	Next     string `json:"next"`
	Revision int    `json:"revision"`
	Rows     []struct {
		Address string `json:"address"`
		Verdict string `json:"verdict"`
		Write   string `json:"write"`
		Group   string `json:"group"`
		Plan    *struct {
			Action string `json:"action"`
		} `json:"plan"`
	} `json:"rows"`
}

func triageShow(t *testing.T, repo string) triageShown {
	t.Helper()
	out, err := runIn(t, repo, "tickets", "triage", "show", "--json")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	var s triageShown
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("show json %q: %v", out, err)
	}
	return s
}

func triagePending(t *testing.T, repo string) []string {
	t.Helper()
	out, err := runIn(t, repo, "tickets", "triage", "pending", "--json")
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	var rows []struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("pending json %q: %v", out, err)
	}
	var addrs []string
	for _, r := range rows {
		addrs = append(addrs, r.Address)
	}
	return addrs
}

// runInStdin is runIn with stdin.
func runInStdin(t *testing.T, cwd, stdin string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	d := testDeps(t, &stdout, bytes.NewBuffer(nil), func() (string, error) { return cwd, nil })
	d.Stdin = strings.NewReader(stdin)
	err := execute(t, args, d)
	return stdout.String(), err
}

// triageAtApprove creates a record, verifies both rows and saves a plan for
// them, optionally with a quick question on the first row.
func triageAtApprove(t *testing.T, repo string, withQuestion bool) triageShown {
	t.Helper()
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	rows := triagePending(t, repo)
	for _, addr := range rows {
		if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", addr); err != nil {
			t.Fatal(err)
		}
	}
	q := ""
	if withQuestion {
		q = `,"questions":[{"id":"q1","text":"which?","kind":"quick","options":["a","b"],"default":"a"}]`
	}
	plan := `[{"rowId":"` + rows[0] + `","action":"ready","destination":"x"` + q + `},{"rowId":"` + rows[1] + `","action":"ready"}]`
	if _, err := runInStdin(t, repo, plan, "tickets", "triage", "plan"); err != nil {
		t.Fatalf("plan: %v", err)
	}
	return triageShow(t, repo)
}

func TestTriage_PlanSaveMovesToApproveAndDropsGoneRows(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	rows := triagePending(t, repo)
	for _, addr := range rows {
		if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", addr); err != nil {
			t.Fatal(err)
		}
	}
	plan := `[{"rowId":"` + rows[0] + `","action":"ready"},{"rowId":"` + rows[1] + `","action":"ready"}]`
	if _, err := runInStdin(t, repo, plan, "tickets", "triage", "plan", "--gone", rows[1]); err != nil {
		t.Fatalf("plan: %v", err)
	}
	s := triageShow(t, repo)
	if s.Next != "approve" || s.Step != "approve" {
		t.Errorf("after plan next/step = %q/%q, want approve/approve", s.Next, s.Step)
	}
	for _, r := range s.Rows {
		if r.Address == rows[1] && (r.Write != "gone" || r.Plan != nil) {
			t.Errorf("gone row write/plan = %q/%v, want gone/none", r.Write, r.Plan)
		}
		if r.Address == rows[0] && r.Plan == nil {
			t.Error("planned row has no plan")
		}
	}
}

func TestTriage_RowRefusals(t *testing.T) {
	repo := triageProject(t)
	// A record still at verify refuses a row save.
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	s0 := triageShow(t, repo)
	_, err := runIn(t, repo, "tickets", "triage", "row", "--row", s0.Rows[0].Address, "--revision", strconv.Itoa(s0.Revision))
	if err == nil {
		t.Error("row at step verify accepted")
	}
	for _, addr := range triagePending(t, repo) {
		if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", addr); err != nil {
			t.Fatal(err)
		}
	}
	plan := `[{"rowId":"` + s0.Rows[0].Address + `","action":"ready"},{"rowId":"` + s0.Rows[1].Address + `","action":"ready"}]`
	if _, err := runInStdin(t, repo, plan, "tickets", "triage", "plan"); err != nil {
		t.Fatal(err)
	}
	s := triageShow(t, repo)
	row := s.Rows[0].Address
	rev := strconv.Itoa(s.Revision)

	if _, err := runIn(t, repo, "tickets", "triage", "row", "--row", row, "--action", "map"); err == nil {
		t.Error("row without --revision accepted")
	}
	if _, err := runIn(t, repo, "tickets", "triage", "row", "--row", row, "--action", "map", "--revision", "99"); err == nil {
		t.Error("row with a stale revision accepted")
	}
	out, err := runIn(t, repo, "tickets", "triage", "row", "--row", row, "--action", "map", "--revision", rev)
	if err != nil || strings.TrimSpace(out) != strconv.Itoa(s.Revision+1) {
		t.Fatalf("row save = %q, %v, want revision %d", out, err, s.Revision+1)
	}
}

func TestTriage_RowUngroupUndoesAGroupAtApproveOnly(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	rows := triagePending(t, repo)
	for _, addr := range rows {
		if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", addr); err != nil {
			t.Fatal(err)
		}
	}
	rev := strconv.Itoa(triageShow(t, repo).Revision)
	if _, err := runIn(t, repo, "tickets", "triage", "row", "--row", rows[0], "--ungroup", "--revision", rev); err == nil {
		t.Fatal("ungroup accepted outside step approve")
	}
	plan := `[{"rowId":"` + rows[0] + `","action":"ready","estimate":"10K","overlapWith":["` + rows[1] + `"]},` +
		`{"rowId":"` + rows[1] + `","action":"ready","estimate":"10K","overlapWith":["` + rows[0] + `"]}]`
	if _, err := runInStdin(t, repo, plan, "tickets", "triage", "plan"); err != nil {
		t.Fatal(err)
	}
	s := triageShow(t, repo)
	if s.Rows[0].Group == "" || s.Rows[0].Group != s.Rows[1].Group {
		t.Fatalf("groups = %q, %q, want one group", s.Rows[0].Group, s.Rows[1].Group)
	}
	out, err := runIn(t, repo, "tickets", "triage", "row", "--row", rows[0], "--ungroup", "--revision", strconv.Itoa(s.Revision))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, rows[0]+" blocked_by "+rows[1]) || !strings.Contains(out, rows[1]+" blocked_by "+rows[0]) {
		t.Errorf("output %q does not print the chain", out)
	}
	if after := triageShow(t, repo); after.Rows[0].Group != "" || after.Rows[1].Group != "" {
		t.Errorf("rows still grouped: %q, %q", after.Rows[0].Group, after.Rows[1].Group)
	}
}

func TestTriage_ApprovePrintsTheGroupRow(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	rows := triagePending(t, repo)
	for _, addr := range rows {
		if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", addr); err != nil {
			t.Fatal(err)
		}
	}
	plan := `[{"rowId":"` + rows[0] + `","action":"ready","estimate":"10K","overlapWith":["` + rows[1] + `"]},` +
		`{"rowId":"` + rows[1] + `","action":"ready","estimate":"10K","overlapWith":["` + rows[0] + `"]}]`
	if _, err := runInStdin(t, repo, plan, "tickets", "triage", "plan"); err != nil {
		t.Fatal(err)
	}
	s := triageShow(t, repo)
	out, err := runIn(t, repo, "tickets", "triage", "approve", "--revision", strconv.Itoa(s.Revision))
	if err != nil {
		t.Fatal(err)
	}
	want := "group " + s.Rows[0].Group + ": " + rows[0] + ", " + rows[1]
	if !strings.Contains(out, want) {
		t.Errorf("approve output %q lacks %q", out, want)
	}
}

func TestTriage_ApproveTwiceSaysAlreadyApproved(t *testing.T) {
	repo := triageProject(t)
	s := triageAtApprove(t, repo, false)
	rev := strconv.Itoa(s.Revision)
	out, err := runIn(t, repo, "tickets", "triage", "approve", "--revision", rev)
	if err != nil || strings.TrimSpace(out) != strconv.Itoa(s.Revision+1)+" write" {
		t.Fatalf("approve = %q, %v, want %d write", out, err, s.Revision+1)
	}
	_, err = runIn(t, repo, "tickets", "triage", "approve", "--revision", rev)
	if err == nil || !strings.Contains(err.Error(), "already approved") {
		t.Errorf("second approve = %v, want already approved", err)
	}
}

func TestTriage_ApproveAfterRowSaveWithOldRevisionIsStale(t *testing.T) {
	repo := triageProject(t)
	s := triageAtApprove(t, repo, false)
	rev := strconv.Itoa(s.Revision)
	if _, err := runIn(t, repo, "tickets", "triage", "row", "--row", s.Rows[0].Address, "--action", "map", "--revision", rev); err != nil {
		t.Fatal(err)
	}
	_, err := runIn(t, repo, "tickets", "triage", "approve", "--revision", rev)
	if !errors.Is(err, triage.ErrStaleRevision) {
		t.Errorf("approve with old revision = %v, want stale", err)
	}
}

func TestTriage_ApproveRefusedWhileQuestionOpen(t *testing.T) {
	repo := triageProject(t)
	s := triageAtApprove(t, repo, true)
	_, err := runIn(t, repo, "tickets", "triage", "approve", "--revision", strconv.Itoa(s.Revision))
	if err == nil || !strings.Contains(err.Error(), "open") {
		t.Errorf("approve with open question = %v, want refusal", err)
	}
	if got := triageShow(t, repo); got.Step != "approve" {
		t.Errorf("step = %q, want approve", got.Step)
	}
}

func TestTriage_AbandonRefusedAtWrite(t *testing.T) {
	repo := triageProject(t)
	s := triageAtApprove(t, repo, false)
	if _, err := runIn(t, repo, "tickets", "triage", "approve", "--revision", strconv.Itoa(s.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "abandon"); err == nil {
		t.Error("abandon at write accepted")
	}
}

func TestTriage_AbandonAtApproveEndsTheLiveRecord(t *testing.T) {
	repo := triageProject(t)
	triageAtApprove(t, repo, false)
	if _, err := runIn(t, repo, "tickets", "triage", "abandon"); err != nil {
		t.Fatalf("abandon: %v", err)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "pending"); err == nil {
		t.Error("a live record remains after abandon")
	}
}

func TestTriage_CreateThenSecondCreateRefusesNamingLive(t *testing.T) {
	repo := triageProject(t)
	out, err := runIn(t, repo, "tickets", "triage", "create")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatal("create printed no id")
	}
	_, err = runIn(t, repo, "tickets", "triage", "create")
	if err == nil || !strings.Contains(err.Error(), id) {
		t.Errorf("second create = %v, want a refusal naming %s", err, id)
	}
}

func TestTriage_ShowNextAcrossTheVerifyStep(t *testing.T) {
	repo := triageProject(t)
	if s := triageShow(t, repo); s.Next != "start" {
		t.Errorf("empty next = %q, want start", s.Next)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	s := triageShow(t, repo)
	if s.Next != "verify" || len(s.Rows) != 2 {
		t.Errorf("pending next/rows = %q/%d, want verify/2", s.Next, len(s.Rows))
	}
	for _, addr := range triagePending(t, repo) {
		if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", addr, "--evidence", "e"); err != nil {
			t.Fatalf("verdict %s: %v", addr, err)
		}
	}
	if s := triageShow(t, repo); s.Next != "plan" || s.Step != "plan" {
		t.Errorf("after last verdict next/step = %q/%q, want plan/plan", s.Next, s.Step)
	}
}

func TestTriage_PendingShrinksAndGoneIsSaved(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	pending := triagePending(t, repo)
	if len(pending) != 2 {
		t.Fatalf("pending = %v, want 2", pending)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", pending[0], "--verdict", "fixed"); err != nil {
		t.Fatal(err)
	}
	// A second session reads only the rows with no verdict.
	rest := triagePending(t, repo)
	if len(rest) != 1 || rest[0] != pending[1] {
		t.Fatalf("pending after one verdict = %v, want [%s]", rest, pending[1])
	}
	if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", pending[1], "--gone"); err != nil {
		t.Fatal(err)
	}
	if got := triagePending(t, repo); len(got) != 0 {
		t.Errorf("pending = %v, want none", got)
	}
	s := triageShow(t, repo)
	for _, r := range s.Rows {
		if r.Address == pending[1] && r.Write != "gone" {
			t.Errorf("gone row write = %q, want gone", r.Write)
		}
	}
}

func TestTriage_StaleRevisionIsRefusedAndRecordUnchanged(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	before := triageShow(t, repo)
	row := before.Rows[0].Address
	_, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", row, "--revision", "99")
	if err == nil {
		t.Fatal("stale revision accepted")
	}
	after := triageShow(t, repo)
	if after.Revision != before.Revision || after.Rows[0].Verdict != "" {
		t.Errorf("record changed: revision %d -> %d, verdict %q", before.Revision, after.Revision, after.Rows[0].Verdict)
	}
	out, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", row, "--revision", "1")
	if err != nil {
		t.Fatalf("current revision refused: %v", err)
	}
	if strings.TrimSpace(out) != "2" {
		t.Errorf("printed revision %q, want 2", out)
	}
}

func TestDropRows_RewritesTheManifest(t *testing.T) {
	repo := triageProject(t)
	dir := t.TempDir()
	for _, f := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := filepath.Join(dir, "manifest.yaml")
	body := "- slug: a\n  type: implement\n  body: a.md\n  from: p:f/01\n" +
		"- slug: b\n  type: implement\n  body: b.md\n  blocked_by: [a]\n  from: p:f/02\n"
	if err := os.WriteFile(m, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runIn(t, repo, "tickets", "triage", "drop-rows", "--manifest", m, "--row", "p:f/01")
	if err != nil {
		t.Fatalf("drop-rows: %v", err)
	}
	if !strings.Contains(out, "dropped a") {
		t.Errorf("output %q", out)
	}
	got, _ := os.ReadFile(m)
	if strings.Contains(string(got), "slug: a") || strings.Contains(string(got), "blocked_by") {
		t.Errorf("manifest still names a:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.md")); !os.IsNotExist(err) {
		t.Errorf("a.md not removed: %v", err)
	}
}

func TestDropRows_RequiresManifest(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "drop-rows", "--row", "p:f/01"); err == nil {
		t.Fatal("want an error without --manifest")
	}
}
