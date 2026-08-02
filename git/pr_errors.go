package git

import (
	"errors"
	"os/exec"
	"strings"
)

// PRListErrorKind classifies a gh pr list failure so callers can render
// tailored inline messages without re-parsing gh's output themselves.
type PRListErrorKind int

const (
	PRListErrorGeneric PRListErrorKind = iota
	PRListErrorGHNotInstalled
	PRListErrorUnauthenticated
)

// PRListError wraps a gh pr list/related failure with a classified kind.
type PRListError struct {
	Kind PRListErrorKind
	Err  error
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
	var runErr *RunError
	if errors.As(err, &runErr) && isGHAuthFailure(runErr.Stderr) {
		return &PRListError{Kind: PRListErrorUnauthenticated, Err: err}
	}
	return &PRListError{Kind: PRListErrorGeneric, Err: err}
}

func isGHAuthFailure(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "gh auth login")
}
