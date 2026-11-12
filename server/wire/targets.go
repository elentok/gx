package wire

// Status values of a TargetResult.
const (
	TargetDone    = "done"
	TargetRefused = "refused"
	// TargetFits is a dry run's answer for a target that would be done.
	TargetFits = "fits"
	// TargetFailed is a target whose write hit an internal error part-way. Its
	// Message says what; the other targets' results still stand.
	TargetFailed = "failed"
)

// TargetResult is what one target of a many-target write came to. A refusal is
// a result, not an error (ADR 0028), and blocks no other target.
type TargetResult struct {
	Target string `json:"target"`
	Status string `json:"status"`
	// Reason is a stable code, set only when refused.
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// NewAddress is set only after a rename.
	NewAddress string `json:"new_address,omitempty"`
	// Sessions are the live Session ids the write stopped, or on a dry run would stop.
	Sessions []string `json:"sessions,omitempty"`
	// OnTrunk is an archive dry run's answer: whether gx sees the epic on trunk.
	OnTrunk *bool `json:"on_trunk,omitempty"`
}

// ArchiveRequest archives loop epics. Yes confirms, for the whole request,
// that archiving stops live Sessions. DryRun runs every rule and writes nothing.
type ArchiveRequest struct {
	Targets []string `json:"targets"`
	Yes     bool     `json:"yes,omitempty"`
	DryRun  bool     `json:"dry_run,omitempty"`
}

// UnarchiveRequest brings epics back out of .archive. Each target is an archive
// address (`project:name`, suffix included).
type UnarchiveRequest struct {
	Targets []string `json:"targets"`
}

// Refusal reasons of archive. The cleanup reasons cover queued, not done and unknown.
const (
	ReasonArchiveNotFinished  = "not-finished"
	ReasonArchiveRenameFailed = "rename-failed"
	// ReasonArchiveImplNotArchivable refuses a Design whose -impl epic cannot archive.
	ReasonArchiveImplNotArchivable = "impl-not-archivable"
)

// Refused reports whether the target was refused.
func (r TargetResult) Refused() bool { return r.Status == TargetRefused }

// TargetsResult is the envelope of a many-target write, in argument order.
type TargetsResult struct {
	Results []TargetResult `json:"results"`
	// Held is set only by hold and unhold: the held keys after the request.
	// A target that needs Yes is refused ReasonNeedsYes and lists its sessions.
	Held []string `json:"held,omitempty"`
}

// RefusedTarget is the result of a target the write refused.
func RefusedTarget(target, reason, message string) TargetResult {
	return TargetResult{Target: target, Status: TargetRefused, Reason: reason, Message: message}
}

// FailedTarget is the result of a target whose write failed with err.
func FailedTarget(target string, err error) TargetResult {
	return TargetResult{Target: target, Status: TargetFailed, Message: err.Error()}
}

// ReasonNeedsYes refuses a target whose write stops live Sessions and came without yes.
const ReasonNeedsYes = "needs-yes"
