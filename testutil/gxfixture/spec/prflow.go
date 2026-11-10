package spec

import (
	"fmt"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/ghfake"
)

// The PR epic every pr-flow scenario seeds, and the PR it has open.
const (
	PRFlowEpic   = "ticket-search"
	PRFlowNumber = 412
	PRFlowURL    = "https://github.com/" + ghfake.Repo + "/pull/412"
	prFlowHead   = "4120000000000000000000000000000000000412"
)

// FakeGH makes the fixture's gh the scripted fake (testutil/ghfake) and the
// project repo's origin a local bare repo. State is the fake's starting state;
// a flow edits the file the fixture prints as gh_state to change CI mid-run.
type FakeGH struct {
	State ghfake.State
	// FixCheck, when set, forks the epic's ci-fix ticket for this failed check
	// once the epic is seeded, which logs its "CI failed" entry too.
	FixCheck string
	// Activity is written to the epic's pr-activity.jsonl through the server's writer.
	Activity []prflow.Entry
	// Resolutions is the Resolution section of the seeded ticket with that id.
	Resolutions map[string]string
	// MergedFile makes the PR's merge commit (on origin) add this file, while local
	// Trunk has an uncommitted file of the same name: it cannot fast-forward.
	MergedFile string
}

// prCheck is a check of PR 412's head; status is "COMPLETED" or "IN_PROGRESS".
func prCheck(name, status, conclusion string) ghfake.Check {
	return ghfake.Check{Name: name, Status: status, Conclusion: conclusion, DetailsURL: PRFlowURL + "/checks?check=" + name}
}

func passed(names ...string) (out []ghfake.Check) {
	for _, n := range names {
		out = append(out, prCheck(n, "COMPLETED", "SUCCESS"))
	}
	return out
}

// prFlowScenario is the epic with done tickets done, whose PR is draft or ready
// with checks. No check flips unless a flow edits the state.
func prFlowScenario(done int, draft bool, checks []ghfake.Check, fixCheck string) scenario {
	var tickets []ticket
	for i := 1; i <= done; i++ {
		tickets = append(tickets, ticket{fmt.Sprintf("%02d", i), fmt.Sprintf("step-%d", i), servertest.TicketOpts{Status: "done"}})
	}
	addr := project + ":" + PRFlowEpic
	sc := scenario{
		// A commit on the epic branch, so it is never an ancestor of trunk and reads as unmerged.
		GitSetup: func(git Git, repo string) error {
			return CommitOnBranch(git, repo, PRFlowEpic, "feature.txt", "feature")
		},
		Epics: []epic{{project, PRFlowEpic, tickets}},
		EpicMeta: map[string]string{addr: "status: open\ncompleted_at: 2026-01-01T10:00:00Z\n" +
			fmt.Sprintf("pr_number: %d\npr_url: %s\n", PRFlowNumber, PRFlowURL)},
		ProjectExtra: map[string]string{project: `,"delivery":"pr"`},
		// A few seconds, so a flow with a no-check PR sees the grace pass.
		Config: []string{"pr.no-checks-grace=3s"},
		FakeGH: &FakeGH{FixCheck: fixCheck, State: ghfake.State{
			PRs:    []ghfake.PR{{Number: PRFlowNumber, URL: PRFlowURL, Head: PRFlowEpic, Base: "main", State: "OPEN", Draft: draft, HeadSHA: prFlowHead, Mergeable: "MERGEABLE"}},
			Checks: map[string][]ghfake.Check{prFlowHead: checks},
		}},
	}
	return sc
}

// PRFlowDraftCIRunning is a finished epic whose draft PR has two of five checks running.
func PRFlowDraftCIRunning() scenario {
	checks := append(passed("go-test", "vet", "lint"), prCheck("webui", "IN_PROGRESS", ""), prCheck("e2e", "IN_PROGRESS", ""))
	return prFlowScenario(8, true, checks, "")
}

// PRFlowFixing is a finished epic whose go-test check failed: its ci-fix ticket
// is claimed by a fake agent that holds.
func PRFlowFixing() scenario {
	checks := append([]ghfake.Check{prCheck("go-test", "COMPLETED", "FAILURE")}, passed("vet", "lint", "webui", "e2e")...)
	sc := prFlowScenario(9, true, checks, "go-test")
	sc.Queue = []string{project + ":" + PRFlowEpic}
	sc.Hold = true
	return sc
}

// PRFlowReady is a finished epic whose PR is out of draft with all five checks passed.
func PRFlowReady() scenario {
	return prFlowScenario(9, false, passed("go-test", "vet", "lint", "webui", "e2e"), "")
}
