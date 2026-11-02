package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
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

const slowSuite = `{"test":{"full":"sleep 3","changed":"sleep 3","limits":{"suite":"1s"}}}`

func TestCheck_SuiteLimitFailsFullRun(t *testing.T) {
	repo, _ := registeredCheckRepo(t, slowSuite)
	out, code := runCheckCmdLoad(t, func() float64 { return 0 }, repo, "--full", "--kind", "test")
	if code != 1 || !strings.Contains(out, "took 3.") || !strings.Contains(out, "1s limit") {
		t.Errorf("code %d, out %q; want exit 1 naming time and limit", code, out)
	}
}

func TestCheck_SuiteLimitIgnoredOnChangedRun(t *testing.T) {
	repo, _ := registeredCheckRepo(t, slowSuite)
	testutil.WriteFile(t, repo, "new.txt", "x")
	if out, code := runCheckCmdLoad(t, func() float64 { return 0 }, repo, "--kind", "test"); code != 0 {
		t.Errorf("changed run: code %d, out %q", code, out)
	}
}

func TestCheck_SuiteLimitWarnsUnderHighLoadAndRecordsLoad(t *testing.T) {
	repo, data := registeredCheckRepo(t, slowSuite)
	out, code := runCheckCmdLoad(t, func() float64 { return 1000 }, repo, "--full", "--kind", "test")
	if code != 0 || !strings.Contains(out, "warning:") {
		t.Fatalf("code %d, out %q; want exit 0 with a warning", code, out)
	}
	recs, _ := check.ReadRecords(data, "p")
	if len(recs) != 1 || len(kindRecord(t, recs[0], "test").Warnings) != 1 {
		t.Errorf("records = %+v; want one warning", recs)
	}
	if rec := recs[0]; rec.LoadStart != 1000 || rec.LoadEnd != 1000 {
		t.Errorf("load = %v/%v; want 1000/1000", rec.LoadStart, rec.LoadEnd)
	}
}

func TestCheck_RecordHasLoadAverages(t *testing.T) {
	repo, data := registeredCheckRepo(t, `{"test":"true"}`)
	n := 0
	load := func() float64 { n++; return float64(n) }
	if out, code := runCheckCmdLoad(t, load, repo, "--full", "--kind", "test"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	recs, _ := check.ReadRecords(data, "p")
	if len(recs) != 1 || recs[0].LoadStart != 1 || recs[0].LoadEnd != 2 {
		t.Errorf("records = %+v; want load 1 then 2", recs)
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
	t.Parallel()
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

func TestCheck_HangLimitKillsHungTestAndNamesIt(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds the gx binary")
	}
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "gx"), "github.com/elentok/gx").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	prependPath(t, bin)
	repo, _ := registeredCheckRepo(t, `{"test":{"full":"gx brief go-test -timeout=60s ./...","limits":{"hang":"2s"}}}`)
	testutil.WriteFile(t, repo, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport \"testing\"\n\nfunc TestStuck(t *testing.T) { select {} }\n")
	testutil.CommitAll(t, repo, "module")

	start := time.Now()
	out, code := runCheckCmd(t, repo, "--full", "--kind", "test")
	if code != 1 || !strings.Contains(logOf(t, out, "test"), "TestStuck") {
		t.Fatalf("exit %d, want 1 naming TestStuck\n%s", code, out)
	}
	if elapsed := time.Since(start); elapsed > 40*time.Second {
		t.Errorf("took %s, want far below the 60s package timeout", elapsed)
	}
}

// slowTestRepo has one Go test that sleeps 1.2s, checked under checksJSON.
func slowTestRepo(t *testing.T, checksJSON string) (repo, data string) {
	t.Helper()
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "gx"), "github.com/elentok/gx").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	prependPath(t, bin)
	repo, data = registeredCheckRepo(t, checksJSON)
	testutil.WriteFile(t, repo, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlow(t *testing.T) { time.Sleep(1200 * time.Millisecond) }\n")
	testutil.CommitAll(t, repo, "module")
	return repo, data
}

