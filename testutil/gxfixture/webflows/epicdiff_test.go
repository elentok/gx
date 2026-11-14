package webflows

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

const (
	diffMapAddr  = "demo:" + spec.EpicDiffMap
	diffEpicAddr = "demo:" + spec.EpicDiffEpic
)

// repoGit runs git in the fixture's repo and returns trimmed stdout.
func (r *running) repoGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", r.Repo()}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// rangeFields reads range_base and range_tip of an epic from `gx tickets show`.
func (r *running) rangeFields(t *testing.T, addr string) (base, tip string) {
	t.Helper()
	out := r.Gx(t, "tickets", "show", addr)
	field := func(name string) string {
		m := regexp.MustCompile(`(?m)^\s*` + name + `:?\s+(\S+)\s*$`).FindStringSubmatch(out)
		if m == nil {
			return ""
		}
		return m[1]
	}
	return field("range_base"), field("range_tip")
}

func (r *running) epicDetail(t *testing.T, addr string) wire.EpicDetail {
	t.Helper()
	var d wire.EpicDetail
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "detail", addr, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestEpicDiffScenario_UnknownShapeListsTheKnownOnes(t *testing.T) {
	t.Parallel()
	out, err := exec.Command(fixturetest.FixtureBin, "--gx", fixturetest.GxBin, "--fakeclaude", fixturetest.FakeBin, "--shape", "nope", "epic-diff").CombinedOutput()
	if err == nil {
		t.Fatalf("--shape nope started:\n%s", out)
	}
	for _, shape := range spec.EpicDiffShapes {
		if !strings.Contains(string(out), shape) {
			t.Errorf("error does not list %q:\n%s", shape, out)
		}
	}
}

func TestEpicDiffScenario_Merged(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-diff", nil, "--shape", "merged")
	base, tip := r.rangeFields(t, diffEpicAddr)
	if base == "" || tip == "" {
		t.Fatalf("range fields = %q, %q; want both stamped", base, tip)
	}
	if n, _ := strconv.Atoi(r.repoGit(t, "rev-list", "--count", base+".."+tip)); n != 15 {
		t.Errorf("%d commits in the range, want 15", n)
	}
	log := r.repoGit(t, "log", "--format=%s%n%b%x00", base+".."+tip)
	for want, n := range map[string]int{
		"Ralph-Loop-Ticket: demo:" + spec.EpicDiffEpic + "/01": 5,
		"Ralph-Loop-Ticket: " + spec.EpicDiffEpic + "/02":      4,
		"Ralph-Loop-Ticket: demo:" + spec.EpicDiffEpic + "/03": 1,
	} {
		if got := strings.Count(log, want); got != n {
			t.Errorf("%d commits with %q, want %d", got, want, n)
		}
	}
	if got := strings.Count(log, "Ralph-Loop-Ticket:"); got != 10 {
		t.Errorf("%d stamped commits, want 10 (the rest are Other commits)", got)
	}
	if n := len(strings.Split(r.repoGit(t, "log", "--merges", "--format=%s", base+".."+tip), "\n")); n != 1 {
		t.Errorf("%d merge commits, want 1", n)
	}
	if got := r.repoGit(t, "log", "--merges", "--format=%s", base+".."+tip); got != "Merge branch '"+spec.EpicDiffMap+"-design' into "+spec.EpicDiffEpic {
		t.Errorf("merge subject = %q", got)
	}
	if !strings.Contains(log, spec.EpicDiffDesignSubject) {
		t.Errorf("no design commit %q", spec.EpicDiffDesignSubject)
	}
	status := r.repoGit(t, "diff", "--name-status", "-M", base, tip)
	for _, want := range []string{
		"R100\t" + spec.EpicDiffRenamedOld + "\t" + spec.EpicDiffRenamedTo,
		"A\t" + spec.EpicDiffPNG,
		"A\t" + spec.EpicDiffBigFile,
		"M\t" + spec.EpicDiffLockFile,
		"M\t" + spec.EpicDiffLongFile,
	} {
		if !strings.Contains(status, want) {
			t.Errorf("diff has no %q:\n%s", want, status)
		}
	}
	if got := r.repoGit(t, "show", tip+":"+spec.EpicDiffBigFile); strings.Count(got, "\n") != spec.EpicDiffBigLines-1 {
		t.Errorf("big file has %d lines, want %d", strings.Count(got, "\n")+1, spec.EpicDiffBigLines)
	}

	if got := r.epicDetail(t, diffEpicAddr); !got.ShowsViewDiff || got.DiffBase != base || got.DiffTip != tip {
		t.Errorf("epic detail = shows %v base %q tip %q, want the stamped range", got.ShowsViewDiff, got.DiffBase, got.DiffTip)
	}
	if b, tp := r.rangeFields(t, diffMapAddr); b != "" || tp != "" {
		t.Errorf("map range = %q, %q; want none", b, tp)
	}
	// gx tickets show does not address archived epics, so read the file.
	archived, err := os.ReadFile(filepath.Join(r.Root(), "data", "gx", "tickets", "demo", ".archive", spec.EpicDiffArchived, "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"merged_at: ", "range_base: ", "range_tip: "} {
		if !strings.Contains(string(archived), key) {
			t.Errorf("archived epic has no %q:\n%s", key, archived)
		}
	}
	// View diff shows on a done ticket with commits, never on the commitless one.
	for id, want := range map[string]bool{"01": true, "02": true, "03": true, "04": false} {
		if got := r.Detail(t, diffEpicAddr+"/"+id).ShowsViewDiff; got != want {
			t.Errorf("ticket %s shows View diff = %v, want %v", id, got, want)
		}
	}
}

func TestEpicDiffScenario_StampedRangeShapes(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"unstamped", "empty", "missing-sha"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "epic-diff", nil, "--shape", shape)
			base, tip := r.rangeFields(t, diffEpicAddr)
			switch shape {
			case "unstamped":
				if base != "" || tip != "" {
					t.Errorf("range = %q, %q; want none", base, tip)
				}
				if r.repoGit(t, "branch", "--list", spec.EpicDiffEpic) != "" {
					t.Errorf("branch %s exists, want it gone", spec.EpicDiffEpic)
				}
			case "empty":
				if base == "" || base != tip {
					t.Errorf("range = %q, %q; want equal and set", base, tip)
				}
			case "missing-sha":
				if tip != spec.EpicDiffMissingTip {
					t.Errorf("tip = %q, want %q", tip, spec.EpicDiffMissingTip)
				}
				if err := exec.Command("git", "-C", r.Repo(), "cat-file", "-e", tip).Run(); err == nil {
					t.Errorf("tip %s is an object, want it absent", tip)
				}
			}
		})
	}
}

