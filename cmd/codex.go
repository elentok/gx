package cmd

import "github.com/spf13/cobra"

func newCodexCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codex",
		Short: "Codex integration (doctor)",
	}
	cmd.AddCommand(newCodexDoctorCmd(d))
	return cmd
}
