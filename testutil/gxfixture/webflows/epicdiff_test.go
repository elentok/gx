package webflows

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
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
	return gitIn(t, r.Repo(), args...)
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// hasObject says whether sha is an object of the repo in dir.
func hasObject(dir, sha string) bool {
	return exec.Command("git", "-C", dir, "cat-file", "-e", sha).Run() == nil
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

// waitStamped polls until every addr has a stamped range, then returns them.
func (r *running) waitStamped(t *testing.T, addrs ...string) map[string][2]string {
	t.Helper()
	got := map[string][2]string{}
	if !testutil.Poll(60*time.Second, 200*time.Millisecond, func() bool {
		for _, a := range addrs {
			base, tip := r.rangeFields(t, a)
			if base == "" || tip == "" {
				return false
			}
			got[a] = [2]string{base, tip}
		}
		return true
	}) {
		t.Fatalf("not all of %v got a range; have %v", addrs, got)
	}
	return got
}

func (r *running) mergedAt(t *testing.T, epic string) bool {
	t.Helper()
	var list wire.EpicList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	for _, row := range list.Epics {
		if row.Name == epic {
			return !row.MergedAt.IsZero()
		}
	}
	t.Fatalf("no epic %s", epic)
	return false
}

func TestEpicDiffScenario_PR(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-diff", nil, "--shape", "pr")
	addr := func(epic string) string { return "demo:" + epic }
	stamped := r.waitStamped(t, addr(spec.EpicDiffPRMerge), addr(spec.EpicDiffPRSquash), addr(spec.EpicDiffPRRebase))
	if !testutil.Poll(60*time.Second, 200*time.Millisecond, func() bool { return r.mergedAt(t, spec.EpicDiffPRReworded) }) {
		t.Fatalf("%s was not marked merged", spec.EpicDiffPRReworded)
	}
	parents := func(rev string) []string {
		return strings.Fields(r.repoGit(t, "rev-list", "--parents", "-n", "1", rev))[1:]
	}

	// Merge commit: base is the first parent, tip the merge commit.
	m := stamped[addr(spec.EpicDiffPRMerge)]
	if p := parents(m[1]); len(p) != 2 || p[0] != m[0] {
		t.Errorf("merge epic: tip %s has parents %v, want two with base %s first", m[1], p, m[0])
	}

	// Squash: one squash commit whose body holds several trailer lines.
	sq := stamped[addr(spec.EpicDiffPRSquash)]
	if p := parents(sq[1]); len(p) != 1 || p[0] != sq[0] {
		t.Errorf("squash epic: tip %s has parents %v, want base %s alone", sq[1], p, sq[0])
	}
	if got := r.repoGit(t, "log", "-1", "--format=%s", sq[1]); got != spec.EpicDiffSquashSubject() {
		t.Errorf("squash subject = %q, want %q", got, spec.EpicDiffSquashSubject())
	}
	body := r.repoGit(t, "log", "-1", "--format=%b", sq[1])
	if got := strings.Count(body, "Ralph-Loop-Ticket:"); got != spec.EpicDiffSquashLinked {
		t.Errorf("squash body has %d trailer lines, want %d:\n%s", got, spec.EpicDiffSquashLinked, body)
	}
	if last := "/0" + strconv.Itoa(spec.EpicDiffSquashTickets); strings.Contains(r.repoGit(t, "log", "--format=%b", sq[0]+".."+sq[1]), last) {
		t.Errorf("a commit of the range names ticket %s, which should have no commits there", last)
	}
	if got := r.Detail(t, addr(spec.EpicDiffPRSquash)+"/03").Status; got != "done" {
		t.Errorf("squash epic's third ticket is %q, want done", got)
	}

	// Rebase: the PR's commits re-created on trunk with the same headlines.
	rb := stamped[addr(spec.EpicDiffPRRebase)]
	want := spec.EpicDiffPRStep(spec.EpicDiffPRRebase, 1) + "\n" + spec.EpicDiffPRStep(spec.EpicDiffPRRebase, 2)
	if got := r.repoGit(t, "log", "--reverse", "--format=%s", rb[0]+".."+rb[1]); got != want {
		t.Errorf("rebase range headlines = %q, want %q", got, want)
	}
	if branch := r.repoGit(t, "log", "--reverse", "--format=%s", "-2", spec.EpicDiffPRRebase, "--"); branch != want {
		t.Errorf("branch headlines = %q, want %q", branch, want)
	}
	if r.repoGit(t, "rev-parse", rb[1]) == r.repoGit(t, "rev-parse", "refs/heads/"+spec.EpicDiffPRRebase) {
		t.Error("rebase tip is the branch tip, want re-created commits")
	}

	// Other headlines: merged, no range.
	if base, tip := r.rangeFields(t, addr(spec.EpicDiffPRReworded)); base != "" || tip != "" {
		t.Errorf("reworded epic has range %q, %q; want none", base, tip)
	}

	// The stacked pair: stamped in the seed, objects present and gone.
	kb, kt := r.rangeFields(t, addr(spec.EpicDiffStackedKept))
	if kb == "" || !hasObject(r.Repo(), kb) || !hasObject(r.Repo(), kt) {
		t.Errorf("kept epic range %q, %q; want stamped objects that exist", kb, kt)
	}
	gb, gt := r.rangeFields(t, addr(spec.EpicDiffStackedGone))
	if gb != spec.EpicDiffGoneBase || gt != spec.EpicDiffMissingTip || hasObject(r.Repo(), gb) || hasObject(r.Repo(), gt) {
		t.Errorf("gone epic range %q, %q; want the stamped SHAs, both absent", gb, gt)
	}

	// A second read changes nothing.
	for a, want := range stamped {
		if base, tip := r.rangeFields(t, a); [2]string{base, tip} != want {
			t.Errorf("%s: range changed to %q, %q, was %v", a, base, tip, want)
		}
	}
}

// reflogLines reads trunk's reflog of the repo in dir as "old new" pairs.
func reflogLines(t *testing.T, dir string) [][2]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".git", "logs", "refs", "heads", "main"))
	if err != nil {
		t.Fatal(err)
	}
	var lines [][2]string
	for l := range strings.SplitSeq(string(data), "\n") {
		// The branch's creation has an all-zero old SHA and no range.
		if f := strings.Fields(l); len(f) >= 2 && strings.Trim(f[0], "0") != "" {
			lines = append(lines, [2]string{f[0], f[1]})
		}
	}
	return lines
}

