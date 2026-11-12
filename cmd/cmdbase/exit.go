package cmdbase

import (
	"fmt"
	"io"
	"os"
)

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

// EncodeProvenanceRefusal prints res as a server-stamped --json body, then
// returns JSONRefusalExit(refused). It is the whole tail of a --json write.
func EncodeProvenanceRefusal(w io.Writer, res any, refused bool) error {
	if err := EncodeProvenance(w, res, ViaServer, CallerActor(os.Getwd)); err != nil {
		return err
	}
	return JSONRefusalExit(refused)
}
