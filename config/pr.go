package config

import "time"

const (
	defaultCIFixAttemptsPerCheck = 3
	defaultCIFixAttemptsPerPR    = 10
	defaultNoChecksGrace         = 2 * time.Minute
)

// PRConfig bounds the pull-request flow. Global only: project.json cannot set it.
type PRConfig struct {
	// CIFixAttemptsPerCheck caps ci-fix tickets for one failing check.
	CIFixAttemptsPerCheck int `json:"ci-fix-attempts-per-check"`
	// CIFixAttemptsPerPR caps ci-fix tickets for one pull request.
	CIFixAttemptsPerPR int `json:"ci-fix-attempts-per-pr"`
	// NoChecksGrace is how long a PR with no checks waits before it counts as
	// green. Zero is valid (no wait), so the key is read as a duration >= 0.
	NoChecksGrace time.Duration `json:"-"`
}

// DefaultPRConfig returns the pull-request flow defaults.
func DefaultPRConfig() PRConfig {
	return PRConfig{
		CIFixAttemptsPerCheck: defaultCIFixAttemptsPerCheck,
		CIFixAttemptsPerPR:    defaultCIFixAttemptsPerPR,
		NoChecksGrace:         defaultNoChecksGrace,
	}
}
