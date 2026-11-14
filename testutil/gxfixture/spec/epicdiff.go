package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitpkg "github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/server/servertest"
)

// EpicDiffShapes are the --shape names of the epic-diff scenario; the first is the default.
var EpicDiffShapes = []string{"merged", "running", "unstamped", "empty", "missing-sha", "pr", "backfill"}

// Names the Diff page flows look at.
const (
	// EpicDiffMap is the map epic; EpicDiffEpic is its merged implementation epic.
	EpicDiffMap  = "widgets"
	EpicDiffEpic = EpicDiffMap + "-impl"
	// EpicDiffArchived is the archived merged epic with its own stamped range.
	EpicDiffArchived = "widgets-old-impl"
	// EpicDiffMissingTip is the well-formed SHA of the missing-sha shape that was never committed.
	EpicDiffMissingTip = "0123456789abcdef0123456789abcdef01234567"

	EpicDiffBigFile    = "src/gen/big.go"
	EpicDiffLongFile   = "src/long/long.go"
	EpicDiffPNG        = "assets/logo.png"
	EpicDiffLockFile   = "go.sum"
	EpicDiffRenamedTo  = "src/util/new_name.go"
	EpicDiffRenamedOld = "src/util/old_name.go"
	// EpicDiffDesignSubject starts with the map name, which makes it a design commit.
	EpicDiffDesignSubject = EpicDiffMap + ": chart the design"
	// EpicDiffBigLines is the length of EpicDiffBigFile.
	EpicDiffBigLines = 5000
)

// The unmerged epics of the running shape.
const (
	EpicDiffAhead    = "alpha-impl"   // unmerged, commits past trunk
	EpicDiffStacked  = "beta-impl"    // blocked by the unmerged alpha-impl: bases on its branch
	EpicDiffShipped  = "shipped-impl" // merged blocker
	EpicDiffOnTrunk  = "gamma-impl"   // blocked by the merged shipped-impl: bases on trunk
	EpicDiffExplicit = "custom-impl"  // base: release
	EpicDiffQuiet    = "quiet-impl"   // no commits past its base
	// EpicDiffReleaseBranch is the explicit base of EpicDiffExplicit.
	EpicDiffReleaseBranch = "release"
)

const epicDiffMergedAt = "2026-02-10T11:00:00Z"

// epicDiffMergedMeta is the frontmatter of a merged epic; extra adds its fields.
func epicDiffMergedMeta(extra string) string {
	return "status: open\ncompleted_at: 2026-02-10T10:00:00Z\nmerged_at: " + epicDiffMergedAt + "\n" + extra
}

