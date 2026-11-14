package history

import (
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/git/internal/run"
)

// PRMerge is what is known of a merged pull request.
type PRMerge struct {
	// TrunkTip is local trunk's tip after it took the merge.
	TrunkTip string
	// MergeCommit is GitHub's merge commit M; empty when unknown (a hand mark).
	MergeCommit string
	Number      int
	// Headlines are the PR's commit subjects, oldest first. Empty when the PR
	// branch is gone.
	Headlines []string
	// ForkPoint is the merge base of the PR head and trunk: the headline search
	// goes no deeper. Needed only with Headlines.
	ForkPoint string
}

// CommitRange is the commits a PR put on trunk: Base is the commit before them
// and Tip the last.
type CommitRange struct{ Base, Tip string }

// PRRange classifies how m landed on trunk. It returns the range, or a reason
// there is none (a guess is never returned). A git failure is an error.
//
//  1. M has two parents: a merge commit, (first parent, M).
//  2. The headlines are exactly one run of consecutive first-parent commits
//     between ForkPoint and M (or the trunk tip when M is unknown): a rebase.
//  3. M has one parent and its subject ends with "(#<number>)": a squash.
func PRRange(dir string, m PRMerge) (r CommitRange, reason string, err error) {
	var parents []string
	if m.MergeCommit != "" {
		if parents, err = parentsOf(dir, m.MergeCommit); err != nil {
			return CommitRange{}, "", err
		}
		if len(parents) == 0 {
			return CommitRange{}, "merge commit is not in the repository", nil
		}
		if len(parents) == 2 {
			return CommitRange{Base: parents[0], Tip: m.MergeCommit}, "", nil
		}
	}
	reason = "no commit run matches the PR's headlines"
	switch {
	case len(m.Headlines) == 0:
		reason = "the PR branch is gone, so its headlines are unknown"
	case m.ForkPoint == "":
		reason = "no fork point for the PR"
	default:
		start := m.TrunkTip
		if m.MergeCommit != "" {
			start = m.MergeCommit
		}
		runs, err := headlineRuns(dir, m.ForkPoint, start, m.Headlines)
		if err != nil {
			return CommitRange{}, "", err
		}
		switch len(runs) {
		case 1:
			return runs[0], "", nil
		case 0:
		default:
			return CommitRange{}, "the PR's headlines match more than one commit run", nil
		}
	}
	if len(parents) == 1 {
		subject, _, err := run.Exec(dir, []string{"log", "-1", "--format=%s", m.MergeCommit})
		if err != nil {
			return CommitRange{}, "", err
		}
		if strings.HasSuffix(strings.TrimSpace(subject), fmt.Sprintf("(#%d)", m.Number)) {
			return CommitRange{Base: parents[0], Tip: m.MergeCommit}, "", nil
		}
		reason += " and the merge commit is not marked as a squash"
	}
	return CommitRange{}, reason, nil
}

// Headlines lists the subjects of the commits in revRange, oldest first.
func Headlines(dir, revRange string) ([]string, error) {
	out, _, err := run.Exec(dir, []string{"log", "--format=%s", "--reverse", revRange, "--"})
	if err != nil {
		return nil, err
	}
	var subjects []string
	for l := range strings.SplitSeq(out, "\n") {
		if l != "" {
			subjects = append(subjects, l)
		}
	}
	return subjects, nil
}

func parentsOf(dir, rev string) ([]string, error) {
	out, _, err := run.Exec(dir, []string{"rev-list", "--parents", "-n", "1", "--end-of-options", rev})
	if err != nil {
		// An unknown commit is a missing one, not a git failure.
		if _, _, verr := run.Exec(dir, []string{"rev-parse", "--verify", "--quiet", rev + "^{commit}"}); verr != nil {
			return nil, nil
		}
		return nil, err
	}
	f := strings.Fields(out)
	if len(f) == 0 {
		return nil, nil
	}
	return f[1:], nil
}

// headlineRuns lists every run of consecutive first-parent commits of
// forkPoint..start whose subjects are headlines, oldest first.
func headlineRuns(dir, forkPoint, start string, headlines []string) ([]CommitRange, error) {
	out, _, err := run.Exec(dir, []string{"log", "--first-parent", "--reverse", "--format=%H %s", forkPoint + ".." + start, "--"})
	if err != nil {
		return nil, err
	}
	var shas, subjects []string
	for l := range strings.SplitSeq(out, "\n") {
		sha, subject, _ := strings.Cut(l, " ")
		if sha == "" {
			continue
		}
		shas, subjects = append(shas, sha), append(subjects, subject)
	}
	var runs []CommitRange
	n := len(headlines)
	for i := 0; i+n <= len(subjects); i++ {
		if !slices.Equal(subjects[i:i+n], headlines) {
			continue
		}
		base, _, err := run.Exec(dir, []string{"rev-parse", "--verify", shas[i] + "^"})
		if err != nil {
			return nil, err
		}
		runs = append(runs, CommitRange{Base: strings.TrimSpace(base), Tip: shas[i+n-1]})
	}
	return runs, nil
}
