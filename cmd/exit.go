package cmd

import "fmt"

// ExitError carries a specific exit code from a child process so that main
// can forward it rather than always exiting 1.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }
