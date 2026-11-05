// Package codex is `gx codex`: doctor and setup for the Codex integration.
package codex

import (
	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
)

// Deps is the runtime context of `gx codex`.
type Deps struct {
	cmdbase.Env
	// StdinIsTerminal reports whether stdin is a terminal; nil means no.
	StdinIsTerminal func() bool
}

// New builds the `gx codex` command.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codex",
		Short: "Codex integration (doctor, setup)",
	}
	cmd.AddCommand(newCodexDoctorCmd(d))
	cmd.AddCommand(newCodexSetupCmd(d))
	return cmd
}