func TestEpicDiffScenario_Backfill(t *testing.T) {
	t.Parallel()
	r := startWith(t, "epic-diff", nil, "--shape", "backfill")
	second := filepath.Join(r.Root(), spec.EpicDiffBackfillSecond+"-repo")
	dirOf := func(project string) string {
		if project == spec.EpicDiffBackfillSecond {
			return second
		}
		return r.Repo()
	}
	addr := func(p [2]string) string { return p[0] + ":" + p[1] }
	var wait []string
	for _, p := range append(slices.Clone(spec.EpicDiffBackfillStamped), spec.EpicDiffBackfillEmpty...) {
		wait = append(wait, addr(p))
	}
	got := r.waitStamped(t, wait...)

	// Exact matches: the range is one reflog entry, holding the epic's commits.
	for _, c := range []struct {
		p       [2]string
		subject []string
	}{
		{spec.EpicDiffBackfillStamped[0], []string{spec.EpicDiffBackfillSubject(spec.EpicDiffExactA, 1), spec.EpicDiffBackfillSubject(spec.EpicDiffExactA, 2)}},
		{spec.EpicDiffBackfillStamped[1], []string{spec.EpicDiffBackfillSubject(spec.EpicDiffExactB, 1)}},
		{spec.EpicDiffBackfillStamped[2], []string{spec.EpicDiffBackfillSubject(spec.EpicDiffSecondExact, 1)}},
	} {
		dir, rng := dirOf(c.p[0]), got[addr(c.p)]
		if headlines := gitIn(t, dir, "log", "--reverse", "--format=%s", rng[0]+".."+rng[1]); headlines != strings.Join(c.subject, "\n") {
			t.Errorf("%s: range holds %q, want %q", c.p[1], headlines, c.subject)
		}
		if !slices.Contains(reflogLines(t, dir), rng) {
			t.Errorf("%s: %v is not a line of trunk's reflog", c.p[1], rng)
		}
	}
	// No commits: an empty range at trunk's tip.
	for _, p := range spec.EpicDiffBackfillEmpty {
		rng, tip := got[addr(p)], gitIn(t, dirOf(p[0]), "rev-parse", "main")
		if rng[0] != tip || rng[1] != tip {
			t.Errorf("%s: range %v, want base and tip %s", p[1], rng, tip)
		}
	}
	// Refused: nothing stamped.
	for _, p := range spec.EpicDiffBackfillRefused {
		if base, tip := r.rangeFields(t, addr(p)); base != "" || tip != "" {
			t.Errorf("%s: range %q, %q; want none", p[1], base, tip)
		}
	}

	// The reflog has the lines the refusals need.
	lines := reflogLines(t, r.Repo())
	holds := func(l [2]string, epic string) bool {
		return strings.Contains(r.repoGit(t, "log", "--format=%s", l[0]+".."+l[1]), epic+": ")
	}
	forward := func(epic string) (n int) {
		for _, l := range lines {
			if holds(l, epic) {
				n++
			}
		}
		return n
	}
	if n := forward(spec.EpicDiffTwice); n != 2 {
		t.Errorf("%d reflog lines hold %s, want 2 (merged twice)", n, spec.EpicDiffTwice)
	}
	if n := forward(spec.EpicDiffSpread); n != 2 {
		t.Errorf("%d reflog lines hold %s, want 2 (spread)", n, spec.EpicDiffSpread)
	}
	var pull [2]string
	for _, l := range lines {
		if holds(l, spec.EpicDiffPullX) {
			pull = l
		}
	}
	if !holds(pull, spec.EpicDiffPullX) || !holds(pull, spec.EpicDiffPullY) {
		t.Errorf("no single reflog line holds both %s and %s", spec.EpicDiffPullX, spec.EpicDiffPullY)
	}
	if r.repoGit(t, "branch", "--list", spec.EpicDiffOffTrunk) == "" {
		t.Errorf("branch %s is gone, want its commits off trunk", spec.EpicDiffOffTrunk)
	}

	// A second read changes nothing.
	for a, want := range got {
		if base, tip := r.rangeFields(t, a); [2]string{base, tip} != want {
			t.Errorf("%s: range changed to %q, %q, was %v", a, base, tip, want)
		}
	}
}
