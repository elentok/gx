package pr

// CheckKey identifies a check across reruns: a legacy status is keyed by its
// context with a nil WorkflowID, as is any check with no workflow run.
type CheckKey struct {
	WorkflowID string
	Name       string
}

// Check is one check run or legacy status on a commit.
type Check struct {
	WorkflowID string
	Name       string
	HeadSHA    string
	Status     string
	Conclusion string

	// Detail for the fix agent; not part of the check's identity.
	URL   string
	JobID string
	RunID string
}

// Key returns the check's identity.
func (c Check) Key() CheckKey { return CheckKey{WorkflowID: c.WorkflowID, Name: c.Name} }

// ChecksResult is the classification of a PR head's checks.
type ChecksResult int

const (
	ChecksNone ChecksResult = iota
	ChecksPending
	ChecksFailed
	ChecksGreen
)

type checkOutcome int

const (
	outcomeGreen checkOutcome = iota
	outcomePending
	outcomeFailed
)

// classifyCheck is the single source of truth for what a status/conclusion
// pair means. CANCELLED, STALE and ACTION_REQUIRED are neither failed nor
// green, so they count as pending; so does any conclusion we do not know.
func classifyCheck(status, conclusion string) checkOutcome {
	if status != "COMPLETED" {
		return outcomePending
	}
	switch conclusion {
	case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE":
		return outcomeFailed
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return outcomeGreen
	}
	return outcomePending
}

// latestChecks is the newest check per key on headSHA, in first-seen order.
func latestChecks(headSHA string, checks []Check) (order []CheckKey, latest map[CheckKey]Check) {
	latest = map[CheckKey]Check{}
	for _, c := range checks {
		if c.HeadSHA != headSHA {
			continue
		}
		if _, seen := latest[c.Key()]; !seen {
			order = append(order, c.Key())
		}
		latest[c.Key()] = c
	}
	return order, latest
}

// ChecksSummary is everything a poll derives from the checks of one head.
type ChecksSummary struct {
	Result ChecksResult
	// Failed lists the keys of the failed checks.
	Failed []CheckKey
	// FailedChecks is the newest check of each failed key, with fix-agent detail.
	FailedChecks []Check
	Counts       CheckCounts
	// Head is the newest check per key, in first-seen order.
	Head []Check
}

// Summarize classifies, counts and lists the checks on headSHA in one pass over
// latestChecks. Checks on other commits are ignored. When the same key appears
// more than once, the last one wins, so callers pass reruns in order.
func Summarize(headSHA string, checks []Check) ChecksSummary {
	order, latest := latestChecks(headSHA, checks)
	s := ChecksSummary{Head: make([]Check, 0, len(order))}
	for _, k := range order {
		c := latest[k]
		s.Head = append(s.Head, c)
		switch classifyCheck(c.Status, c.Conclusion) {
		case outcomeFailed:
			s.Counts.Failed++
			s.Failed = append(s.Failed, k)
			s.FailedChecks = append(s.FailedChecks, c)
		case outcomePending:
			s.Counts.Pending++
		default:
			s.Counts.Green++
		}
	}
	switch {
	case len(order) == 0:
		s.Result = ChecksNone
	case s.Counts.Failed > 0:
		s.Result = ChecksFailed
	case s.Counts.Pending > 0:
		s.Result = ChecksPending
	default:
		s.Result = ChecksGreen
	}
	return s
}

// Green reports whether the check completed without failing.
func (c Check) Green() bool { return classifyCheck(c.Status, c.Conclusion) == outcomeGreen }

// Completed reports whether the check has finished, whatever its conclusion.
func (c Check) Completed() bool { return c.Status == "COMPLETED" }

// CheckCounts is how many checks on a head are in each state.
type CheckCounts struct{ Pending, Failed, Green int }
