package wire

// EventTriageChanged is emitted with the project name as its address when a
// project's live Triage record changed.
const EventTriageChanged = "triage-changed"

// TriageChangedRequest is the body of POST /v1/triage/changed: a CLI verb's
// "this project's Triage record changed" ping.
type TriageChangedRequest struct {
	Project string `json:"project"`
}

// TriageChangedResult acknowledges a ping.
type TriageChangedResult struct {
	Project string `json:"project"`
}
