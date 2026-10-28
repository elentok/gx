package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// registeredCheckRepo is a TempRepo registered as project "p" with checksJSON
// committed; it returns the repo and gx's data dir.
func registeredCheckRepo(t *testing.T, checksJSON string) (repo, data string) {
	t.Helper()
	repo = testutil.TempRepo(t)
	registerWithTrunk(t, repo, "main")
	testutil.Mkdir(t, filepath.Join(repo, ".gx"))
	testutil.WriteFile(t, repo, ".gx/checks.json", checksJSON)
	testutil.CommitAll(t, repo, "checks")
	return repo, filepath.Join(envValue(envFor(t), "XDG_DATA_HOME"), "gx")
}

func kindRecord(t *testing.T, rec check.Record, kind string) check.KindRecord {
	t.Helper()
	for _, k := range rec.Kinds {
		if k.Kind == kind {
			return k
		}
	}
	t.Fatalf("record has no kind %q: %+v", kind, rec.Kinds)
	return check.KindRecord{}
}

func TestCheck_RecordHasEveryField(t *testing.T) {
	repo, data := registeredCheckRepo(t, `{"test":"true"}`)
	if out, code := runCheckCmd(t, repo, "--full", "--kind", "test"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	recs, err := check.ReadRecords(data, "p")
	if err != nil || len(recs) != 1 {
		t.Fatalf("records = %+v, err %v", recs, err)
	}
	rec := recs[0]
	head, err := git.RevParse(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := git.TreeID(repo)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Run == "" || rec.Time == "" || rec.Exit != 0 || rec.Scope != "full" || rec.KindFilter != "test" ||
		rec.Branch != "main" || rec.Worktree != repo || rec.Head != head || rec.TreeID != tree || rec.Link != check.LinkTrunk {
		t.Errorf("record = %+v", rec)
	}
	if k := kindRecord(t, rec, "test"); k.Status != check.StatusPass {
		t.Errorf("test kind = %+v", k)
	}
}

func TestCheck_RecordLinksTicketBranch(t *testing.T) {
	repo, data := registeredCheckRepo(t, `{"test":"true"}`)
	testutil.MustGitExported(t, repo, "checkout", "-b", "ralph-loop/widgets-item-03")
	if out, code := runCheckCmd(t, repo, "--full", "--kind", "test"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	recs, _ := check.ReadRecords(data, "p")
	if len(recs) != 1 || recs[0].Link != check.LinkTicket || recs[0].Epic != "widgets" || recs[0].Ticket != "03" {
		t.Errorf("records = %+v", recs)
	}
}

func TestCheck_SweepsOldRecordsAndRunLogs(t *testing.T) {
	repo, data := registeredCheckRepo(t, `{"test":"true"}`)
	checkDir := filepath.Join(data, "check")
	testutil.Mkdir(t, checkDir)
	old := time.Now().Add(-31 * 24 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	testutil.WriteFile(t, checkDir, "p.jsonl",
		`{"run":"old","time":"`+old+`"}`+"\n"+`{"run":"recent","time":"`+recent+`"}`+"\n")
	oldDir := filepath.Join(checkDir, "old-run")
	testutil.Mkdir(t, oldDir)
	when := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(oldDir, when, when); err != nil {
		t.Fatal(err)
	}

	if out, code := runCheckCmd(t, repo, "--full", "--kind", "test"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	recs, _ := check.ReadRecords(data, "p")
	var runs []string
	for _, r := range recs {
		runs = append(runs, r.Run)
	}
	if len(runs) != 2 || runs[0] != "recent" || runs[1] == "old" {
		t.Errorf("runs = %v", runs)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old run dir still there: %v", err)
	}
}

func TestCheck_ConfiguredCommandWithoutBriefRecordsNoTestNames(t *testing.T) {
	repo, data := registeredCheckRepo(t, `{"test":"echo FAIL some.Test; exit 1"}`)
	if out, code := runCheckCmd(t, repo, "--full", "--kind", "test"); code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	recs, _ := check.ReadRecords(data, "p")
	if len(recs) != 1 || recs[0].Exit != 1 {
		t.Fatalf("records = %+v", recs)
	}
	if k := kindRecord(t, recs[0], "test"); k.Status != check.StatusFail || len(k.Failed) != 0 {
		t.Errorf("test kind = %+v", k)
	}
}

func TestCheck_BriefCalledByConfiguredCommandNamesFailingTests(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the gx binary")
	}
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "gx"), "github.com/elentok/gx").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	prependPath(t, bin)
	repo, data := registeredCheckRepo(t, `{"test":"gx brief go-test ./..."}`)
	testutil.WriteFile(t, repo, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\nfunc TestB(t *testing.T) { t.Fail() }\n")
	testutil.CommitAll(t, repo, "module")

	if out, code := runCheckCmd(t, repo, "--full", "--kind", "test"); code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	recs, _ := check.ReadRecords(data, "p")
	if len(recs) != 1 {
		t.Fatalf("records = %+v", recs)
	}
	if k := kindRecord(t, recs[0], "test"); !reflect.DeepEqual(k.Failed, []string{"example.com/tiny.TestB"}) {
		t.Errorf("failed = %v", k.Failed)
	}
	lines, err := brief.ReadIndex(data)
	if err != nil || len(lines) != 1 || lines[0].CheckRun != recs[0].Run {
		t.Errorf("index = %+v, err %v; run %q", lines, err, recs[0].Run)
	}
}