// flakySlowRepo has one Go test that sleeps 1.5s when the file `state` does not
// exist yet (and creates it) and sleeps 0.9s afterwards, under the 1s slow
// limit but long enough to show in the kind's time if it were counted; with
// stayslow it always sleeps 1.5s.
func flakySlowRepo(t *testing.T, stayslow bool) (repo, data string) {
	t.Helper()
	const brief = `gx brief go-test ./...`
	repo, data = slowTestRepo(t, `{"test":{"full":"`+brief+`","limits":{"slow":"1s","suite":"1m"}}}`)
	state := filepath.Join(t.TempDir(), "state")
	body := "_, err := os.Stat(" + strconv.Quote(state) + ")\n\tif err != nil {\n\t\tos.WriteFile(" + strconv.Quote(state) + ", nil, 0o644)\n\t\ttime.Sleep(1500 * time.Millisecond)\n\t} else {\n\t\ttime.Sleep(900 * time.Millisecond)\n\t}"
	if stayslow {
		body = "time.Sleep(1500 * time.Millisecond)"
	}
	testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport (\n\t\"os\"\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlow(t *testing.T) {\n\t"+body+"\n\t_ = os.Stat\n}\n")
	testutil.CommitAll(t, repo, "flaky")
	return repo, data
}

func TestCheck_SlowTestRerunUnderHighLoad(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds the gx binary")
	}
	high := func() float64 { return 1000 }
	t.Run("passes alone: warns, exits 0, record keeps the warning", func(t *testing.T) {
		repo, data := flakySlowRepo(t, false)
		out, code := runCheckCmdLoad(t, high, repo, "--full", "--kind", "test")
		if code != 0 || !strings.Contains(out, "warning:") || !strings.Contains(out, "example.com/tiny.TestSlow") {
			t.Fatalf("exit %d; want 0 with a warning naming the test\n%s", code, out)
		}
		recs, _ := check.ReadRecords(data, "p")
		k := kindRecord(t, recs[0], "test")
		if len(recs) != 1 || len(k.Warnings) != 1 || len(k.Failed) != 0 {
			t.Errorf("record = %+v; want one warning and no failures", recs)
		}
		lines, _ := brief.ReadIndex(data)
		var first, rerun int64
		reruns := 0
		for _, l := range lines {
			switch {
			case l.CheckKind == "test-rerun" && slices.Equal(l.Args, []string{"./...", "-run", "^TestSlow$"}):
				reruns++
				rerun = l.DurationMS
			case l.CheckKind == "test":
				first = l.DurationMS
			}
		}
		if reruns != 1 {
			t.Fatalf("index has %d tagged re-runs, want 1: %+v", reruns, lines)
		}
		// The kind's time is the first run plus the check's own overhead, which is
		// less than the re-run would add.
		if over := k.DurationMS - first; over >= rerun {
			t.Errorf("kind took %d ms, first run %d ms, re-run %d ms: the re-run is in the kind's time", k.DurationMS, first, rerun)
		}
	})
	t.Run("stays slow: exits 1 and names the test", func(t *testing.T) {
		repo, _ := flakySlowRepo(t, true)
		out, code := runCheckCmdLoad(t, high, repo, "--full", "--kind", "test")
		if code != 1 || !strings.Contains(out, "example.com/tiny.TestSlow") {
			t.Fatalf("exit %d; want 1 naming the test\n%s", code, out)
		}
	})
	t.Run("low load: exits 1 with no re-run", func(t *testing.T) {
		repo, data := flakySlowRepo(t, false)
		out, code := runCheckCmdLoad(t, func() float64 { return 0 }, repo, "--full", "--kind", "test")
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		lines, _ := brief.ReadIndex(data)
		for _, l := range lines {
			if l.CheckKind == "test-rerun" {
				t.Errorf("low load re-ran a test: %+v", l)
			}
		}
	})
}