func TestEpicDiffScenario_Running(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-diff", nil, "--shape", "running")
	ahead := func(base, branch string) int {
		n, _ := strconv.Atoi(r.repoGit(t, "rev-list", "--count", base+".."+branch))
		return n
	}
	for _, w := range []struct {
		epic, base string
		commits    int
	}{
		{spec.EpicDiffAhead, "main", 2},
		{spec.EpicDiffStacked, spec.EpicDiffAhead, 1},
		{spec.EpicDiffOnTrunk, "main", 1},
		{spec.EpicDiffExplicit, spec.EpicDiffReleaseBranch, 1},
		{spec.EpicDiffQuiet, "main", 0},
	} {
		if got := ahead(w.base, w.epic); got != w.commits {
			t.Errorf("%s: %d commits past %s, want %d", w.epic, got, w.base, w.commits)
		}
		d := r.epicDetail(t, "demo:"+w.epic)
		if d.ShowsViewDiff != (w.commits > 0) || d.HasCommitsAhead != (w.commits > 0) {
			t.Errorf("%s: shows %v, ahead %v, want both %v", w.epic, d.ShowsViewDiff, d.HasCommitsAhead, w.commits > 0)
		}
		if w.commits > 0 && (d.DiffBase != w.base || d.DiffTip != w.epic) {
			t.Errorf("%s: diff %q..%q, want %q..%q", w.epic, d.DiffBase, d.DiffTip, w.base, w.epic)
		}
	}
	if base, tip := r.rangeFields(t, "demo:"+spec.EpicDiffAhead); base != "" || tip != "" {
		t.Errorf("unmerged epic has range %q, %q", base, tip)
	}
}
