package spec

import (
	"fmt"

	"github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/ghfake"
)

// The epics of the pr shape. The first four are merged on origin only: the
// server's trunk sync brings the merge in and stamps the range.
const (
	EpicDiffPRMerge    = "pr-merge-impl"    // merge commit: (first parent, merge commit)
	EpicDiffPRSquash   = "pr-squash-impl"   // squash commit whose body holds several trailer lines
	EpicDiffPRRebase   = "pr-rebase-impl"   // commits re-created on trunk with the same headlines
	EpicDiffPRReworded = "pr-reworded-impl" // rebased with other headlines: no range
	// Stacked on the squash-merged blocker, with the stamp written in the seed.
	EpicDiffStackedKept = "stack-kept-impl" // its stamped objects exist
	EpicDiffStackedGone = "stack-gone-impl" // its stamped SHAs were never committed

	// EpicDiffSquashLinked are the tickets of the squash epic whose trailers the squash body holds.
	EpicDiffSquashLinked = 2
	// EpicDiffSquashTickets is how many done tickets the squash epic has; the last has no commit.
	EpicDiffSquashTickets = 3
	// EpicDiffGoneBase is the stamped base of EpicDiffStackedGone, which was never committed.
	EpicDiffGoneBase = "89abcdef0123456789abcdef0123456789abcdef"

	epicDiffPRFirst = 501 // PR numbers run from here in the order above
)

var epicDiffPREpics = []string{EpicDiffPRMerge, EpicDiffPRSquash, EpicDiffPRRebase, EpicDiffPRReworded}

// EpicDiffPRNumber is the PR number of one of the four PR epics.
func EpicDiffPRNumber(epic string) int {
	for i, name := range epicDiffPREpics {
		if name == epic {
			return epicDiffPRFirst + i
		}
	}
	return 0
}

// EpicDiffSquashSubject is the subject of the squash commit, GitHub's mark of a squash.
func EpicDiffSquashSubject() string {
	return fmt.Sprintf("Squash %s (#%d)", EpicDiffPRSquash, EpicDiffPRNumber(EpicDiffPRSquash))
}

// EpicDiffPRStep is the headline of commit n of a PR epic's branch.
func EpicDiffPRStep(epic string, n int) string { return fmt.Sprintf("%s: step %d", epic, n) }

