package pr

import (
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/git/internal/run"
)

// PRListErrorKind classifies a gh pr list failure so callers can render
// tailored inline messages without re-parsing gh's output themselves.
type PRListErrorKind int

const (
	PRListErrorGeneric PRListErrorKind = iota
	PRListErrorGHNotInstalled
	PRListErrorUnauthenticated
	PRListErrorForbidden   // HTTP 403
	PRListErrorNotFound    // HTTP 404: missing, or a private repo the token cannot see
	PRListErrorRateLimited // HTTP 429
)

// PRListError wraps a gh pr list/related failure with a classified kind.
type PRListError struct {
	Kind PRListErrorKind
	Err  error
	// RetryAfter is the delay gh printed from a retry-after header, or zero.
	RetryAfter time.Duration
}

func (e *PRListError) Error() string { return e.Err.Error() }
func (e *PRListError) Unwrap() error { return e.Err }

// classifyPRListError distinguishes "gh not installed" and "gh
// unauthenticated" from gh's other failure modes (network, rate limit, no
// GitHub remote, ...), which fall back to gh's raw wrapped message.
func classifyPRListError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return &PRListError{Kind: PRListErrorGHNotInstalled, Err: err}
	}
	var runErr *run.RunError
	if !errors.As(err, &runErr) {
		return &PRListError{Kind: PRListErrorGeneric, Err: err}
	}
	// gh exits 4 when it needs authentication.
	if runErr.Code == 4 || isGHAuthFailure(runErr.Stderr) {
		return &PRListError{Kind: PRListErrorUnauthenticated, Err: err}
	}
	kind := PRListErrorGeneric
	switch {
	case strings.Contains(runErr.Stderr, "HTTP 429"):
		kind = PRListErrorRateLimited
	case strings.Contains(runErr.Stderr, "HTTP 403"):
		kind = PRListErrorForbidden
	case strings.Contains(runErr.Stderr, "HTTP 404"):
		kind = PRListErrorNotFound
	}
	return &PRListError{Kind: kind, Err: err, RetryAfter: parseRetryAfter(runErr.Stderr)}
}

var retryAfterRe = regexp.MustCompile(`(?i)retry-after:?\s*(\d+)`)

func parseRetryAfter(stderr string) time.Duration {
	m := retryAfterRe.FindStringSubmatch(stderr)
	if m == nil {
		return 0
	}
	secs, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func isGHAuthFailure(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "gh auth login")
}
