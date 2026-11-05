package tickets

import (
	"fmt"
	"io"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
)

// runTicketsRoot prints the cwd repo's ticket root (its project directory in
// the ticket store) to w with no trailing decoration, so it's safe to use as
// `$(gx tickets root)`.
func runTicketsRoot(dirs config.Dirs, cwd string, w io.Writer) error {
	root, err := tickets.RootForDirs(dirs, cwd)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, root)
	return nil
}
