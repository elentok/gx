package wire

// EventExplainVerdictChange streams when a queued ticket's explain verdict
// moves to a different word. Address is the ticket; clients re-explain it.
const EventExplainVerdictChange = "explain-verdict-change"

// Verdicts only the server's queue and slot state can give. The rest are the
// scheduler's own decision words (blocked, stalled, done, error, claimed, ...).
const (
	VerdictEligible       = "eligible"
	VerdictWaitingInQueue = "waiting in queue"
	VerdictConcurrencyCap = "concurrency cap"
	VerdictNotQueued      = "not queued"

	VerdictRateLimited = "rate-limited"

	// These start every usage-gate verdict, one per harness (the rest is the
	// window and its reset); the web shows its override button on them.
	VerdictUsagePrefixClaude = "Claude usage: "
	VerdictUsagePrefixCodex  = "Codex usage: "

	VerdictWaitingOnAnswer    = "waiting on answer"
	VerdictWaitingReleased    = "waiting (slot released)"
	VerdictWaitingReleaseCapd = "waiting (slot held, release cap reached)"
)

// LaunchStep is the step a launching run is in: its label and its 1-based
// place among the four steps. The zero value means no launch runs.
type LaunchStep struct {
	Label    string `json:"label"`
	Position int    `json:"position"`
}

// PendingRow is one queue entry with its explain verdict.
type PendingRow struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
}
