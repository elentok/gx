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

// ClassifyChecks classifies the checks on headSHA. Checks on other commits are
// ignored. When the same key appears more than once, the last one wins, so
// callers pass reruns in order. Failed lists the keys of the failed checks.
func ClassifyChecks(headSHA string, checks []Check) (result ChecksResult, failed []CheckKey) {
	latest := map[CheckKey]Check{}
	var order []CheckKey
	for _, c := range checks {
		if c.HeadSHA != headSHA {
			continue
		}
		if _, seen := latest[c.Key()]; !seen {
			order = append(order, c.Key())
		}
		latest[c.Key()] = c
	}
	if len(order) == 0 {
		return ChecksNone, nil
	}
	pending := false
	for _, k := range order {
		switch classifyCheck(latest[k].Status, latest[k].Conclusion) {
		case outcomeFailed:
			failed = append(failed, k)
		case outcomePending:
			pending = true
		}
	}
	switch {
	case len(failed) > 0:
		return ChecksFailed, failed
	case pending:
		return ChecksPending, nil
	}
	return ChecksGreen, nil
}