// EpicDiffScenario is the epic-diff scenario in the given shape. The stored
// epics hold SHAs of the repo's own history, so GitSetup writes them into the
// meta once it has made the commits (seed reads the meta after GitSetup).
func EpicDiffScenario(shape string) (Scenario, bool) {
	sc := Scenario{EpicMeta: map[string]string{}}
	done := func(id, slug string) Ticket {
		return Ticket{ID: id, Slug: slug, Opts: servertest.TicketOpts{Status: "done", DoneAt: epicDiffMergedAt}}
	}
	mergedMeta := epicDiffMergedMeta
	mapMeta := mergedMeta("kind: map\n")
	switch shape {
	case "merged":
		sc.Epics = []Epic{
			{Project: project, Name: EpicDiffMap},
			{Project: project, Name: EpicDiffEpic, Tickets: []Ticket{
				done("01", "every-commit-stamped"), done("02", "legacy-trailer"), done("03", "last-commit-stamped"),
				{ID: "04", Slug: "commitless-check", Opts: servertest.TicketOpts{Status: "done", DoneAt: epicDiffMergedAt, Commitless: true}},
			}},
		}
		sc.Archived = []ArchivedEpic{{Project: project, Name: EpicDiffArchived, Tickets: []Ticket{done("01", "old-work")}}}
		sc.GitSetup = func(git Git, repo string) error {
			s := &epicDiffGit{git: git, repo: repo}
			oldBase, oldTip := s.archivedRange()
			base, tip := s.mergedRange()
			if s.err != nil {
				return s.err
			}
			sc.EpicMeta[project+":"+EpicDiffMap] = mapMeta
			sc.EpicMeta[project+":"+EpicDiffEpic] = mergedMeta("range_base: " + base + "\nrange_tip: " + tip + "\n")
			sc.Archived[0].Meta = mergedMeta("archived_at: 2026-02-11T10:00:00Z\nrange_base: " + oldBase + "\nrange_tip: " + oldTip + "\n")
			return nil
		}
	case "unstamped", "empty", "missing-sha":
		sc.Epics = []Epic{
			{Project: project, Name: EpicDiffMap},
			{Project: project, Name: EpicDiffEpic, Tickets: []Ticket{done("01", "only-ticket")}},
		}
		sc.GitSetup = func(git Git, repo string) error {
			trunk, err := gitpkg.RevParse(repo, "main")
			if err != nil {
				return err
			}
			var rng string
			switch shape {
			case "empty":
				rng = "range_base: " + trunk + "\nrange_tip: " + trunk + "\n"
			case "missing-sha":
				rng = "range_base: " + trunk + "\nrange_tip: " + EpicDiffMissingTip + "\n"
			}
			sc.EpicMeta[project+":"+EpicDiffMap] = mapMeta
			sc.EpicMeta[project+":"+EpicDiffEpic] = mergedMeta(rng)
			return nil
		}
	case "running":
		open := func(name string) Epic {
			return Epic{Project: project, Name: name, Tickets: []Ticket{{ID: "01", Slug: "work", Opts: servertest.TicketOpts{}}}}
		}
		sc.Epics = []Epic{open(EpicDiffAhead), open(EpicDiffStacked), {Project: project, Name: EpicDiffShipped}, open(EpicDiffOnTrunk), open(EpicDiffExplicit), open(EpicDiffQuiet)}
		// A derived stacked base needs the on-done landing policy; under the default one
		// a dependent always bases on trunk.
		sc.ProjectExtra = map[string]string{project: `,"landing":"on-done"`}
		sc.EpicMeta[project+":"+EpicDiffStacked] = "status: open\nblocked_by: [" + EpicDiffAhead + "]\n"
		sc.EpicMeta[project+":"+EpicDiffShipped] = mergedMeta("")
		sc.EpicMeta[project+":"+EpicDiffOnTrunk] = "status: open\nblocked_by: [" + EpicDiffShipped + "]\n"
		sc.EpicMeta[project+":"+EpicDiffExplicit] = "status: open\nbase: " + EpicDiffReleaseBranch + "\n"
		sc.GitSetup = func(git Git, repo string) error {
			s := &epicDiffGit{git: git, repo: repo}
			s.runningBranches()
			return s.err
		}
	case "pr":
		return epicDiffPRScenario(), true
	case "backfill":
		return epicDiffBackfillScenario(), true
	default:
		return Scenario{}, false
	}
	return sc, true
}

// epicDiffGit makes the scenario's commits; the first failure sticks and
// turns every later call into a no-op.
type epicDiffGit struct {
	git  Git
	repo string
	err  error
}

func (s *epicDiffGit) run(args ...string) {
	if s.err == nil {
		s.err = s.git(s.repo, args...)
	}
}

func (s *epicDiffGit) write(path string, data string) {
	if s.err != nil {
		return
	}
	full := filepath.Join(s.repo, path)
	if s.err = os.MkdirAll(filepath.Dir(full), 0o755); s.err == nil {
		s.err = os.WriteFile(full, []byte(data), 0o644)
	}
}

// commit stages everything and commits it with each trailer as a Ralph-Loop-Ticket line.
func (s *epicDiffGit) commit(subject string, ticketTrailers ...string) {
	s.run("add", "-A")
	args := []string{"commit", "-m", subject}
	if len(ticketTrailers) > 0 {
		// One paragraph: git reads trailers from the last one only.
		lines := make([]string, len(ticketTrailers))
		for i, v := range ticketTrailers {
			lines[i] = merge.TicketTrailerKey + ": " + v
		}
		args = append(args, "-m", strings.Join(lines, "\n"))
	}
	s.run(args...)
}

func (s *epicDiffGit) sha(ref string) string {
	if s.err != nil {
		return ""
	}
	sha, err := gitpkg.RevParse(s.repo, ref)
	s.err = err
	return sha
}

// numbered is n lines of text, "<prefix> <i>" each.
func numbered(prefix string, n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%s %d\n", prefix, i)
	}
	return b.String()
}

