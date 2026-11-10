package spec

import (
	"strconv"
	"time"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/ghfake"
)

// prFlowAddr is the address of the epic every pr-flow scenario seeds.
const prFlowAddr = project + ":" + PRFlowEpic

const prFlowDone = "status: open\ncompleted_at: 2026-01-01T10:00:00Z\n"

// PRFlowCIFailing is a draft PR whose go-test check failed three times: two done
// ci-fix tickets, the activity of the page mock, and "CI failing" stored.
func PRFlowCIFailing() scenario {
	checks := append([]ghfake.Check{prCheck("go-test", "COMPLETED", "FAILURE")}, passed("vet", "lint", "webui", "e2e")...)
	sc := prFlowScenario(9, true, checks, "")
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	stamp := func(d time.Duration) string { return ago(d).UTC().Format(time.RFC3339) }
	fix := func(id string, d time.Duration) ticket {
		return ticket{id, "ci-fix-" + id, servertest.TicketOpts{Type: "ci-fix", Status: "done", DoneAt: stamp(d)}}
	}
	sc.Epics[0].Tickets = append(sc.Epics[0].Tickets, fix("10", 290*time.Minute), fix("11", 230*time.Minute))
	reason := "go-test failed 3 times. The last fix did not change the result."
	sc.EpicMeta[prFlowAddr] += "land_block_kind: ci-failing\nland_block_reason: " + strconv.Quote(reason) + "\n"
	sc.FakeGH.Resolutions = map[string]string{
		"10": "Outcome: fixed\nCause: nil case in the index lookup\nCommit: abc1234\n\nSecond attempt: guarded the nil case. Pushed abc1234.",
		"11": "Outcome: fixed\nCause: nil case in the index lookup\nCommit: def5678\n\nThird attempt: guarded the nil case. Pushed def5678.",
	}
	sc.FakeGH.Activity = []prflow.Entry{
		{Time: ago(360 * time.Minute), Kind: prflow.KindPROpened},
		{Time: ago(300 * time.Minute), Kind: prflow.KindCIFailed, Checks: []string{"go-test"}, TicketID: "10"},
		{Time: ago(240 * time.Minute), Kind: prflow.KindCIFailed, Checks: []string{"go-test"}, TicketID: "11"},
		{Time: ago(180 * time.Minute), Kind: prflow.KindCIFailed, Detail: "TestSearchIndex: nil pointer in index.go:88", Checks: []string{"go-test"}},
		{Time: ago(179 * time.Minute), Kind: prflow.KindPausedCIFailing, Detail: "CI failing"},
	}
	return sc
}

// PRFlowMergeBlocked is an epic whose PR is merged on GitHub while local Trunk
// holds an uncommitted same.txt that the merge adds, so Trunk cannot fast-forward.
func PRFlowMergeBlocked() scenario {
	sc := prFlowScenario(9, false, passed("go-test", "vet", "lint", "webui", "e2e"), "")
	sc.FakeGH.State.PRs[0].State = "MERGED"
	sc.FakeGH.MergedFile = "same.txt"
	// Merge again needs the epic's branch, which the PR's merge leaves behind.
	sc.GitSetup = func(git Git, repo string) error {
		return CommitOnBranch(git, repo, PRFlowEpic, "feature.txt", "feature")
	}
	sc.EpicMeta[prFlowAddr] = DirtyTrunkMeta("same.txt") + "completed_at: 2026-01-01T10:00:00Z\n" + prMeta()
	return sc
}

// PRFlowAwaiting is a complete epic of a `pr` project that has no PR yet.
func PRFlowAwaiting() scenario {
	sc := prFlowScenario(9, false, nil, "")
	sc.FakeGH.State.PRs = nil
	sc.EpicMeta[prFlowAddr] = prFlowDone
	return sc
}

// PRFlowMergeProject is a complete epic of project dfl, whose delivery is unset.
func PRFlowMergeProject() scenario {
	sc := PRFlowAwaiting()
	sc.Epics[0].Project = "dfl"
	sc.EpicMeta = map[string]string{"dfl:" + PRFlowEpic: prFlowDone}
	sc.ProjectExtra = nil
	// The Awaiting merge pill shows only while auto-merge is off.
	sc.ManualMerge = []string{"dfl"}
	return sc
}

func prMeta() string {
	return "pr_number: " + strconv.Itoa(PRFlowNumber) + "\npr_url: " + PRFlowURL + "\n"
}