// TestCheck_SlowTestRerunNodeTools stubs vitest and playwright to be slow on
// the first run only, and checks the re-run is narrowed to the one test.
func TestCheck_SlowTestRerunNodeTools(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds the gx binary")
	}
	high := func() float64 { return 1000 }
	for _, c := range []struct {
		tool, name, report string
		want               []string
	}{
		{
			"vitest", "a.test.ts.suite slow",
			`{"numTotalTestSuites":1,"numTotalTests":1,"testResults":[{"name":"a.test.ts","status":"passed","assertionResults":[{"fullName":"suite slow","status":"passed","duration":DUR}]}]}`,
			[]string{"-t", "^suite slow$"},
		},
		{
			"playwright", "a.e2e.ts.7. slow",
			`{"suites":[{"title":"a.e2e.ts","file":"a.e2e.ts","specs":[{"title":"7. slow","file":"a.e2e.ts","tests":[{"results":[{"duration":DUR}]}]}]}]}`,
			[]string{"-g", `› 7\. slow$`},
		},
	} {
		t.Run(c.tool, func(t *testing.T) {
			bin := t.TempDir()
			if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "gx"), "github.com/elentok/gx").CombinedOutput(); err != nil {
				t.Fatalf("go build: %v\n%s", err, out)
			}
			prependPath(t, bin)
			repo, data := registeredCheckRepo(t, `{"test":{"full":"gx brief -C webui `+c.tool+`","limits":{"slow":{"`+c.tool+`":"1s"},"suite":"1m"}}}`)
			testutil.WriteFile(t, repo, "webui/e2e/playwright.config.ts", "")
			testutil.WriteFile(t, repo, "webui/dist/index.html", "")
			testutil.CommitAll(t, repo, "web")
			// The first run reports 1500 ms and any later run 900 ms. Playwright
			// writes its report to the file named by PLAYWRIGHT_JSON_OUTPUT_NAME;
			// vitest prints it.
			state := filepath.Join(t.TempDir(), "state")
			slow, fast := strings.ReplaceAll(c.report, "DUR", "1500"), strings.ReplaceAll(c.report, "DUR", "900")
			script := "#!/bin/sh\nif [ -e '" + state + "' ]; then R='" + fast + "'; else R='" + slow + "'; : > '" + state + "'; fi\n" +
				"if [ -n \"$PLAYWRIGHT_JSON_OUTPUT_NAME\" ]; then printf '%s' \"$R\" > \"$PLAYWRIGHT_JSON_OUTPUT_NAME\"; else printf '%s' \"$R\"; fi\n"
			nodeBin := filepath.Join(repo, "webui", "node_modules", ".bin")
			testutil.Mkdir(t, nodeBin)
			testutil.WriteFile(t, nodeBin, c.tool, script)
			if err := os.Chmod(filepath.Join(nodeBin, c.tool), 0o755); err != nil {
				t.Fatal(err)
			}

			out, code := runCheckCmdLoad(t, high, repo, "--full", "--kind", "test")
			lines, _ := brief.ReadIndex(data)
			if code != 0 || !strings.Contains(out, "warning:") {
				t.Fatalf("exit %d; want 0 with a warning\n%s\n%+v", code, out, lines)
			}
			reruns := 0
			for _, l := range lines {
				if l.CheckKind == "test-rerun" && slices.Equal(l.Args, c.want) {
					reruns++
				}
			}
			if reruns != 1 {
				t.Fatalf("index has %d re-runs with args %v, want 1: %+v", reruns, c.want, lines)
			}
		})
	}
}

func TestCheck_SlowLimit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds the gx binary")
	}
	const brief = `gx brief go-test ./...`
	cases := []struct {
		name, test string
		args       []string
		want       int
	}{
		{"scalar full", `{"full":"` + brief + `","changed":"` + brief + `","limits":{"slow":"1s"}}`, []string{"--full"}, 1},
		{"scalar changed", `{"full":"` + brief + `","changed":"` + brief + `","limits":{"slow":"1s"}}`, nil, 1},
		{"map go", `{"full":"` + brief + `","limits":{"slow":{"go":"1s"}}}`, []string{"--full"}, 1},
		{"map other tool", `{"full":"` + brief + `","limits":{"slow":{"playwright":"1s","vitest":"1s"}}}`, []string{"--full"}, 0},
		{"under limit", `{"full":"` + brief + `","limits":{"slow":"20s"}}`, []string{"--full"}, 0},
		{"no brief", `{"full":"sleep 2","limits":{"slow":"1s"}}`, []string{"--full"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo, data := slowTestRepo(t, `{"test":`+c.test+`}`)
			if c.args == nil {
				testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlow(t *testing.T) { time.Sleep(1300 * time.Millisecond) }\n")
			}
			out, code := runCheckCmd(t, repo, append(c.args, "--kind", "test")...)
			if code != c.want {
				t.Fatalf("exit %d, want %d\n%s", code, c.want, out)
			}
			if c.want == 0 {
				return
			}
			if !strings.Contains(out, "example.com/tiny.TestSlow") || !strings.Contains(out, "over the 1s limit") {
				t.Errorf("output lacks test name and time:\n%s", out)
			}
			recs, _ := check.ReadRecords(data, "p")
			if len(recs) != 1 {
				t.Fatalf("records = %+v", recs)
			}
			if k := kindRecord(t, recs[0], "test"); !reflect.DeepEqual(k.Failed, []string{"example.com/tiny.TestSlow"}) {
				t.Errorf("failed = %v", k.Failed)
			}
		})
	}
}
