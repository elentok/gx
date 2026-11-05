// Package ticketsflow holds the workflow verbs of `gx tickets`: epic, publish,
// follow-up, the ensure verbs, verify-plan, calibration, lint-claims and
// filter-run-log. Root hands them to tickets.New as extra commands.
package ticketsflow

import (
	"os"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"

	"github.com/spf13/cobra"
)

// Deps is the hooks of the flow verbs on top of the shared env.
type Deps struct {
	cmdbase.Env
	// WriteFile writes ticket files in `tickets publish`; nil means
	// os.WriteFile (tests inject a faulty one).
	WriteFile func(path string, data []byte, perm os.FileMode) error
	// CommandTree builds the gx command tree `lint-claims` checks command
	// claims against. Root supplies it lazily: building the tree at
	// construction would recurse.
	CommandTree func() *cobra.Command
}

func (d Deps) dirs() config.Dirs { return d.Env.Dirs() }