func epicDiffPRScenario() Scenario {
	sc := Scenario{EpicMeta: map[string]string{}}
	done := func(id string) Ticket {
		return Ticket{ID: id, Slug: "step-" + id, Opts: servertest.TicketOpts{Status: "done", DoneAt: epicDiffMergedAt}}
	}
	tickets := func(n int) (out []Ticket) {
		for i := 1; i <= n; i++ {
			out = append(out, done(fmt.Sprintf("%02d", i)))
		}
		return out
	}
	prs := make([]ghfake.PR, len(epicDiffPREpics))
	for i, name := range epicDiffPREpics {
		n := epicDiffPRFirst + i
		url := fmt.Sprintf("https://github.com/%s/pull/%d", ghfake.Repo, n)
		sc.Epics = append(sc.Epics, Epic{Project: project, Name: name, Tickets: tickets(2)})
		sc.EpicMeta[project+":"+name] = fmt.Sprintf("status: open\ncompleted_at: 2026-02-10T10:00:00Z\npr_number: %d\npr_url: %s\n", n, url)
		prs[i] = ghfake.PR{Number: n, URL: url, Head: name, Base: "main", State: "OPEN", Mergeable: "MERGEABLE"}
	}
	sc.Epics[1].Tickets = tickets(EpicDiffSquashTickets)
	sc.Epics = append(sc.Epics,
		Epic{Project: project, Name: EpicDiffStackedKept, Tickets: tickets(1)},
		Epic{Project: project, Name: EpicDiffStackedGone, Tickets: tickets(1)},
	)
	sc.ProjectExtra = map[string]string{project: `,"delivery":"pr"`}
	sc.FakeGH = &FakeGH{State: ghfake.State{PRs: prs}}

	sc.GitSetup = func(git Git, repo string) error {
		s := &epicDiffGit{git: git, repo: repo}
		trailer := func(epic string, id string) string { return "demo:" + merge.TicketTrailerValue(epic, id) }
		branch := func(name, from string, commits int) {
			s.run("checkout", "-q", "-b", name, from)
			for i := 1; i <= commits; i++ {
				s.write(name+"/"+fmt.Sprint(i)+".txt", numbered(name, i))
				s.commit(EpicDiffPRStep(name, i), trailer(name, fmt.Sprintf("%02d", i)))
			}
		}
		for _, name := range epicDiffPREpics {
			branch(name, "main", 2)
		}
		branch(EpicDiffStackedKept, EpicDiffPRSquash, 1)
		keptBase, keptTip := s.sha(EpicDiffPRSquash), s.sha(EpicDiffStackedKept)
		s.run("checkout", "-q", "main")
		if s.err != nil {
			return s.err
		}
		for i := range prs {
			prs[i].HeadSHA = s.sha(prs[i].Head)
		}
		sc.EpicMeta[project+":"+EpicDiffStackedKept] = stackedMeta("range_base: " + keptBase + "\nrange_tip: " + keptTip + "\n")
		sc.EpicMeta[project+":"+EpicDiffStackedGone] = stackedMeta("range_base: " + EpicDiffGoneBase + "\nrange_tip: " + EpicDiffMissingTip + "\n")
		sc.FakeGH.OriginSetup = func(git Git, repo string) error {
			return landOnOrigin(&epicDiffGit{git: git, repo: repo}, prs, trailer)
		}
		return s.err
	}
	return sc
}

func stackedMeta(rng string) string {
	return "status: open\ncompleted_at: 2026-02-10T10:00:00Z\nmerged_at: " + epicDiffMergedAt + "\nblocked_by: [" + EpicDiffPRSquash + "]\n" + rng
}

// landOnOrigin merges each PR on origin's trunk in its own style, on a scratch
// branch pushed to origin's main, and leaves local trunk where it was. It marks
// the fake PRs MERGED with their merge commits.
func landOnOrigin(s *epicDiffGit, prs []ghfake.PR, trailer func(epic, id string) string) error {
	s.run("checkout", "-q", "-b", "landed", "main")
	mergeSHA := map[string]string{}

	s.run("merge", "--no-ff", "-q", "-m", fmt.Sprintf("Merge pull request #%d from %s", EpicDiffPRNumber(EpicDiffPRMerge), EpicDiffPRMerge), EpicDiffPRMerge)
	mergeSHA[EpicDiffPRMerge] = s.sha("HEAD")

	var body []string
	for i := 1; i <= EpicDiffSquashLinked; i++ {
		body = append(body, trailer(EpicDiffPRSquash, fmt.Sprintf("%02d", i)))
	}
	s.run("merge", "--squash", "-q", EpicDiffPRSquash)
	s.commit(EpicDiffSquashSubject(), body...)
	mergeSHA[EpicDiffPRSquash] = s.sha("HEAD")

	s.run("cherry-pick", EpicDiffPRRebase+"~1", EpicDiffPRRebase)
	mergeSHA[EpicDiffPRRebase] = s.sha("HEAD")

	for i, rev := range []string{EpicDiffPRReworded + "~1", EpicDiffPRReworded} {
		s.run("cherry-pick", rev)
		s.run("commit", "-q", "--amend", "--allow-empty", "-m", "Reworded: "+EpicDiffPRStep(EpicDiffPRReworded, i+1))
	}
	mergeSHA[EpicDiffPRReworded] = s.sha("HEAD")

	s.run("push", "-q", "origin", "landed:main")
	s.run("checkout", "-q", "main")
	s.run("branch", "-q", "-D", "landed")
	for i := range prs {
		prs[i].State, prs[i].MergeCommit = "MERGED", mergeSHA[prs[i].Head]
	}
	return s.err
}
