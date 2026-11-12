package cmdbase

import "fmt"

// ExitError carries a specific exit code from a child process so that main
// can forward it rather than always exiting 1.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// JSONRefusalExit is the return value for a --json command after it printed its
// result body: a refusal exits 1 so scripts can skip parsing the body, and
// anything else exits 0. A bare exit code adds no second message to the body.
func JSONRefusalExit(refused bool) error {
	if refused {
		return &ExitError{Code: 1}
	}
	return nil
}
