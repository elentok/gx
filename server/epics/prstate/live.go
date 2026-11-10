package prstate

import (
	"fmt"

	"github.com/elentok/gx/server/wire"
)

// Live is the watcher's view of a PR, reduced to what the words need.
type Live struct {
	Draft, Merged, Unreachable bool
	Pending, Failed, Green     int
	// FailedName is the first failed check's name.
	FailedName string
}

// CheckSummary is the one-line check state: "2 of 5 checks running",
// "go-test failed", "5 of 5 checks passed". It is "" with no checks.
func CheckSummary(l Live) string {
	total := l.Pending + l.Failed + l.Green
	switch {
	case total == 0:
		return ""
	case l.Failed > 0 && l.FailedName != "":
		return l.FailedName + " failed"
	case l.Failed > 0:
		return fmt.Sprintf("%d of %d checks failed", l.Failed, total)
	case l.Pending > 0:
		return fmt.Sprintf("%d of %d checks running", l.Pending, total)
	}
	return fmt.Sprintf("%d of %d checks passed", total, total)
}

// Info is the PRInfo of an epic with a PR: number and URL from the epic, the
// rest from live, which is nil before the first poll answers or after a restart.
func Info(number int, url string, merged bool, live *Live) wire.PRInfo {
	info := wire.PRInfo{Number: number, URL: url, Chip: fmt.Sprintf("#%d", number)}
	if merged {
		info.State = wire.PRMerged
	}
	if live != nil {
		info.Unreachable = live.Unreachable
		info.Checks = CheckSummary(*live)
		switch {
		case live.Merged:
			info.State = wire.PRMerged
		case live.Draft:
			info.State = wire.PRDraft
		default:
			info.State = wire.PRReady
		}
	}
	if info.State != "" {
		info.Chip += " · " + string(info.State)
	}
	return info
}

// Words is the status line of a PR-flow state, "" when the merge rules own it.
// attempt is the current ci-fix attempt out of max. A merge-blocked epic whose
// PR merged reads "PR merged".
func Words(state wire.EpicListState, info *wire.PRInfo, live *Live, attempt, max int) string {
	switch state {
	case wire.EpicListWritingPRText:
		return "Writing PR text"
	case wire.EpicListFixingCI:
		return fmt.Sprintf("Fixing CI (attempt %d of %d)", attempt, max)
	case wire.EpicListCIFailing:
		return "CI failing"
	case wire.EpicListMergeBlocked:
		if info != nil && info.State == wire.PRMerged {
			return "PR merged"
		}
	case wire.EpicListPROpen:
		switch {
		case info == nil || info.State == "":
			return "PR open"
		case info.State == wire.PRMerged:
			return "PR merged"
		case info.State == wire.PRReady:
			return "Ready for review"
		case live != nil && live.Pending > 0:
			return "PR draft · CI running"
		}
		return "PR draft"
	}
	return ""
}
