package wire

// Refusal reasons of the retry-migration route.
const (
	ReasonRetryRunning        = "migrating"
	ReasonRetryUnknownProject = "unknown-project"
)

// RetryMigrationRequest is the body of POST /v1/projects/retry-migration.
type RetryMigrationRequest struct {
	Project string `json:"project"`
}

// RetryMigrationResult answers it: Started, or Refused with a Reason.
type RetryMigrationResult struct {
	Started bool   `json:"started"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
