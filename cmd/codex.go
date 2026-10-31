package cmd

import "github.com/spf13/cobra"

func newCodexCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codex",
		Short: "Codex integration (doctor, setup)",
	}
	cmd.AddCommand(newCodexDoctorCmd(d))
	cmd.AddCommand(newCodexSetupCmd(d))
	return cmd
}