// archivedRange commits the two-commit range of the archived epic on trunk.
func (s *epicDiffGit) archivedRange() (base, tip string) {
	s.write("go.sum", numbered("sum", 10))
	s.write(EpicDiffRenamedOld, numbered("// util", 20))
	s.write(EpicDiffLongFile, numbered("// long", 200))
	for _, p := range []string{"src/server/handler.go", "src/server/routes.go", "src/web/app.tsx", "src/web/state.ts", "docs/guide.md"} {
		s.write(p, numbered("// "+filepath.Base(p), 30))
	}
	s.commit("Add project sources")
	base = s.sha("main")
	old := func(id string) string { return merge.TicketTrailerValue(EpicDiffArchived, id) }
	s.write("docs/old.md", "old\n")
	s.commit("Write the old notes", old("01"))
	s.write("docs/old.md", "old\nnotes\n")
	s.commit("Extend the old notes", old("01"))
	return base, s.sha("main")
}

// mergedRange commits the 15-commit Epic range of the merged shape on a branch,
// merges it into trunk by fast-forward and deletes the branches.
func (s *epicDiffGit) mergedRange() (base, tip string) {
	base = s.sha("main")
	a := "demo:" + merge.TicketTrailerValue(EpicDiffEpic, "01") // current, project-qualified form
	b := merge.TicketTrailerValue(EpicDiffEpic, "02")           // legacy form
	c := "demo:" + merge.TicketTrailerValue(EpicDiffEpic, "03")
	design := EpicDiffMap + "-design"

	s.run("branch", design, base)
	s.run("checkout", "-q", "-b", EpicDiffEpic, base)
	s.run("checkout", "-q", design)
	s.write("docs/design.md", "design\n")
	s.commit(EpicDiffDesignSubject)
	s.run("checkout", "-q", EpicDiffEpic)

	edit := func(path, subject string, trailers ...string) {
		s.write(path, numbered("// "+filepath.Base(path), 30)+"// edited by "+subject+"\n")
		s.commit(subject, trailers...)
	}
	// Ticket 03 has only its last commit stamped; ticket 01's commits are not adjacent.
	edit("src/server/handler.go", "Sketch the cache layer")
	edit("src/server/routes.go", "Wire the cache into routes")
	edit("src/web/app.tsx", "Show the widget list", a)
	edit("src/web/state.ts", "Keep the widget list in state", a)
	edit("docs/guide.md", "Fix a typo in the guide")
	edit("src/web/components/list.tsx", "Add the list component", a)
	edit("src/server/cache/cache.go", "Add the cache", b)
	s.run("mv", EpicDiffRenamedOld, EpicDiffRenamedTo)
	s.commit("Rename the util file", b)
	s.write(EpicDiffPNG, "\x89PNG\r\n\x1a\n\x00"+strings.Repeat("\x00", 100))
	s.commit("Add the logo", a)
	s.write(EpicDiffLockFile, numbered("sum", 10)+"sum new\n")
	s.commit("Update the sums", b)
	s.write(EpicDiffBigFile, numbered("// generated", EpicDiffBigLines))
	s.commit("Generate the big file", a)
	long := strings.Replace(numbered("// long", 200), "// long 100\n", "// long 100 changed\n", 1)
	s.write(EpicDiffLongFile, long)
	s.commit("Tweak the long file", b)
	edit("src/server/handler.go", "Finish the cache layer", c)
	s.run("merge", "--no-ff", "-q", design)

	tip = s.sha(EpicDiffEpic)
	s.run("checkout", "-q", "main")
	s.run("merge", "--ff-only", "-q", EpicDiffEpic)
	s.run("branch", "-q", "-D", EpicDiffEpic, design)
	return base, tip
}

// runningBranches makes the unmerged epics' branches of the running shape.
func (s *epicDiffGit) runningBranches() {
	branch := func(name, from, file string, commits int) {
		s.run("checkout", "-q", "-b", name, from)
		for i := range commits {
			s.write(file, numbered("// "+name, i+1))
			s.commit(fmt.Sprintf("%s: step %d", name, i+1))
		}
	}
	branch(EpicDiffAhead, "main", "alpha.txt", 2)
	branch(EpicDiffStacked, EpicDiffAhead, "beta.txt", 1)
	branch(EpicDiffOnTrunk, "main", "gamma.txt", 1)
	branch(EpicDiffReleaseBranch, "main", "release.txt", 1)
	branch(EpicDiffExplicit, EpicDiffReleaseBranch, "custom.txt", 1)
	branch(EpicDiffQuiet, "main", "", 0)
	s.run("checkout", "-q", "main")
}
