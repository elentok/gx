package wire

import (
	"encoding/json"
)

// Refusal reasons of the ask verb.
const (
	ReasonInvalidQuestion = "invalid-question"
	ReasonHandoffRequired = "handoff-required"
	ReasonQuestionOpen    = "question-open"
)

// Refusal reasons of the answer verb.
const (
	ReasonStale         = "stale"
	ReasonNotPaused     = "not-paused"
	ReasonNotAnswerable = "not-answerable"
	ReasonBadChoice     = "bad-choice"
	ReasonEmptyAnswer   = "empty-answer"
	ReasonTooLong       = "too-long"
)

// Outcomes of the answer verb.
const (
	OutcomeDelivered = "delivered"
	OutcomeAdopted   = "adopted"
	OutcomeQueued    = "queued"
)

// AFKRequest sets AFK mode; a nil On toggles it.
type AFKRequest struct {
	On *bool `json:"on,omitempty"`
}

// AFKResult is AFK mode after a write.
type AFKResult struct {
	AFK     bool   `json:"afk"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// EventAFKChanged announces that AFK mode was switched.
const EventAFKChanged = "afk-changed"

// Refusal reasons of the agent verbs. iteration-not-live is shared with nudge.
const (
	ReasonAgentNotReady = "agent-not-ready"
	ReasonNotBlocked    = "not-blocked"
	ReasonBadDecision   = "invalid-decision"
	// With a request ID: the open request differs, needs a real answer, or the agent is gone.
	ReasonRequestChanged = "request-changed"
	ReasonNotApprovable  = "not-approvable"
	// ReasonRequestIDRequired is the web's refusal of an answer that names no request.
	ReasonRequestIDRequired = "request-id-required"
	ReasonAgentGone         = "agent-gone"
	// ReasonAgentWaiting is the refusal of a plain resume on a live waiting
	// self-reported pause. It is public (ADR 0028) and named in the tracker text.
	ReasonAgentWaiting = "agent-waiting"
	// ReasonNotDelivered: a deny closed the dialog but its reason never reached the agent.
	ReasonNotDelivered = "reason-not-delivered"
)

// Refusal reasons of the nudge verb.
const (
	ReasonTextRequired     = "text-required"
	ReasonIterationNotLive = "iteration-not-live"
	ReasonRateLimited      = "rate-limited"
)

// EventTicketNudged is emitted after text was typed into an iteration's pane.
const EventTicketNudged = "ticket-nudged"

// Refusal reasons of the ticket writes (pause, relaunch, unmute).
const (
	ReasonReasonRequired   = "reason-required"
	ReasonAlreadyPaused    = "already-paused"
	ReasonNotMuted         = "not-muted"
	ReasonIterationRunning = "iteration-running"
	ReasonTicketDone       = "ticket-done"
	ReasonHerdrUnavailable = "herdr-unavailable"
	// ReasonAgentWorking refuses a retry while the ticket's agent is still
	// working; the web maps it to "Stop agent and retry".
	ReasonAgentWorking = "agent-working"
	// ReasonBudgetLimit refuses a relaunch or nudge past the daily soft
	// limit (the hard limit implies it), as claims are held back.
	ReasonBudgetLimit = "budget-limit"
	// ReasonUsageLimit refuses an automatic relaunch while the usage gate holds.
	ReasonUsageLimit = "usage-limit"
)

const (
	// NoCleanCommand is the text for a clean with no cleanup command to run.
	NoCleanCommand = "no cleanup command set"
)

// VerdictHerdrDown is the explain verdict for a queued ticket that launches on
// herdr while herdr is down or not installed.
const VerdictHerdrDown = "herdr down"

// CPUSpikeResult is what became of a reported spike.
type CPUSpikeResult struct {
	// Address is the investigate ticket filed for it; empty when none was.
	Address string `json:"address,omitempty"`
	// Message says why no ticket was filed.
	Message string `json:"message,omitempty"`
}

const (
	// EventHerdrUnavailable and EventHerdrAvailable are emitted on transition
	// only, never per failed retry.
	EventHerdrUnavailable = "herdr-unavailable"
	EventHerdrAvailable   = "herdr-available"
)

const (
	// EventTrunkMissing streams once when a project with queued roots enters
	// Missing trunk. Address is the project; Message is the reason.
	EventTrunkMissing = "trunk-missing"
	// EventTrunkFound streams once when that project's trunk resolves again.
	EventTrunkFound = "trunk-found"
	// VerdictTrunkMissing is the explain verdict for a queued ticket of a
	// project in Missing trunk.
	VerdictTrunkMissing = "trunk missing"
	// VerdictMigrationBlocked is the explain verdict for a queued ticket of a
	// project whose migration failed.
	VerdictMigrationBlocked = "Migration blocked"
)

// VerdictProjectUnavailable is the explain verdict for a ticket of a project
// whose path is missing.
const VerdictProjectUnavailable = "project unavailable"

// ChangedRequest is the body of POST /v1/tickets/changed: a direct write's
// "address changed" ping.
type ChangedRequest struct {
	Address string `json:"address"`
}

// EventReclaimed streams when a restarted server takes over an iteration that
// was already running.
const EventReclaimed = "reclaimed"

// ReasonVerbNotOffered refuses a recommendation of a verb outside detail's actions.
const ReasonVerbNotOffered = "verb-not-offered"

// VerdictHarnessMissing is the explain verdict for a queued ticket whose
// resolved harness is not on PATH.
const VerdictHarnessMissing = "harness missing"

const (
	// EventMigrationFailuresChanged is emitted when the migration failure record changes on disk.
	EventMigrationFailuresChanged = "migration-failures-changed"
)

// SaveChecksRequest saves one project's check config. Checks has the shape of
// .gx/checks.json.
type SaveChecksRequest struct {
	Project string          `json:"project"`
	Target  string          `json:"target"`
	Checks  json.RawMessage `json:"checks"`
}

// SaveChecksResult says what a save did. A refusal saved nothing.
type SaveChecksResult struct {
	Project string `json:"project,omitempty"`
	Target  string `json:"target,omitempty"`
	// Trunk and Commit are set for a repo save. Commit is empty when the file
	// already held this config.
	Trunk  string `json:"trunk,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Path is the file written for a project.json save.
	Path    string `json:"path,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Refusal reasons of a check save.
const (
	ReasonInvalidChecks = "invalid-checks"
	ReasonBadTarget     = "bad-target"
	ReasonVCSNone       = "vcs-none"
	ReasonNoTrunk       = "no-trunk-checkout"
	ReasonWrongBranch   = "wrong-branch"
	ReasonDirtyChecks   = "dirty-checks"
	ReasonCommitFailed  = "commit-failed"
)

// Save targets of a check config.
const (
	SaveTargetRepo        = "repo"
	SaveTargetProjectJSON = "project.json"
)

// ProjectRequest names a project; Path is what set-path points it at.
type ProjectRequest struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// ProjectResult is the outcome of a project remove or set-path.
type ProjectResult struct {
	Name    string `json:"name,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Refusal reasons of a project remove or set-path.
const (
	ReasonUnknownProject = "unknown-project"
	ReasonProjectBusy    = "project-busy"
	ReasonPathTaken      = "path-taken"
)
