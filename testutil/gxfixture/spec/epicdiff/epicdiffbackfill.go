package epicdiff

import (
	"fmt"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// The merged epics of the backfill shape: none has a stamped range, and the
// server's start-up backfill stamps the ones its rules accept.
const (
	// EpicDiffBackfillSecond is the second project, with a repo of its own.
	EpicDiffBackfillSecond = "second"

	// Exact matches: all of the epic's trailer commits lie in one reflog entry.
	EpicDiffExactA = "bf-exact-a-impl" // two commits
	EpicDiffExactB = "bf-exact-b-impl" // one commit
	// EpicDiffExactMap is the map of EpicDiffExactA: merged_at copied from it, no range.
	EpicDiffExactMap = "bf-exact-a"
	// EpicDiffSecondExact is an exact match in the second project's repo.
	EpicDiffSecondExact = "bf-second-exact-impl"
	// EpicDiffPullX and EpicDiffPullY came in one reflog entry, as by one `git pull`: both are refused.
	EpicDiffPullX = "bf-pull-x-impl"
	EpicDiffPullY = "bf-pull-y-impl"
	// EpicDiffSpread is ambiguous: its commits lie in two reflog entries.
	EpicDiffSpread = "bf-spread-impl"
	// EpicDiffTwice was merged into trunk twice: two entries each hold all its commits.
	EpicDiffTwice = "bf-twice-impl"
	// EpicDiffNoLog has commits on trunk and no run log.
	EpicDiffNoLog = "bf-nolog-impl"
	// EpicDiffOffTrunk has a run log with a pick, and its commits are on a branch only.
	EpicDiffOffTrunk = "bf-offtrunk-impl"
	// EpicDiffNoCommits has a run log with no pick: it gets an empty range at trunk's tip.
	EpicDiffNoCommits = "bf-nocommit-impl"
	// EpicDiffSecondNoCommits is the same in the second project.
	EpicDiffSecondNoCommits = "bf-second-nocommit-impl"
)

// EpicDiffBackfillStamped lists the epics (project, name) that get a range.
var EpicDiffBackfillStamped = [][2]string{
	{project, EpicDiffExactA}, {project, EpicDiffExactB}, {EpicDiffBackfillSecond, EpicDiffSecondExact},
}

// EpicDiffBackfillRefused lists the epics (project, name) that get none.
var EpicDiffBackfillRefused = [][2]string{
	{project, EpicDiffExactMap}, {project, EpicDiffPullX}, {project, EpicDiffPullY}, {project, EpicDiffSpread},
	{project, EpicDiffTwice}, {project, EpicDiffNoLog}, {project, EpicDiffOffTrunk},
}

// EpicDiffBackfillEmpty lists the epics (project, name) that get an empty range at trunk's tip.
var EpicDiffBackfillEmpty = [][2]string{{project, EpicDiffNoCommits}, {EpicDiffBackfillSecond, EpicDiffSecondNoCommits}}

// EpicDiffBackfillSubject is the headline of commit n of an epic of the backfill shape.
func EpicDiffBackfillSubject(epic string, n int) string { return fmt.Sprintf("%s: commit %d", epic, n) }

func epicDiffBackfillScenario() scenario {
	sc := scenario{
		EpicMeta: map[string]string{},
		// The default left the project's applied list empty, so the server runs every
		// step at start-up, the range backfill among them.
		Migrate:  true,
		OwnRepos: []string{EpicDiffBackfillSecond},
		Recovered: map[string]string{
			project + ":" + EpicDiffNoCommits:                      string(events.AllPaused),
			EpicDiffBackfillSecond + ":" + EpicDiffSecondNoCommits: string(events.AllPaused),
		},
	}
	add := func(proj, name string, picked bool) {
		sc.Epics = append(sc.Epics, epic{Project: proj, Name: name})
		sc.EpicMeta[proj+":"+name] = epicDiffMergedMeta("")
		if picked {
			sc.PickedEpics = append(sc.PickedEpics, proj+":"+name)
		}
	}
	for _, name := range []string{EpicDiffExactA, EpicDiffExactB, EpicDiffPullX, EpicDiffPullY, EpicDiffSpread, EpicDiffTwice, EpicDiffOffTrunk} {
		add(project, name, true)
	}
	add(project, EpicDiffNoLog, false)
	add(project, EpicDiffNoCommits, false)
	add(EpicDiffBackfillSecond, EpicDiffSecondExact, true)
	add(EpicDiffBackfillSecond, EpicDiffSecondNoCommits, false)
	sc.Epics = append(sc.Epics, epic{Project: project, Name: EpicDiffExactMap})
	sc.EpicMeta[project+":"+EpicDiffExactMap] = epicDiffMergedMeta("kind: map\n")

	sc.GitSetup = func(git spec.Git, repo string) error {
		s := &epicDiffGit{git: git, repo: repo}
		w := backfillWork{s: s, project: project}
		w.merged(EpicDiffExactA, 2)
		w.merged(EpicDiffExactB, 1)
		// One entry holding both epics.
		s.run("checkout", "-q", "-b", "pull", "main")
		w.commits(EpicDiffPullX, 1, 1)
		w.commits(EpicDiffPullY, 1, 1)
		w.fastForward("pull")
		s.run("branch", "-q", "-D", "pull")
		// The second commit lands in a later entry.
		s.run("checkout", "-q", "-b", EpicDiffSpread, "main")
		w.commits(EpicDiffSpread, 1, 1)
		w.fastForward(EpicDiffSpread)
		s.run("checkout", "-q", EpicDiffSpread)
		w.commits(EpicDiffSpread, 2, 2)
		w.fastForward(EpicDiffSpread)
		s.run("branch", "-q", "-D", EpicDiffSpread)
		// Merged, taken back and merged again.
		s.run("checkout", "-q", "-b", EpicDiffTwice, "main")
		w.commits(EpicDiffTwice, 1, 1)
		w.fastForward(EpicDiffTwice)
		s.run("reset", "-q", "--hard", "HEAD~1")
		w.fastForward(EpicDiffTwice)
		s.run("branch", "-q", "-D", EpicDiffTwice)
		w.merged(EpicDiffNoLog, 1)
		// Not merged: its branch stays.
		s.run("checkout", "-q", "-b", EpicDiffOffTrunk, "main")
		w.commits(EpicDiffOffTrunk, 1, 1)
		s.run("checkout", "-q", "main")
		return s.err
	}
	sc.OwnGitSetup = map[string]func(git spec.Git, repo string) error{
		EpicDiffBackfillSecond: func(git spec.Git, repo string) error {
			s := &epicDiffGit{git: git, repo: repo}
			(backfillWork{s: s, project: EpicDiffBackfillSecond}).merged(EpicDiffSecondExact, 1)
			return s.err
		},
	}
	return sc
}

// backfillWork makes the commits of one project's repo.
type backfillWork struct {
	s       *epicDiffGit
	project string
}

// commits adds the commits from..to of epic to the current branch.
func (w backfillWork) commits(epic string, from, to int) {
	for i := from; i <= to; i++ {
		w.s.write(epic+"/"+fmt.Sprint(i)+".txt", numbered(epic, i))
		w.s.commit(EpicDiffBackfillSubject(epic, i), w.project+":"+merge.TicketTrailerValue(epic, fmt.Sprintf("%02d", i)))
	}
}

// fastForward moves trunk to branch with one real fast-forward merge, which
// writes one "old new" line to trunk's reflog.
func (w backfillWork) fastForward(branch string) {
	w.s.run("checkout", "-q", "main")
	w.s.run("merge", "--ff-only", "-q", branch)
}

// merged commits n commits of epic on a branch and fast-forwards trunk to it.
func (w backfillWork) merged(epic string, n int) {
	w.s.run("checkout", "-q", "-b", epic, "main")
	w.commits(epic, 1, n)
	w.fastForward(epic)
	w.s.run("branch", "-q", "-D", epic)
}
